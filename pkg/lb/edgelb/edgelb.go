package edgelb

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	//logging library
	"github.com/Sirupsen/logrus"

	// DC/OS dependencies
	"github.com/dcos/dcos-go/dcos/http/transport"
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

	// Decode the DC/OS service account credentials from the JSON
	dcosCreds := &dcos.AuthCreds{}
	if err = json.Unmarshal([]byte(dcosCredsStr), dcosCreds); err != nil {
		err = errors.New(fmt.Sprintf("Failed to decode dcos auth credentials. Error: %s", err))
		return
	}

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
	}

	mkClient := func() (elbOpsClient *edgelbOperations.Client, err error) {
		elbOpsClient, err = edgelbClient.NewWithRoundTripper(rt)
		return
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

	params := edgelbOperations.NewV2CreatePoolParams().
		WithPool(poolContainer.V2)

	_, err = elbClient.V2CreatePool(params)

	return
}

func (elb *EdgeLB) UnconfigureVHost(vhost state.VHost) (err error) {
	return
}
