package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	httptransport "github.com/go-openapi/runtime/client"
	strfmt "github.com/go-openapi/strfmt"
	sdkClient "github.com/mesosphere/dcos-commons/cli/client"
	"github.com/mesosphere/dcos-commons/cli/config"
	edgelbClient "github.com/mesosphere/dcos-edge-lb/apiserver/client"
	edgelbOperations "github.com/mesosphere/dcos-edge-lb/apiserver/client/operations"
)

// GetToken wraps the sdk config DcosAuthToken
func GetToken() string {
	token := config.DcosAuthToken
	if token == "" {
		// if the token wasn't manually provided by the user, try to fetch it from the main CLI.
		// this value is optional: clusters can be configured to not require any auth
		return sdkClient.OptionalCLIConfigValue("core.dcos_acs_token")
	}
	return token
}

// PrintJSONError takes a payload and error that may contain json to print. Returns the error
func PrintJSONError(payload interface{}, err error) error {
	if err != nil {
		if printErr := PrintJSON(err); printErr != nil {
			return printErr
		}
		return err
	}
	return PrintJSON(payload)
}

// PrintJSON takes an interface that may contain json and prints it
func PrintJSON(payload interface{}) error {
	b, err := GetJSONBytes(payload)
	if err != nil {
		return err
	}
	sdkClient.PrintJSONBytes(b)
	return nil
}

// GetJSONBytes attempts to extract a .Payload, but defaults to returning the marshaled interface
func GetJSONBytes(payload interface{}) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error creating JSON from response: %s, response: %v", err, payload)
	}

	j := map[string]interface{}{}
	if err := json.Unmarshal(b, &j); err != nil {
		return b, nil
	}
	v, ok := j["Payload"]
	if !ok {
		return b, nil
	}
	switch v := v.(type) {
	case string:
		return []byte(v), nil
	}
	return GetJSONBytes(v)
}

// New configures and returns an apiserver operations client
func New() (*edgelbOperations.Client, error) {
	defaultResponseHandler := func(resp *http.Response) (*http.Response, error) {
		return checkHTTPResponse(resp)
	}
	return NewWithResponseHandler(defaultResponseHandler)
}

// NewWithResponseHandler configures and returns an apiserver operations client with a response handler func
func NewWithResponseHandler(responseHandler func(*http.Response) (*http.Response, error)) (*edgelbOperations.Client, error) {
	token := GetToken()
	var roundTripper http.RoundTripper = &dcosRoundtripper{
		token:           &token,
		checkResponse:   false,
		responseHandler: responseHandler,
	}
	return NewWithRoundTripper(roundTripper)
}

// NewWithRoundTripper configures and returns an apiserver operations client with a roundtripper
func NewWithRoundTripper(roundTripper http.RoundTripper) (*edgelbOperations.Client, error) {
	if len(config.DcosURL) == 0 {
		config.DcosURL = sdkClient.RequiredCLIConfigValue(
			"core.dcos_url",
			"DC/OS Cluster URL",
			"Run 'dcos config set core.dcos_url http://your-cluster.com' to configure.")
	}

	// Remove eg "/#/" and prefix from copy-pasted Dashboard URL:
	scheme, dcosHost, basePath, err := makeEdgelbURL(config.DcosURL, config.ServiceName)

	if err != nil {
		return nil, err
	}

	schemes := []string{scheme}

	transport := httptransport.New(dcosHost, basePath, schemes)
	transport.Transport = roundTripper
	return edgelbClient.New(transport, strfmt.Default).Operations, nil
}

// makeEdgelbURL returns the url pieces necessary to make a httptransport
func makeEdgelbURL(dcosURL, serviceName string) (string, string, string, error) {
	parsedURL, err := url.Parse(dcosURL)
	if err != nil {
		return "", "", "", err
	}

	if parsedURL.Scheme == "" && parsedURL.Host == "" {
		// Add default scheme of http if we have a URL like "host.com/"
		return makeEdgelbURL(fmt.Sprintf("http://%s", dcosURL), serviceName)
	} else if parsedURL.Scheme == "" {
		// Set default scheme of http if we can't find one
		parsedURL.Scheme = "http"
	}

	basePath := path.Join("service", serviceName, edgelbClient.DefaultBasePath)

	return parsedURL.Scheme, parsedURL.Host, basePath, nil
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

type dcosRoundtripper struct {
	token           *string
	checkResponse   bool
	responseHandler func(*http.Response) (*http.Response, error)
}

func (t *dcosRoundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(*t.token) != 0 {
		req.Header.Set("Authorization", fmt.Sprintf("token=%s", *t.token))
	}
	resp := sdkClient.HTTPQuery(req)
	return t.responseHandler(resp)
}
