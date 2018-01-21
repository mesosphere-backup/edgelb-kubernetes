package models

import (
	"fmt"
	"strconv"
	"strings"
)

// CheckConfig config
func V2CheckConfig(cfg *V2Config) error {
	pnames := make(map[string]struct{})
	for _, pool := range cfg.Pools {
		name := pool.Name
		if _, exist := pnames[name]; exist {
			// We purposefully leave out the namespace when checking. This is
			// because we use the name without the namespace in the HTTP API
			// url path.
			return failCheck("duplicate pool.name: %s", name)
		}
		pnames[name] = struct{}{}
	}

	for _, pool := range cfg.Pools {
		if err := V2CheckPool(pool); err != nil {
			return err
		}
	}
	return nil
}

// CheckPool pool
func V2CheckPool(pool *V2Pool) error {
	if pool.Name == "" {
		return failString("pool.name")
	}
	if err := checkName(pool.Name); err != nil {
		return failCheck("pool.name: %s", err)
	}
	if err := checkNamespace(*pool.Namespace); err != nil {
		return failCheck("pool.namespace: %s", err)
	}
	if err := v2checkPoolBindPorts(pool); err != nil {
		return err
	}
	if err := v2checkPoolSecrets(pool); err != nil {
		return err
	}
	if err := v2checkPoolVirtualNetworks(pool); err != nil {
		return err
	}
	return v2checkHaproxy(pool.Haproxy)
}

// PoolBindPorts creates a slice of all ports needed for binding
// XXX: The CLI or WebUI should provide a warning if the pool.ports field is
//      used to let the user know that only those ports will be allocated.
//      This represents a potentially unexpected change in behaviour from
//      the default of allocating all frontend bindPorts and stats.bindPort.
//      If the field is used, then it should include all ports that need to
//      be allocated by the pool scheduler. Normally these are [80, 443, 9090].
// Create a slice of ports that need to be allocated for a pool
func V2PoolBindPorts(pool *V2Pool) []int32 {
	// Override for ports to allocate was specified
	if len(pool.Ports) > 0 {
		return pool.Ports
	}

	// Infer ports to allocate from rest of config
	var ports []int32
	ports = append(ports, pool.Haproxy.Stats.BindPort)
	for _, fe := range pool.Haproxy.Frontends {
		ports = append(ports, *fe.BindPort)
	}

	return ports
}

// V2PoolBindPortsStr returns all bind ports for a v2 pool as a string slice
func V2PoolBindPortsStr(pool *V2Pool) []string {
	var sPorts []string
	ports := V2PoolBindPorts(pool)
	for _, p := range ports {
		s := strconv.Itoa(int(p))
		sPorts = append(sPorts, s)
	}
	return sPorts

}

// V2CheckMarathonService checks a marathon service
func V2CheckMarathonService(sv *V2ServiceMarathon) error {
	if sv.ServiceID == "" && sv.ServiceIDPattern == "" {
		return failString("server.marathon.serviceID")
	}
	return nil
}

// V2CheckMesosService checks a mesos service
func V2CheckMesosService(sv *V2ServiceMesos) error {
	if sv.FrameworkName == "" && sv.FrameworkNamePattern == "" &&
		sv.FrameworkID == "" && sv.FrameworkIDPattern == "" {
		return failString("server.mesos.FrameworkName and server.mesos.FrameworkID")
	}
	if sv.TaskName == "" && sv.TaskNamePattern == "" &&
		sv.TaskID == "" && sv.TaskIDPattern == "" {
		return failString("server.mesos.TaskName and server.mesos.TaskID")
	}
	return nil
}

func v2checkPoolBindPorts(pool *V2Pool) error {
	ports := V2PoolBindPorts(pool)
	portsMap := make(map[int32]struct{})
	for _, p := range ports {
		if err := checkPort("bindPort", p); err != nil {
			return err
		}
		if _, exists := portsMap[p]; exists {
			return failCheck("duplicate bindPort: %d", p)
		}
		portsMap[p] = struct{}{}
	}
	return nil
}

func v2checkPoolSecrets(pool *V2Pool) error {
	secretMap := make(map[string]struct{})
	fileMap := make(map[string]struct{})
	for _, sc := range pool.Secrets {
		name := sc.Secret
		file := sc.File

		if name == "" {
			return failString("pool.secrets.secret")
		}
		if file == "" {
			return failString("pool.secrets.file")
		}
		if err := checkSecretFile(file); err != nil {
			return failCheck("pool.secrets.file: %s", err)
		}

		if _, exists := secretMap[name]; exists {
			return failCheck("duplicate secret name: %s", name)
		}
		secretMap[name] = struct{}{}

		if _, exists := fileMap[file]; exists {
			return failCheck("duplicate secret file: %s", file)
		}
		fileMap[file] = struct{}{}
	}
	return nil
}

func v2checkPoolVirtualNetworks(pool *V2Pool) error {
	nameMap := make(map[string]struct{})
	for _, net := range pool.VirtualNetworks {
		name := net.Name

		if name == "" {
			return failString("pool.virtualNetworks.name")
		}

		if _, exists := nameMap[name]; exists {
			return failCheck("duplicate virtualNetwork name: %s", name)
		}
		nameMap[name] = struct{}{}
	}
	return nil
}

func v2checkHaproxy(hap *V2Haproxy) error {
	fenames := make(map[string]struct{})
	referencedBackends := make(map[string]struct{})
	for _, fe := range hap.Frontends {
		var name string
		if fe.Name == "" {
			name = fmt.Sprintf("frontend_%s_%d", fe.BindAddress, *fe.BindPort)
		} else {
			name = fe.Name
		}

		if _, exist := fenames[name]; exist {
			return failCheck("duplicate frontend.name: %s", name)
		}
		fenames[name] = struct{}{}

		referencedBackends[fe.LinkBackend.DefaultBackend] = struct{}{}
		for _, lbeMap := range fe.LinkBackend.Map {
			referencedBackends[lbeMap.Backend] = struct{}{}
		}
	}
	delete(referencedBackends, "")

	benames := make(map[string]struct{})
	for _, be := range hap.Backends {
		name := be.Name
		if _, exist := benames[name]; exist {
			return failCheck("duplicate backend.name: %s", name)
		}
		benames[name] = struct{}{}
	}

	for _, fe := range hap.Frontends {
		if err := v2checkFrontend(fe); err != nil {
			return err
		}
	}

	for _, be := range hap.Backends {
		if err := v2checkBackend(be); err != nil {
			return err
		}
	}

	return frontendBackendCrossCheck(referencedBackends, benames)
}

func v2checkFrontend(fe *V2Frontend) error {
	if i := strings.IndexAny(fe.BindAddress, "*[]"); i != -1 {
		badChar := fe.BindAddress[i]
		return failCheck("frontend.bindAddress invalid character: %q", badChar)
	}

	if fe.Protocol == "" {
		return failString("frontend.protocol")
	}

	if err := v2checkCertificates(fe.Protocol, fe.Certificates); err != nil {
		return err
	}

	if fe.RedirectToHTTPS != nil {
		if fe.Protocol != V2ProtocolHTTP {
			s := "frontend.redirectToHttps cannot be set with frontend.protocol %s"
			return failCheck(s, fe.Protocol)
		}
	}

	for _, lbeMap := range fe.LinkBackend.Map {
		if lbeMap.Backend == "" {
			return failString("frontend.linkBackend.map.backend")
		}

		fakeMap := V2FrontendLinkBackendMapItems0{Backend: lbeMap.Backend}
		if *lbeMap == fakeMap {
			msg := "at least one of the condition fields must be filled out"
			return failCheck("frontend.linkBackend.map.backend: %s", msg)
		}
	}
	return nil
}

func v2checkCertificates(prot V2Protocol, certs []string) error {
	switch prot {
	case V2ProtocolHTTPS:
		fallthrough
	case V2ProtocolTLS:
		if len(certs) == 0 {
			s := "frontend.protocol %s must have non-empty frontend.certificates"
			return failCheck(s, prot)
		}
		for _, cert := range certs {
			if err := checkCertificate(cert); err != nil {
				return failCheck("frontend.certificates: %s", err)
			}
		}
	}
	return nil
}

func v2checkBackend(be *V2Backend) error {
	if be.Name == "" {
		return failString("backend.name")
	}
	if be.Protocol == "" {
		return failString("backend.protocol")
	}
	if err := v2checkRewriteHTTP(be.RewriteHTTP); err != nil {
		return err
	}
	if len(be.Services) == 0 {
		return failCheck("backend.services: must have at least 1")
	}
	for _, sv := range be.Services {
		if err := v2checkService(sv); err != nil {
			return err
		}
	}
	return nil
}

func v2checkRewriteHTTP(rh *V2RewriteHTTP) error {
	if rh == nil {
		return nil
	}
	if rh.Path == nil {
		return nil
	}
	fromEnding := getPathEnding(*rh.Path.FromPath)
	toEnding := getPathEnding(*rh.Path.ToPath)
	if fromEnding != toEnding {
		return failCheck("backend.rewriteHttp.path: "+
			"fromPath '%s' toPath '%s' ending mismatch: "+
			"either both end with '/' or neither end with '/'",
			*rh.Path.FromPath, *rh.Path.ToPath)
	}
	return nil
}

func v2checkService(sv *V2Service) error {
	if err := v2checkEndpoint(sv.Endpoint); err != nil {
		return err
	}
	if sv.Endpoint.Type == V2EndpointTypeADDRESS {
		return nil
	}
	marathonErr := V2CheckMarathonService(sv.Marathon)
	mesosErr := V2CheckMesosService(sv.Mesos)
	if marathonErr != nil && mesosErr != nil {
		return failCheck("server.marathon or server.mesos must be specified, %s, %s", marathonErr, mesosErr)
	}
	return nil
}

func v2checkEndpoint(ep *V2Endpoint) error {
	if ep.Type == V2EndpointTypeADDRESS {
		if ep.Address == "" {
			return failString("server.endpoint.address")
		}
		if ep.Port < 0 {
			s := "server.endpoint.type %s must have valid server.port specified"
			return failCheck(s, ep.Type)
		}
		return nil
	}
	if ep.PortName == "" && !ep.AllPorts {
		if portErr := checkPort("server.endpoint.port", ep.Port); portErr != nil {
			s := "server.endpoint.type %s must have valid port, portName, portNamePattern, or true allPorts"
			return failCheck(s, ep.Type)
		}
	}
	return nil
}
