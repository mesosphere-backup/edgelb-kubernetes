package com.mesosphere.sdk.edgelb_pool.scheduler;

import com.mesosphere.sdk.dcos.DcosConstants;
import com.mesosphere.sdk.offer.Constants;
import com.mesosphere.sdk.scheduler.SchedulerFlags;
import com.mesosphere.sdk.scheduler.DefaultScheduler;
import com.mesosphere.sdk.specification.*;
import com.mesosphere.sdk.specification.yaml.RawServiceSpec;

import com.google.common.annotations.VisibleForTesting;
import com.google.common.base.Strings;

import org.apache.mesos.Protos;

import org.apache.commons.lang3.StringUtils;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.File;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collection;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.HashMap;
import java.util.HashSet;
import java.util.stream.Collectors;

/**
 * EdgeLB service.
 */
public class Main {
    static final String SECRETS_PREFIX = "PARSESECRET_";
    static final String NETWORKS_PREFIX = "PARSENETWORK_";

    @VisibleForTesting
    static final String POOL_PORTS_VAR = "POOL_PORTS";
    @VisibleForTesting
    static final String SECRETS_SECRET_PREFIX = SECRETS_PREFIX + "SECRET";
    @VisibleForTesting
    static final String SECRETS_FILE_PREFIX = SECRETS_PREFIX + "FILE";
    @VisibleForTesting
    static final String NETWORKS_NAME_PREFIX = NETWORKS_PREFIX + "NAME";
    @VisibleForTesting
    static final String NETWORKS_LABELS_PREFIX = NETWORKS_PREFIX + "LABELS";

    private static final String POOL_POD_NAME = "edgelb-pool";
    private static final String POOL_TASK_NAME = "server";
    private static final Logger LOGGER = LoggerFactory.getLogger(Main.class);

    public static void main(String[] args) throws Exception {
        ArrayList<Integer> ports = new ArrayList<Integer>();
        String portStr = System.getenv(POOL_PORTS_VAR);
        if (StringUtils.isBlank(portStr)) {
            throw new IllegalStateException(String.format("Environment variable not defined: %s", POOL_PORTS_VAR));
        }
        LOGGER.info(String.format("%s=%s", POOL_PORTS_VAR, portStr));
        for (String s : portStr.split(",")) {
            ports.add(Integer.valueOf(s));
        }

        List<SecretInfo> secrets = fetchSecrets(System.getenv());
        for (SecretInfo s : secrets) {
            LOGGER.info(String.format("Secret: %s File: %s", s.secret(), s.file()));
        }

        List<NetworkInfo> networks = fetchNetworks(System.getenv());
        for (NetworkInfo s : networks) {
            LOGGER.info(String.format("Network: %s Labels: %s", s.name(), s.labels()));
        }

        if (args.length > 0) {
            RawServiceSpec spec = RawServiceSpec.newBuilder(new File(args[0])).build();
            new DefaultService(getBuilder(spec, ports, secrets, networks)).run();
        } else {
            LOGGER.error("Missing file argument");
            System.exit(1);
        }
    }

    private static List<SecretInfo> fetchSecrets(Map<String, String> envMap) throws Exception {
        Map<String, String> secretMap = new HashMap<String, String>();
        Map<String, String> fileMap = new HashMap<String, String>();
        for (Map.Entry<String, String> e : envMap.entrySet()) {
            String key = e.getKey();
            String value = e.getValue();
            if (key.startsWith(SECRETS_SECRET_PREFIX)) {
                String id = key.substring(SECRETS_SECRET_PREFIX.length());
                secretMap.put(id, value);
            }
            if (key.startsWith(SECRETS_FILE_PREFIX)) {
                String id = key.substring(SECRETS_FILE_PREFIX.length());
                fileMap.put(id, value);
            }
        }

        ArrayList<SecretInfo> secrets = new ArrayList<SecretInfo>();

        // We must completely copy the set of keys into a new object because we can't remove
        // items from secretMap while iterating the .keySet(). From the docs: "The set is
        // backed by the map, so changes to the map are reflected in the set, and vice-versa."
        HashSet<String> ids = new HashSet<>(secretMap.keySet());
        for (String id : ids) {
            String secret = secretMap.get(id);
            String file = fileMap.get(id);
            if (file == null) {
                throw new IllegalStateException(String.format("Missing Secret file: %s", id));
            }
            secretMap.remove(id);
            fileMap.remove(id);

            secrets.add(new SecretInfo(secret, file));
        }

        if (!(secretMap.isEmpty() && fileMap.isEmpty())) {
            throw new IllegalStateException("Secrets did not match up");
        }
        return secrets;
    }

    private static List<NetworkInfo> fetchNetworks(Map<String, String> envMap) throws Exception {
        Map<String, String> nameMap = new HashMap<>();
        Map<String, String> labelsMap = new HashMap<>();
        for (Map.Entry<String, String> e : envMap.entrySet()) {
            String key = e.getKey();
            String value = e.getValue();
            if (key.startsWith(NETWORKS_NAME_PREFIX)) {
                String id = key.substring(NETWORKS_NAME_PREFIX.length());
                nameMap.put(id, value);
            }
            if (key.startsWith(NETWORKS_LABELS_PREFIX)) {
                String id = key.substring(NETWORKS_LABELS_PREFIX.length());
                labelsMap.put(id, value);
            }
        }

        ArrayList<NetworkInfo> networks = new ArrayList<>();

        // See the explanation for the HashSet in Secrets.
        HashSet<String> ids = new HashSet<>(nameMap.keySet());
        for (String id : ids) {
            String name = nameMap.get(id);
            String labels = labelsMap.get(id);
            if (name == null) {
                throw new IllegalStateException(String.format("Missing Network Name: %s", id));
            }
            nameMap.remove(id);
            if (labels != null) {
                labelsMap.remove(id);
            }
            networks.add(new NetworkInfo(name, labels));
        }

        if (!(nameMap.isEmpty() && labelsMap.isEmpty())) {
            throw new IllegalStateException("Networks did not match up");
        }
        return networks;
    }

    private static DefaultScheduler.Builder getBuilder(
            RawServiceSpec rawServiceSpec,
            List<Integer> ports,
            List<SecretInfo> secrets,
            List<NetworkInfo> networks) throws Exception {

        SchedulerFlags schedulerFlags = SchedulerFlags.fromEnv();
        DefaultServiceSpec serviceSpec = DefaultServiceSpec.newGenerator(rawServiceSpec, schedulerFlags).build();
        return DefaultScheduler
                .newBuilder(customServiceSpec(serviceSpec, ports, secrets, networks), schedulerFlags)
                .setPlansFrom(rawServiceSpec);
    }

    private static ServiceSpec customServiceSpec(
            DefaultServiceSpec serviceSpec,
            List<Integer> ports,
            List<SecretInfo> secrets,
            List<NetworkInfo> networks) throws Exception {

        List<PodSpec> newPods = new ArrayList<PodSpec>();
        for (PodSpec p : serviceSpec.getPods()) {
            if (!p.getType().equals(POOL_POD_NAME)) {
                newPods.add(p);
                continue;
            }
            newPods.add(customPodSpec(serviceSpec, ports, secrets, networks, p));
        }
        return DefaultServiceSpec.newBuilder(serviceSpec)
            .pods(newPods)
            .build();
    }

    private static PodSpec customPodSpec(
            DefaultServiceSpec serviceSpec,
            List<Integer> ports,
            List<SecretInfo> secrets,
            List<NetworkInfo> networks,
            PodSpec podSpec) throws Exception {

        if (!podSpec.getNetworks().isEmpty()) {
            throw new IllegalStateException("Networks already defined");
        }
        ArrayList<String> networkNames = new ArrayList<>();
        List<NetworkSpec> newNetworks = new ArrayList<>();
        for (NetworkInfo n : networks) {
            // Warn for networks that support port mapping for now
            // (only mesos-bridge) since we aren't dealing with portMappings.
            boolean supportsPortMapping = DcosConstants.networkSupportsPortMapping(n.name());
            if (supportsPortMapping) {
                LOGGER.warn(
                    "Virtual network '{}' supports port mappings, but none will be ceated",
                    n.name());
            }
            DcosConstants.warnIfUnsupportedNetwork(n.name);
            networkNames.add(n.name);
            newNetworks.add(customNetworkSpec(n));
        }

        List<TaskSpec> newTasks = new ArrayList<TaskSpec>();
        for (TaskSpec t : podSpec.getTasks()) {
            if (!t.getName().equals(POOL_TASK_NAME)) {
                newTasks.add(t);
                continue;
            }
            newTasks.add(customTaskSpec(serviceSpec, podSpec, ports, networkNames, t));
        }

        if (!podSpec.getSecrets().isEmpty()) {
            throw new IllegalStateException("Secrets already defined");
        }
        List<SecretSpec> newSecrets = new ArrayList<SecretSpec>();
        for (SecretInfo s : secrets) {
            newSecrets.add(customSecretSpec(s));
        }

        return DefaultPodSpec.newBuilder(podSpec)
            .tasks(newTasks)
            .secrets(newSecrets)
            .networks(newNetworks)
            .build();
    }

    private static SecretSpec customSecretSpec(SecretInfo secretInfo) throws Exception {
        return DefaultSecretSpec.newBuilder()
            .secretPath(secretInfo.secret())
            .filePath(secretInfo.file())
            .build();
    }

    private static NetworkSpec customNetworkSpec(NetworkInfo networkInfo) throws Exception {
        Map<String, String> labels;
        if (!Strings.isNullOrEmpty(networkInfo.labels())) {
            labels = networkInfo
                    .getValidatedLabels()
                    .stream()
                    .collect(Collectors.toMap(s -> s[0], s -> s[1]));
        } else {
             labels = Collections.emptyMap();
        }

        return DefaultNetworkSpec.newBuilder()
            .networkName(networkInfo.name())
            .networkLabels(labels)
            .portMappings(Collections.emptyMap())
            .build();
    }

    private static TaskSpec customTaskSpec(
            DefaultServiceSpec serviceSpec,
            PodSpec podSpec,
            List<Integer> ports,
            ArrayList<String> networkNames,
            TaskSpec taskSpec) throws Exception {

        ResourceSet resourceSet = taskSpec.getResourceSet();
        return DefaultTaskSpec.newBuilder(taskSpec)
            .resourceSet(customResourceSet(serviceSpec, podSpec, ports, networkNames, resourceSet))
            .build();
    }

    private static ResourceSet customResourceSet(
            DefaultServiceSpec serviceSpec,
            PodSpec podSpec,
            List<Integer> ports,
            ArrayList<String> networkNames,
            ResourceSet resourceSet) throws Exception {

        if (!(resourceSet instanceof DefaultResourceSet)) {
            throw new IllegalStateException("ResourceSet not a DefaultResourceSet");
        }
        DefaultResourceSet.Builder resourceSetBuilder = DefaultResourceSet.newBuilder((DefaultResourceSet) resourceSet);
        Collection<PortSpec> portSpecs = customPortSpecs(serviceSpec, podSpec, ports, networkNames, resourceSet);
        for (PortSpec p : portSpecs) {
            resourceSetBuilder.addResource(p);
        }
        return resourceSetBuilder
            .id(resourceSet.getId()) // XXX Currently a hack in here to set
                                     // the id, probably should be done in the
                                     // newBuiler() itself.
            .build();
    }

    private static Collection<PortSpec> customPortSpecs(
            DefaultServiceSpec serviceSpec,
            PodSpec podSpec,
            List<Integer> ports,
            ArrayList<String> networkNames,
            ResourceSet resourceSet) throws Exception {

        // Mostly copied from YAMLToInternalMappers

        String role = customRole(serviceSpec, podSpec);

        String principal = serviceSpec.getPrincipal();
        String envKey = null;
        Collection<PortSpec> portSpecs = new ArrayList<PortSpec>();
        Protos.Value.Builder portsValueBuilder = Protos.Value.newBuilder().setType(Protos.Value.Type.RANGES);

        for (Integer p : ports) {
            String name = String.format("p%d", p);
            Protos.Value.Builder portValueBuilder = Protos.Value.newBuilder()
                    .setType(Protos.Value.Type.RANGES);
            portValueBuilder.getRangesBuilder().addRangeBuilder()
                    .setBegin(p)
                    .setEnd(p);
            portsValueBuilder.mergeRanges(portValueBuilder.getRanges());
            portSpecs.add(new PortSpec(
                    portValueBuilder.build(),
                    role,
                    podSpec.getPreReservedRole(),
                    principal,
                    envKey,
                    name,
                    Constants.DISPLAYED_PORT_VISIBILITY,
                    networkNames));
        }

        return portSpecs;
    }

    private static String customRole(
            DefaultServiceSpec serviceSpec,
            PodSpec podSpec) {

        String preRole = podSpec.getPreReservedRole();
        String subRole = serviceSpec.getRole();

        if (preRole == null || preRole.equals(Constants.ANY_ROLE)) {
            return subRole;
        } else {
            return String.format("%s/%s", preRole, subRole);
        }
    }

    public static final class SecretInfo {
        final private String secret;
        final private String file;

        private SecretInfo(String secret, String file) {
            this.secret = secret;
            this.file = file;
        }

        public String secret() {
            return this.secret;
        }

        public String file() {
            return this.file;
        }
    }

    public static final class NetworkInfo {
        final private String name;
        final private String labels;

        private NetworkInfo(String name, String labels) {
            this.name = name;
            this.labels = labels;
        }

        public String name() {
            return this.name;
        }

        public String labels() {
            return this.labels;
        }

        public List<String[]> getValidatedLabels() throws IllegalArgumentException {
        List<String[]> kvs = Arrays.stream(this.labels.split(","))
                .map(s -> s.split(":"))
                .collect(Collectors.toList());
        kvs.forEach(kv -> {
            if (kv.length != 2) {
                throw new IllegalArgumentException(String.format("Illegal label string, got %s, should be " +
                        "comma-seperated key value pairs (seperated by colons)." +
                        " For example: k_0:v_0,k_1:v_1,...,k_n:v_n", this.labels));
            }
        });
        return kvs;
    }
    }
}
