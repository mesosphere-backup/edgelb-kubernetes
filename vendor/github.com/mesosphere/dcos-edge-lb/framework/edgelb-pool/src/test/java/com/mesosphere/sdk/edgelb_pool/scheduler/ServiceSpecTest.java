package com.mesosphere.sdk.edgelb_pool.scheduler;

import com.mesosphere.sdk.testing.BaseServiceSpecTest;
import org.junit.Test;

public class ServiceSpecTest extends BaseServiceSpecTest {

    public ServiceSpecTest() {
        super(
            "EXECUTOR_URI", "",
            "LIBMESOS_URI", "",
            "PORT_API", "8080",
            "FRAMEWORK_NAME", "edgelb-pool",
            "POOL_COUNT", "2",
            "POOL_RESERVED_ROLE", "slave_public",
            "POOL_CPUS", "0.1",
            "POOL_MEM", "512",
            "POOL_SIDECAR_CPUS", "0.1",
            "POOL_SIDECAR_MEM", "32",
            "POOL_DISK", "5000",
            "POOL_IMAGE", "mesosphere/dhape:0.0.1",
            "POOL_CONSTRAINTS", "",
            "POOL_RELATIVE_VOLUME_PATH", "persistent",
            Main.POOL_PORTS_VAR, "80,443",
            Main.SECRETS_SECRET_PREFIX + "1", "mysecretpath",
            Main.SECRETS_FILE_PREFIX + "1", "myfilepath",
            Main.NETWORKS_NAME_PREFIX + "1", "myvnet",
            Main.NETWORKS_LABELS_PREFIX + "1", "key0:value0,key1:value1");
    }

    @Test
    public void testYaml() throws Exception {
        super.testYaml("svc.yml");
    }
}
