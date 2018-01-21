package models

import (
	"fmt"
	"strings"
)

// V1CheckConfig validates a v1 config
func V1CheckConfig(cfg *V1Config) error {
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
		if err := V1CheckPool(pool); err != nil {
			return err
		}
	}
	return nil
}

// V1CheckPool validates a v1 pool
func V1CheckPool(pool *V1Pool) error {
	if pool.Name == "" {
		return failString("pool.name")
	}
	if err := checkName(pool.Name); err != nil {
		return failCheck("pool.name: %s", err)
	}
	if err := checkNamespace(*pool.Namespace); err != nil {
		return failCheck("pool.namespace: %s", err)
	}
	if err := v1checkPoolBindPorts(pool); err != nil {
		return err
	}
	if err := v1checkPoolSecrets(pool); err != nil {
		return err
	}
	if err := v1checkPoolVirtualNetworks(pool); err != nil {
		return err
	}
	return v1checkHaproxy(pool.Haproxy)
}

// V1PoolBindPorts returns all bind ports for a v1 pool
func V1PoolBindPorts(pool *V1Pool) []int32 {
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

func v1checkPoolBindPorts(pool *V1Pool) error {
	ports := V1PoolBindPorts(pool)
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

func v1checkPoolSecrets(pool *V1Pool) error {
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

func v1checkPoolVirtualNetworks(pool *V1Pool) error {
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

func v1checkHaproxy(hap *V1Haproxy) error {
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
		if err := v1checkFrontend(fe); err != nil {
			return err
		}
	}

	for _, be := range hap.Backends {
		if err := v1checkBackend(be); err != nil {
			return err
		}
	}

	return frontendBackendCrossCheck(referencedBackends, benames)
}

func v1checkFrontend(fe *V1Frontend) error {
	if i := strings.IndexAny(fe.BindAddress, "*V1[]"); i != -1 {
		badChar := fe.BindAddress[i]
		return failCheck("frontend.bindAddress invalid character: %q", badChar)
	}

	if fe.Protocol == "" {
		return failString("frontend.protocol")
	}

	if err := v1checkCertificates(fe.Protocol, fe.Certificates); err != nil {
		return err
	}

	if fe.RedirectToHTTPS != nil {
		if fe.Protocol != V1ProtocolHTTP {
			s := "frontend.redirectToHttps cannot be set with frontend.protocol %s"
			return failCheck(s, fe.Protocol)
		}
	}

	for _, lbeMap := range fe.LinkBackend.Map {
		if lbeMap.Backend == "" {
			return failString("frontend.linkBackend.map.backend")
		}

		fakeMap := V1FrontendLinkBackendMapItems0{Backend: lbeMap.Backend}
		if *lbeMap == fakeMap {
			msg := "at least one of the condition fields must be filled out"
			return failCheck("frontend.linkBackend.map.backend: %s", msg)
		}
	}
	return nil
}

func v1checkCertificates(prot V1Protocol, certs []string) error {
	switch prot {
	case V1ProtocolHTTPS:
		fallthrough
	case V1ProtocolTLS:
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

func v1checkBackend(be *V1Backend) error {
	if be.Name == "" {
		return failString("backend.name")
	}
	if be.Protocol == "" {
		return failString("backend.protocol")
	}
	if err := v1checkRewriteHTTP(be.RewriteHTTP); err != nil {
		return err
	}
	if len(be.Servers) == 0 {
		return failCheck("backend.servers: must have at least 1")
	}
	for _, sv := range be.Servers {
		if err := v1checkServer(sv); err != nil {
			return err
		}
	}
	return nil
}

func v1checkRewriteHTTP(rh *V1RewriteHTTP) error {
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

func v1checkServer(sv *V1Server) error {
	if sv.Framework.Value == "" {
		return failString("server.framework.value")
	}
	if sv.Type != V1ServerTypeVIP && sv.Task.Value == "" {
		return failString("server.task.value")
	}
	if err := v1checkServerPort(sv.Port, sv.Type); err != nil {
		return err
	}
	return nil
}

func v1checkServerPort(sp *V1ServerPort, svType V1ServerType) error {
	fakeSp := V1ServerPort{}
	if *sp == fakeSp {
		msg := "at least one of the fields must be filled out"
		return failCheck("server.port: %s", msg)
	}

	switch svType {
	case V1ServerTypeAGENTIP:
		fallthrough
	case V1ServerTypeCONTAINERIP:
		if !(sp.Name != "" || sp.All) {
			s := "server.type %s must have non-empty port.name or true port.all"
			return failCheck(s, svType)
		}
	case V1ServerTypeVIP:
		if sp.Vip == "" {
			s := "server.type %s must have non-empty port.vip"
			return failCheck(s, svType)
		}
	}
	return nil
}
