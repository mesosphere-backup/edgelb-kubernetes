package models

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// ConfigContainerToMixedBytes converts config container to the following format:
// {"pools": [{...V1Pool},{...V2Pool},...]}
func ConfigContainerToMixedBytes(config *ConfigContainer) ([]byte, error) {
	mixedConfig := make(map[string][]interface{})
	mixedConfig["pools"] = []interface{}{}
	for _, pool := range config.Pools {
		if pool.APIVersion == APIVersionV1 {
			mixedConfig["pools"] = append(mixedConfig["pools"], pool.V1)
		} else {
			mixedConfig["pools"] = append(mixedConfig["pools"], pool.V2)
		}
	}
	return json.Marshal(mixedConfig)
}

// ConfigContainerFromMixedBytes converts a structure like the following:
// {"pools": [{...V1Pool},{...V2Pool},...]} into a config container
func ConfigContainerFromMixedBytes(b []byte) (*ConfigContainer, error) {
	configContainer := ConfigContainer{
		Pools: []*PoolContainer{},
	}
	var mixedConfig map[string][]interface{}
	if err := json.Unmarshal(b, &mixedConfig); err != nil {
		return nil, err
	}
	mixedPools, ok := mixedConfig["pools"]
	if !ok {
		return &configContainer, nil
	}
	for _, mixedPool := range mixedPools {
		mixedPoolBytes, err := json.Marshal(mixedPool)
		if err != nil {
			return nil, err
		}
		var poolContainer PoolContainer
		if err = poolContainer.UnmarshalBinary(mixedPoolBytes); err != nil {
			return nil, err
		}
		if poolContainer.APIVersion == APIVersionV1 {
			var v1pool V1Pool
			if err = v1pool.UnmarshalBinary(mixedPoolBytes); err != nil {
				return nil, err
			}
			poolContainer.V1 = &v1pool

		} else {
			var v2pool V2Pool
			if err = v2pool.UnmarshalBinary(mixedPoolBytes); err != nil {
				return nil, err
			}
			poolContainer.V2 = &v2pool
		}
		configContainer.Pools = append(configContainer.Pools, &poolContainer)
	}
	return &configContainer, nil
}

// V2ConfigFromContainer Gets a V2 Config from a container, omitting V1 pools
func V2ConfigFromContainer(configContainer *ConfigContainer) *V2Config {
	var config V2Config
	config.Pools = []*V2Pool{}
	for _, p := range configContainer.Pools {
		if p.APIVersion == APIVersionV2 {
			config.Pools = append(config.Pools, p.V2)
		}
	}
	return &config
}

// V1ConfigFromContainer Gets a V1 Config from a container, omitting V2 pools
func V1ConfigFromContainer(configContainer *ConfigContainer) *V1Config {
	var config V1Config
	config.Pools = []*V1Pool{}
	for _, p := range configContainer.Pools {
		if p.APIVersion == APIVersionV1 {
			config.Pools = append(config.Pools, p.V1)
		}
	}
	return &config
}

// V2PoolFromContainer Gets a V2 Pool from a container, converting from V1 if necessary
func V2PoolFromContainer(poolContainer *PoolContainer) (*V2Pool, error) {
	var pool *V2Pool
	var err error
	if poolContainer.APIVersion == APIVersionV1 {
		pool, err = V2PoolFromV1(poolContainer.V1)
		if err != nil {
			return nil, err
		}
	} else {
		pool = poolContainer.V2
	}
	return pool, nil
}

// ContainerFromV1Config wraps a v1 config in a container
func ContainerFromV1Config(config *V1Config) *ConfigContainer {
	var configContainer ConfigContainer
	for _, p := range config.Pools {
		configContainer.Pools = append(configContainer.Pools, ContainerFromV1Pool(p))
	}
	return &configContainer
}

// ContainerFromV2Pool wraps a pool in a container
func ContainerFromV2Pool(pool *V2Pool) *PoolContainer {
	var poolContainer PoolContainer
	poolContainer.V2 = pool
	poolContainer.APIVersion = APIVersionV2
	poolContainer.Name = pool.Name
	poolContainer.Namespace = pool.Namespace
	return &poolContainer
}

// ContainerFromV1Pool wraps a v1 pool in a container
func ContainerFromV1Pool(pool *V1Pool) *PoolContainer {
	var poolContainer PoolContainer
	poolContainer.V1 = pool
	poolContainer.APIVersion = APIVersionV1
	poolContainer.Name = pool.Name
	poolContainer.Namespace = pool.Namespace
	return &poolContainer
}

// V2ServiceFromV1Server converts a v1 server to service
func V2ServiceFromV1Server(v1server *V1Server) (*V2Service, error) {
	var svc V2Service
	svc.UnmarshalBinary([]byte(`{}`))
	switch v1server.Type {
	case V1ServerTypeVIP:
		svc.Endpoint.Type = V2EndpointTypeADDRESS
		if matches, err := regexp.MatchString("^/.*", v1server.Port.Vip); matches && err == nil {
			// VIP syntax (/myvip:80)
			splitVip := strings.Split(v1server.Port.Vip, ":")
			svc.Endpoint.Address = fmt.Sprintf("%s.%s.l4lb.thisdcos.directory", strings.Replace(splitVip[0], "/", "", -1), v1server.Framework.Value)
			if len(splitVip) > 1 {
				// Port included (/myvip:80)
				if p, pErr := strconv.Atoi(splitVip[1]); pErr == nil {
					svc.Endpoint.Port = int32(p)
				}
			}
		} else if vipHost, vipPort, err := net.SplitHostPort(v1server.Port.Vip); err == nil {
			// Static VIP / address
			if vipHost != "" && vipPort != "" {
				svc.Endpoint.Address = vipHost
				if p, pErr := strconv.Atoi(vipPort); pErr == nil {
					svc.Endpoint.Port = int32(p)
				}
			} else if vipHost != "" {
				svc.Endpoint.Address = vipHost
			} else {
				svc.Endpoint.Address = v1server.Port.Vip
			}
		} else {
			// There was an error trying to parse, just assign the VIP to address
			svc.Endpoint.Address = v1server.Port.Vip
		}
	case V1ServerTypeAUTOIP:
		svc.Endpoint.Type = V2EndpointTypeAUTOIP
	case V1ServerTypeAGENTIP:
		svc.Endpoint.Type = V2EndpointTypeAGENTIP
	case V1ServerTypeCONTAINERIP:
		svc.Endpoint.Type = V2EndpointTypeCONTAINERIP
	}

	svc.Endpoint.MiscStr = v1server.MiscStr
	svc.Endpoint.Check.Enabled = v1server.Check.Enabled
	svc.Endpoint.Check.CustomStr = v1server.Check.CustomStr
	svc.Endpoint.PortName = v1server.Port.Name
	svc.Endpoint.AllPorts = v1server.Port.All

	if v1server.Framework.Match == V1MatchEXACT {
		svc.Mesos.FrameworkName = v1server.Framework.Value
	} else {
		svc.Mesos.FrameworkNamePattern = v1server.Framework.Value
	}
	if v1server.Task.Match == V1MatchEXACT {
		svc.Mesos.TaskName = v1server.Task.Value
	} else {
		svc.Mesos.TaskNamePattern = v1server.Task.Value
	}
	return &svc, nil
}

// V2PoolFromV1 Converts a V1Pool model to V2Pool
func V2PoolFromV1(v1pool *V1Pool) (*V2Pool, error) {
	v1poolBytes, err := v1pool.MarshalBinary()
	if err != nil {
		return nil, err
	}
	var pool V2Pool
	err = pool.UnmarshalBinary(v1poolBytes)
	if err != nil {
		return nil, err
	}
	services := make(map[string][]*V2Service)
	for _, v1be := range v1pool.Haproxy.Backends {
		services[v1be.Name] = []*V2Service{}
		for _, v1sv := range v1be.Servers {
			if svc, svcErr := V2ServiceFromV1Server(v1sv); svcErr == nil {
				services[v1be.Name] = append(services[v1be.Name], svc)
			}
		}
	}
	for _, be := range pool.Haproxy.Backends {
		be.Services = services[be.Name]
	}
	return &pool, nil
}

// MesosTaskIDToMarathonServiceIDContainerName converts a mesos task name to a marathon service id
// Used for extracting marathon based serviceID and containerNames from a taskID
// For pods, the structure is "<service-id>.instance-<uuid>.<container-name>"
// For apps, the structure is "<service-id>.<uuid>"
// Example Pod task id: "pod-overlay-mesos-id-unused-podname.instance-b5407709-df83-11e7-b5eb-4e2c5443c397.pod-overlay-mesos-id"
// Example App task id: "foo-svc.8cb5a16a-df84-11e7-b5eb-4e2c5443c397"
// Example Pod output: "group1_group2_appID.instance-UUID.container-name" -> "/group1/group2/appID", "container-name"
// Example App output: "group1_group2_appID.UUID" -> "/group1/group2/appID", ""
func MesosTaskIDToMarathonServiceIDContainerName(taskID string) (string, string) {
	parts := strings.Split(taskID, ".")
	serviceID := ""
	containerName := ""
	if len(parts) < 2 {
		return serviceID, containerName
	}
	if len(parts) == 3 {
		containerName = parts[2]
	}
	serviceIDParts := strings.Split(parts[0], "_")
	serviceID = "/" + strings.Join(serviceIDParts, "/")
	return serviceID, containerName
}
