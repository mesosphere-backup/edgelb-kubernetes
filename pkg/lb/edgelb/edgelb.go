// Copyright (c) 2018 Mesosphere, Inc
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package edgelb

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/AsynkronIT/protoactor-go/actor"
	log "github.com/Sirupsen/logrus"

	edgelbOperations "github.com/mesosphere/dcos-edge-lb/client/operations"
	"github.com/mesosphere/dcos-edge-lb/dcos"
	"github.com/mesosphere/dcos-edge-lb/models"

	"github.com/mesosphere/edgelb-kubernetes/pkg/lb/config"
	"github.com/mesosphere/edgelb-kubernetes/pkg/lb/messages"
)

// A client for `dcos-edge-lb` that implements the `lb.LoadBalancerBackend` interface.
type EdgeLB struct {
	k8sPool        *models.PoolContainer
	frontends      map[string]*models.V2FrontendLinkBackendMapItems0 // Keep track of VHosts that get added to the pool.
	backends       map[string]*models.V2Backend                      // Keep track of all the backends that got added to the pool.
	ingress        map[string]*models.V2Frontend                     // Keep track of the different frontend that this pool exposes.
	defaultIngress *models.V2Frontend
	mkClient       func() (*dcos.APIServerClient, error)
}

func New() (*EdgeLB, error) {
	// TODO rework auth logic to support open
	mkDcosClient, err := dcos.MakeClientFn("DCOS_SERVICE_ACCOUNT_CREDENTIAL", "leader.mesos", "https")
	if err != nil {
		log.Fatalf("error making dcos client: %s", err)
	}
	mkClient := dcos.MakeAPIServerClientFn(mkDcosClient)

	// Setup the closure for creating edge-lb clients.
	edgeLB := &EdgeLB{
		mkClient:  mkClient,
		frontends: make(map[string]*models.V2FrontendLinkBackendMapItems0),
		backends:  make(map[string]*models.V2Backend),
		ingress:   make(map[string]*models.V2Frontend),
	}

	client, err := edgeLB.mkClient();
	if err != nil {
		return nil, err
	}

	params := edgelbOperations.NewPingParams()
	resp, err := client.Operations.Ping(params);
	if err != nil {
		err = errors.New(fmt.Sprintf("Failed to get a ping response from Edge-LB:%s", err))
		return nil, err
	}

	log.Printf("Edge-lb responded:%s", resp.Payload)

	// Check if the `k8s` pool already exists?
	k8sExists := false

	// We always initialize the k8s edge-lb pool to the default values
	// even if the pool and its config exist in the Edge-LB API server. This is
	// because during initialization, the controller will inform all the
	// ingress resources that exist in the k8s API server, with which we will
	// end up generating a new config and will simply over-write the existing
	// configuration for the k8s pool in the API server.
	k8sPool := edgeLB.newK8sPool()

	poolParams := edgelbOperations.NewGetPoolContainerParams().
		WithName("k8s")
	_, err = client.Operations.GetPoolContainer(poolParams)
	if err == nil {
		log.Printf("Found edge-lb pool for k8s: %v", resp.Payload)
		k8sExists = true
	}

	if !k8sExists {
		// The k8s pool does not exist so go ahead and create it.
		log.Printf("Could not find default k8s pool. Atempting to create one ....")
		params := edgelbOperations.NewV2CreatePoolParams().
			WithPool(k8sPool.V2)

		_, err = client.Operations.V2CreatePool(params)

		if err != nil {
			err = errors.New(fmt.Sprintf("Failed to create k8s Edge-LB pool:%s", err))
			return nil, err
		}
		log.Printf("Successfully created k8s pool.")
	}

	return edgeLB, nil
}

func getBackendID(vhost *config.VHost, route *config.Route) string {
	var host string
	if vhost.Host == "" {
		host = "0.0.0.0"
	} else {
		host = vhost.Host
	}
	return fmt.Sprintf("%s/%s", host, route.Path)
}

// There is a fundamental assumption here that there exists a
// 1-to-1 mapping between the frontend and the backend.
// Therefore if we end up with multiple frontends (VHost)
// serving the same backend (set of endpoints), it would just result in us
// maintaining duplicate information about the backends.
func getFrontendID(vhost *config.VHost, route *config.Route) string {
	return getBackendID(vhost, route)
}

func (elb *EdgeLB) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *messages.SyncMsg:
		elb.sync()
		log.Printf("Syncing with the edgelb API server.")
	case *messages.RemoveVHostMsg:
		removeVHostMsg, _ := ctx.Message().(*messages.RemoveVHostMsg)
		// Reset the Edge-LB configuration.
		elb.reset()

		log.Printf("Re-configuring VHosts in the k8s pool.")
		for _, vhost := range removeVHostMsg.VHosts {
			elb.configureVHost(vhost)
		}
	case *messages.ConfigVHostsMsg:
		configVHostsMsg, _ := ctx.Message().(*messages.ConfigVHostsMsg)
		log.Printf("Configuring VHosts in the k8s pool.")
		for _, vhost := range configVHostsMsg.VHosts {
			elb.configureVHost(vhost)
		}
	case *messages.ConfigVHostMsg:
		configVHostMsg, _ := ctx.Message().(*messages.ConfigVHostMsg)
		log.Printf("Configuring a VHost as part of the k8s pool.")
		elb.configureVHost(configVHostMsg.VHost)
	default:
		log.Printf("Undefined message type for EdgeLB backend: %T", ctx.Message())
	}
}

func (elb *EdgeLB) reset() {
	elb.newK8sPool()
	elb.frontends = make(map[string]*models.V2FrontendLinkBackendMapItems0)
	elb.backends = make(map[string]*models.V2Backend)
	elb.ingress = make(map[string]*models.V2Frontend)
}

func (elb *EdgeLB) addBackendToPool(vhost *config.VHost, route *config.Route) *models.V2Backend {
	backend, ok := elb.backends[getBackendID(vhost, route)]
	if ok {
		return backend
	}

	// New `backend`.
	backend = &models.V2Backend{
		Name:     getBackendID(vhost, route),
		Protocol: models.V2ProtocolHTTP,
	}

	// Keep track of it in Edge-LB.
	elb.backends[backend.Name] = backend

	// Add it to the pool configuration.
	elb.k8sPool.V2.Haproxy.Backends = append(elb.k8sPool.V2.Haproxy.Backends, backend)

	return backend
}

func (elb *EdgeLB) addFrontendToPool(backend *models.V2Backend,
	vhost *config.VHost,
	route *config.Route) *models.V2FrontendLinkBackendMapItems0 {
	frontend, ok := elb.frontends[getFrontendID(vhost, route)]
	if ok {
		return frontend
	}

	frontend = &models.V2FrontendLinkBackendMapItems0{
		Backend: backend.Name,
		HostEq:  vhost.Host,
		PathBeg: route.Path}

	// Update the Edge-lb cache with this frontend.
	elb.frontends[getFrontendID(vhost, route)] = frontend

	var ingress *models.V2Frontend
	if vhost.Frontend == (config.Ingress{}) {
		ingress = elb.defaultIngress
	} else {
		panic("Unknown ingress")
	}

	// Update the ingress information in the pool config.
	ingress.LinkBackend.Map = append(ingress.LinkBackend.Map, frontend)

	return frontend
}

func (elb *EdgeLB) newK8sPool() *models.PoolContainer {
	// Create the k8s edge-lb pool.
	k8sPool := &models.PoolContainer{APIVersion: models.APIVersionV2, V2: &models.V2Pool{Name: "k8s"}}

	var backends []*models.V2Backend
	var frontEndMap []*models.V2FrontendLinkBackendMapItems0

	frontEnd := models.V2FrontendLinkBackendMapItems0{
		Backend: "dummy",
		HostEq:  "k8s.edgelb",
		PathBeg: "/"}

	frontEndMap = append(frontEndMap, &frontEnd)

	v2Backend := models.V2Backend{Name: "dummy"}
	backends = append(backends, &v2Backend)

	endpoint := models.V2Endpoint{Address: "127.0.0.1", Port: 443, Type: "ADDRESS"}

	service := models.V2Service{Endpoint: &endpoint}
	v2Backend.Services = append(v2Backend.Services, &service)
	v2Backend.Protocol = models.V2ProtocolHTTP

	// Setup the frontend for the pool
	frontendLink := models.V2FrontendLinkBackend{
		Map: frontEndMap,
	}

	bindPort := int32(8080)
	frontend := &models.V2Frontend{
		BindPort:    &bindPort,
		LinkBackend: &frontendLink,
		Protocol:    models.V2ProtocolHTTP,
	}

	// Set the above as the default ingress for the time being.
	elb.defaultIngress = frontend

	var frontends []*models.V2Frontend
	frontends = append(frontends, frontend)

	haProxy := models.V2Haproxy{
		Backends:  backends,
		Frontends: frontends,
	}

	// Store the HAProxy config in the pool container.
	k8sPool.V2.Haproxy = &haProxy

	elb.k8sPool = k8sPool

	return k8sPool
}

func (elb *EdgeLB) configureVHost(vhost config.VHost) (err error) {

	// Create a backend for each route supported by the VHost.
	for _, route := range vhost.Routes {
		backend := elb.addBackendToPool(&vhost, &route)
		// Reset the backend services since we are going to learn them all over here.
		backend.Services = nil

		// Make sure to add the frontend for this backend.
		elb.addFrontendToPool(backend, &vhost, &route)
		for _, instance := range route.Service.Endpoints {
			ip, port, err := net.SplitHostPort(instance)
			if err != nil {
				err = errors.New(fmt.Sprintf("Unable to decode the endpoint: %s, with error:%s",
					instance,
					err))

				return err
			}

			endpoint := models.V2Endpoint{Address: ip, Type: "ADDRESS"}
			iPort, err := strconv.Atoi(port)
			if err != nil {
				err = errors.New(
					fmt.Sprintf("Unable set the endpoint port: %s, for instance: %s, with error:%s",
						port,
						instance,
						err))

				return err
			}
			endpoint.Port = int32(iPort)

			service := models.V2Service{Endpoint: &endpoint}
			backend.Services = append(backend.Services, &service)
		}
	}

	return
}

func (elb *EdgeLB) sync() {
	// Create the edge-LB client.
	elbClient, err := elb.mkClient()
	if err != nil {
		log.Fatalf("Unable to create the edge-lb client during 'sync':%s", err)
		return
	}

	// Create an edge-lb specific pool config.
	poolContainer := elb.k8sPool
	params := edgelbOperations.NewV2UpdatePoolParams().
		WithName(poolContainer.V2.Name).
		WithPool(poolContainer.V2)

	_, err = elbClient.Operations.V2UpdatePool(params)

	if err != nil {
		log.Fatalf("Unable to update the edge-lb pool during 'sync':%s", err)
	}

	return
}
