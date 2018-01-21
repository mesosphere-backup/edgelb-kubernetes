package dcos

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/dcos/dcos-go/dcos/http/transport"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
)

const (
	defaultHTTPBackoff       = 100 * time.Millisecond
	defaultHTTPBackoffFactor = 2
	defaultHTTPBackoffMax    = 5 * time.Second
	defaultTokenExpiration   = time.Minute * 10
)

// Client info for DCOS requests
type Client interface {
	WithURL(string) Client
	WithToken(*string) Client
	WithAuth(*AuthCreds) Client
	CreateRequest(string, string, string) (*http.Request, error)
	HTTPExecute(context.Context, func() (*http.Request, error), func(*http.Response) bool) (*http.Response, error)
}

// dcosClient implementation of Client
type dcosClient struct {
	backoff       time.Duration
	backoffMax    time.Duration
	backoffFactor int
	url           string
	auth          *AuthCreds
	token         *string
}

// AuthCreds info for DCOS authentication
type AuthCreds struct {
	UID           string `json:"uid"`
	Secret        string `json:"private_key"`
	LoginEndpoint string `json:"login_endpoint"`
}

// MakeClientFn creates a function that returns a new client with auth credentials configured
func MakeClientFn(authCredsStr, dcosAddr, dcosProt string) func() Client {
	var authCreds *AuthCreds
	if authCredsStr != "" {
		authCreds = &AuthCreds{}
		if err := json.Unmarshal([]byte(authCredsStr), authCreds); err != nil {
			logger.Warnf("Failed to decode dcos auth credentials. Error: %+v", err)
		}
	} else {
		logger.Info("No service account credentials found, continuing without authentication")
	}
	dcosURL := fmt.Sprintf("%s://%s", dcosProt, dcosAddr)
	return func() Client {
		return NewClient(authCreds, dcosURL)
	}
}

// NewClient creates a new dcos client
func NewClient(authCreds *AuthCreds, dcosURL string) Client {
	c := &dcosClient{
		backoff:       defaultHTTPBackoff,
		backoffMax:    defaultHTTPBackoffMax,
		backoffFactor: defaultHTTPBackoffFactor,
		url:           dcosURL,
		auth:          authCreds,
		token:         nil,
	}
	if c.auth != nil {
		u, _ := url.Parse(c.auth.LoginEndpoint)
		u.Path = ""
		dcosURL := u.String()
		c.url = dcosURL
	}
	return c
}

// WithURL overrides the URL
func (c *dcosClient) WithURL(url string) Client {
	c.url = url
	return c
}

// WithToken overrides the configured service credentials
func (c *dcosClient) WithToken(token *string) Client {
	c.token = token
	return c
}

// WithAuth overrides the configured service credentials
func (c *dcosClient) WithAuth(authCreds *AuthCreds) Client {
	c.auth = authCreds
	return c
}

// HTTPExecute creates, executes, and handles the response for a request
// It takes a function that creates HTTP requests instead of the request
// itself because we retry internally and the body of the request is
// consumed each time you retry so you need to reconstruct the request
// each time.
func (c *dcosClient) HTTPExecute(ctx context.Context, mkReq func() (*http.Request, error), shouldRetry func(*http.Response) bool) (*http.Response, error) {
	backoff := c.backoff
	for {
		req, reqErr := mkReq()
		if reqErr != nil {
			return nil, fmt.Errorf("could not make request: %s", reqErr)
		}
		// Pass body on upon success, no need to close.
		resp, respErr := c.doRequest(ctx, req, shouldRetry)
		if respErr == nil {
			return resp, nil
		}
		logger.Debugf("HTTPExecute failed (%+v), retrying...", respErr)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("HTTPExecute context cancelled before successful request, last error: %+v", respErr)
		case <-time.After(backoff):
			backoff *= time.Duration(c.backoffFactor)
			if backoff >= c.backoffMax {
				backoff = c.backoffMax
			}
		}
	}
}

// CreateRequest creates a request
func (c *dcosClient) CreateRequest(method, path, payload string) (*http.Request, error) {
	url, _ := url.Parse(fmt.Sprintf("%s/%s", c.url, path))
	request, err := http.NewRequest(method, url.String(), bytes.NewReader([]byte(payload)))
	if err != nil {
		return nil, fmt.Errorf("Failed to create HTTP %s request for %s: %s", method, url, err)
	}
	if c.token != nil {
		request.Header.Set("Authorization", fmt.Sprintf("token=%s", *c.token))
	}
	return request, nil
}

// NOOPRetry is a retry function that never retries
func NOOPRetry(resp *http.Response) bool {
	return false
}

// Internal
func (c *dcosClient) doRequest(ctx context.Context, req *http.Request, shouldRetry func(*http.Response) bool) (*http.Response, error) {
	logger.Debugf("(dcos) client request: Method:%s URL:%s", req.Method, req.URL)

	// Close body when request succeeds and we retry
	resp, respErr := c.httpQuery(ctx, req)
	if respErr != nil {
		return nil, fmt.Errorf("(dcos) client resp error, will retry: %s", respErr)
	}

	if !shouldRetry(resp) {
		logger.Debugf("(dcos) client resp, will not retry: %s", util.DebugResp(resp))
		return resp, nil
	}

	msg := fmt.Sprintf("(dcos) client resp, will retry: %s", util.ModDebugResp(resp))
	return nil, fmt.Errorf(util.MsgWithClose(msg, resp.Body))
}

func (c *dcosClient) httpQuery(ctx context.Context, request *http.Request) (*http.Response, error) {
	httpClient, clientErr := c.httpClient()
	if clientErr != nil {
		return nil, clientErr
	}

	resp, httpErr := httpClient.Do(request.WithContext(ctx))
	switch parsedHTTPErr := httpErr.(type) {
	case *url.Error:
		urlErr := parsedHTTPErr.Err
		switch urlErr.(type) {
		case x509.UnknownAuthorityError:
			msg := "Encountered x509.UnknownAuthorityError error while trying to reach %s: %s "
			return nil, fmt.Errorf(msg, request.URL, urlErr)
		default:
			msg := "Encountered error while trying to reach %s: %s"
			return nil, fmt.Errorf(msg, request.URL, urlErr)
		}
	}
	return resp, nil
}

func (c *dcosClient) httpClient() (*http.Client, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{},
	}
	// Configure HTTPS options
	url, _ := url.Parse(c.url)
	if url.Scheme == "https" {
		// Setup HTTPS client
		tlsConfig := &tls.Config{}
		// Load CA cert
		caCert, err := getCACert()
		if err != nil {
			return nil, err
		}
		// Use the cert if it exists, otherwise use InsecureSkipVerify
		if caCert != nil {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig.RootCAs = caCertPool
			tlsConfig.BuildNameToCertificate()
		} else {
			tlsConfig.InsecureSkipVerify = true
		}
		httpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}
	// Authenticate with service account if present
	if c.auth != nil {
		creds := transport.OptionCredentials(c.auth.UID, c.auth.Secret, c.auth.LoginEndpoint)
		expire := transport.OptionTokenExpire(defaultTokenExpiration)
		rt, err := transport.NewRoundTripper(httpClient.Transport, creds, expire)
		if err != nil {
			return nil, fmt.Errorf("Failed to create HTTP client with configured service account")
		}
		httpClient.Transport = rt
	}
	return httpClient, nil
}

// getCACert attempts to read the CA cert bundle from disk, returns false if it doesn't exist
func getCACert() ([]byte, error) {
	location := "/dcosfiles/dcos-ca.crt"
	if _, err := os.Stat(location); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	caCert, err := ioutil.ReadFile(location)
	if err != nil {
		return nil, err
	}
	return caCert, nil
}
