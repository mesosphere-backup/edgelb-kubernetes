package edgelb

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	// Actor
	"github.com/AsynkronIT/protoactor-go/actor"

	//logging library
	"github.com/Sirupsen/logrus"

	// DC/OS dependencies
	"github.com/dcos/dcos-go/dcos/http/transport"
	sdkClient "github.com/mesosphere/dcos-commons/cli/client"
	sdkConfig "github.com/mesosphere/dcos-commons/cli/config"

	// Edge-lb dependencies
	"edgelb-k8s/pkg/lb/config"
	"edgelb-k8s/pkg/lb/messages"
	edgelbOperations "github.com/mesosphere/dcos-edge-lb/apiserver/client/operations"
	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/models"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
	edgelbClient "github.com/mesosphere/dcos-edge-lb/framework/edgelb/cli/dcos-edgelb/client"
)

// A client for `dcos-edge-lb` that implements the `lb.LoadBalancerBackend` interface.
type EdgeLB struct {
	k8sPool        *models.PoolContainer
	frontends      map[string]*models.V2FrontendLinkBackendMapItems0 // Keep track of VHosts that get added to the pool.
	backends       map[string]*models.V2Backend                      // Keep track of all the backends that got added to the pool.
	ingress        map[string]*models.V2Frontend                     // Keep track of the different frontend that this pool exposes.
	defaultIngress *models.V2Frontend
	mkClient       func() (*edgelbOperations.Client, error)
}

// hasContentTypes returns true if any item in cts is included in resp, else returns false
func hasContentTypes(resp *http.Response, cts []string) bool {
	respCt := resp.Header.Get("Content-Type")
	if respCt == "" {
		respCt = "application/octet-stream"
	}
	for _, ct := range cts {
		if strings.Contains(respCt, ct) {
			return true
		}
	}
	return false
}

// checkHTTPResponse wraps sdk's CheckHTTPResponse with content-type check
func checkHTTPResponse(resp *http.Response) (*http.Response, error) {
	// XXX: Need a better indicator for if a response is coming from edgelb or adminrouter
	adminRouterContentTypes := []string{
		"text/html",                // adminrouter auth
		"application/octet-stream", // adminrouter service unavailable
	}
	if hasContentTypes(resp, adminRouterContentTypes) {
		if _, err := sdkClient.CheckHTTPResponse(resp); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

type EdgeLBRoundTripper struct {
	rt http.RoundTripper
}

func (edgelbRT *EdgeLBRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	resp, err = edgelbRT.rt.RoundTrip(req)
	log.Printf("Finished a round trip to edge-lb")

	if err == nil {
		log.Printf("Checking response from edge-lb")
		return checkHTTPResponse(resp)
	} else {
		log.Printf("Edge LB round tripper errored out: %s", err)
	}

	return
}

func New(serviceName, dcosURL, secrets string) (elb *EdgeLB, err error) {
	// Setting up the global service name and DC/OS URL.
	sdkConfig.ServiceName = serviceName
	sdkConfig.DcosURL = dcosURL

	logger := util.Logger
	logger.SetLevel(logrus.DebugLevel)

	// Read the creds from the secret store.
	dat, err := ioutil.ReadFile(secrets)
	if err != nil {
		err = errors.New(fmt.Sprintf("Unable to get DC/OS service account credentials:%s", err))
		return
	}

	dcosCredsStr := string(dat)
	log.Printf("Retrieved the following encoded DC/OS credentials: %s", dcosCredsStr)

	// Decode the DC/OS service account credentials from the JSON
	dcosCreds := &dcos.AuthCreds{}
	if err = json.Unmarshal([]byte(dcosCredsStr), dcosCreds); err != nil {
		err = errors.New(fmt.Sprintf("Failed to decode dcos auth credentials. Error: %s", err))
		return
	}

	log.Printf("Decoded `dcosCreds`: %v", dcosCreds)

	// Setup the client configuration.
	httpClient := &http.Client{
		Transport: &http.Transport{},
	}

	// Setup HTTPS client.
	//
	// NOTE: We are setting the HTTPS client to authenticate the server against
	// a CA. This might be required for making this working DC/OS EE strict
	// mode.
	tlsConfig := &tls.Config{}
	tlsConfig.InsecureSkipVerify = true
	httpClient.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	// Set up the HTTP round-tripper to use the DC/OS credentials.
	creds := transport.OptionCredentials(dcosCreds.UID, dcosCreds.Secret, dcosCreds.LoginEndpoint)
	expire := transport.OptionTokenExpire(time.Minute * 10)
	rt, err := transport.NewRoundTripper(httpClient.Transport, creds, expire)
	if err != nil {
		err = errors.New(fmt.Sprintf("Failed to create HTTP client with configured service account: %s", err))
		return
	}

	mkClient := func() (elbOpsClient *edgelbOperations.Client, err error) {
		elbOpsClient, err = edgelbClient.NewWithRoundTripper(&EdgeLBRoundTripper{rt: rt})
		return
	}

	// Setup the closure for creating edge-lb clients.
	elb = &EdgeLB{mkClient: mkClient}

	// During initialization we want to make sure that this backend to talk to
	// the Edge-LB API server. We will also need to make sure that a k8s pool
	// exist on the Edge-lb, for this k8s cluster. If a k8s pool does not exist
	// we need to create it during initialization.

	// Initiate a ping to the edge-lb server.
	params := edgelbOperations.NewPingParams()

	elbClient, err := elb.mkClient()
	if err != nil {
		err = errors.New(fmt.Sprintf("Failed to create an Edge-LB client to initiate a ping: %s", err))
		return
	}

	resp, err := elbClient.Ping(params)
	if err != nil {
		err = errors.New(fmt.Sprintf("Failed to get a ping response from Edge-LB:%s", err))
		return
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
	k8sPool := elb.newK8sPool()

	poolParams := edgelbOperations.NewGetPoolContainerParams().
		WithName("k8s")
	_, err = elbClient.GetPoolContainer(poolParams)
	if err == nil {
		log.Printf("Found edge-lb pool for k8s: %v", resp.Payload)
		k8sExists = true
	}

	if !k8sExists {
		// The k8s pool does not exist so go ahead and create it.
		log.Printf("Could not find default k8s pool. Atempting to create one ....")
		params := edgelbOperations.NewV2CreatePoolParams().
			WithPool(k8sPool.V2)

		_, err = elbClient.V2CreatePool(params)

		if err != nil {
			err = errors.New(fmt.Sprintf("Failed to create k8s Edge-LB pool:%s", err))
			return
		}
		log.Printf("Successfully created k8s pool.")
	}

	return
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
		log.Printf("Undefined operation requested on EdgeLB backend")
	}
}

func (elb *EdgeLB) addBackendToPool(vhost *config.VHost, route *config.Route) *models.V2Backend {
	backend, ok := elb.backends[getBackendID(vhost, route)]
	if ok {
		return backend
	}

	// New `backend`.
	backend = &models.V2Backend{
		Name: getBackendID(vhost, route),
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

func (elb *EdgeLB) unconfigureVHost(vhost config.VHost) (err error) {
	return errors.New("Operation not supported")
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
		WithPool(poolContainer.V2)

	_, err = elbClient.V2UpdatePool(params)

	if err != nil {
		log.Fatalf("Unable to update the edge-lb pool during 'sync':%s", err)
	}

	return
}
