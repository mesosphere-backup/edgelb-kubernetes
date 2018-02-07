package edgelb

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	//logging library
	"github.com/Sirupsen/logrus"

	// DC/OS dependencies
	"github.com/dcos/dcos-go/dcos/http/transport"
	sdkClient "github.com/mesosphere/dcos-commons/cli/client"
	"github.com/mesosphere/dcos-commons/cli/config"

	// Edge-lb dependencies
	"edgelb-k8s/pkg/lb/state"
	edgelbOperations "github.com/mesosphere/dcos-edge-lb/apiserver/client/operations"
	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/models"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
	edgelbClient "github.com/mesosphere/dcos-edge-lb/framework/edgelb/cli/dcos-edgelb/client"
)

// A client for `dcos-edge-lb` that implements the `lb.LoadBalancerBackend` interface.
type EdgeLB struct {
	mkClient func() (*edgelbOperations.Client, error)
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
	config.ServiceName = serviceName
	config.DcosURL = dcosURL

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

	// Initiate a ping to the edge-lb server.
	params := edgelbOperations.NewPingParams()

	elbClient, err := mkClient()
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
	k8sPool := false
	poolParams := edgelbOperations.NewGetPoolContainerParams().
		WithName("k8s")
	_, err = elbClient.GetPoolContainer(poolParams)
	if err == nil {
		log.Printf("Found edge-lb pool for k8s: %v", resp.Payload)
		k8sPool = true
	}

	if !k8sPool {
		log.Printf("Could find default k8s pool. Atempting to create one ....")
		// Create the k8s edge-lb pool.
		poolContainer := models.PoolContainer{APIVersion: models.APIVersionV2, V2: &models.V2Pool{Name: "k8s"}}

		var backends []*models.V2Backend
		var frontEndMap []*models.V2FrontendLinkBackendMapItems0

		frontEnd := models.V2FrontendLinkBackendMapItems0{
			Backend: "dummy",
			HostEq:  "k8s.edgelb",
			PathBeg: "/"}

		frontEndMap = append(frontEndMap, &frontEnd)

		v2Backend := models.V2Backend{Name: "dummy"}
		backends = append(backends, &v2Backend)

		endpoint := models.V2Endpoint{Address: "127.0.0.1"}
		endpoint.Port = 443

		service := models.V2Service{Endpoint: &endpoint}
		v2Backend.Services = append(v2Backend.Services, &service)
		v2Backend.Protocol = models.V2ProtocolHTTP

		// Setup the frontend for the pool
		frontendLink := models.V2FrontendLinkBackend{
			Map: frontEndMap,
		}

		bindPort := int32(8080)
		frontend := models.V2Frontend{
			BindPort:    &bindPort,
			LinkBackend: &frontendLink,
			Protocol:    models.V2ProtocolHTTP,
		}

		var frontends []*models.V2Frontend
		frontends = append(frontends, &frontend)

		haProxy := models.V2Haproxy{
			Backends:  backends,
			Frontends: frontends,
		}

		// Store the HAProxy config in the pool container.
		poolContainer.V2.Haproxy = &haProxy
		params := edgelbOperations.NewV2CreatePoolParams().
			WithPool(poolContainer.V2)

		_, err = elbClient.V2CreatePool(params)

		if err != nil {
			err = errors.New(fmt.Sprintf("Failed to create k8s Edge-LB pool:%s", err))
			return
		}
	}

	// Setup the closure for creating edge-lb clients.
	elb = &EdgeLB{mkClient: mkClient}
	return
}

func (elb *EdgeLB) ConfigureVHost(vhost state.VHost) (err error) {
	// Create the edge-LB client.
	elbClient, err := elb.mkClient()

	if err != nil {
		return
	}

	// Create an edge-lb specific pool config.
	poolContainer := models.PoolContainer{APIVersion: models.APIVersionV2, Name: "k8s", V2: &models.V2Pool{}}

	// Setup the backend for this pool.
	var backends []*models.V2Backend
	var frontEndMap []*models.V2FrontendLinkBackendMapItems0

	for path, route := range vhost.Routes {
		v2Backend := models.V2Backend{Name: path}
		frontEnd := models.V2FrontendLinkBackendMapItems0{
			Backend: v2Backend.Name,
			HostEq:  vhost.Host,
			PathBeg: path}

		frontEndMap = append(frontEndMap, &frontEnd)

		for _, backend := range route.Backends {
			endpoint := models.V2Endpoint{Address: backend.Address}
			service := models.V2Service{Endpoint: &endpoint}
			v2Backend.Services = append(v2Backend.Services, &service)
		}
		backends = append(backends, &v2Backend)
	}

	// Setup the frontend for the pool
	frontendLink := models.V2FrontendLinkBackend{
		Map: frontEndMap,
	}

	frontend := models.V2Frontend{
		LinkBackend: &frontendLink,
	}

	var frontends []*models.V2Frontend
	frontends = append(frontends, &frontend)

	haProxy := models.V2Haproxy{
		Backends:  backends,
		Frontends: frontends,
	}

	// Store the HAProxy config in the pool container.
	poolContainer.V2.Haproxy = &haProxy

	params := edgelbOperations.NewV2UpdatePoolParams().
		WithPool(poolContainer.V2)

	_, err = elbClient.V2UpdatePool(params)

	return
}

func (elb *EdgeLB) UnconfigureVHost(vhost state.VHost) (err error) {
	return
}
