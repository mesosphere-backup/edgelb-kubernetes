package dcos

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
)

// MarathonClient contains a dcos client and allows marathon specific operations
type MarathonClient struct {
	Client
}

// MakeMarathonClientFn returns a new MarathonClient func
func MakeMarathonClientFn(mkClient func() Client) func() *MarathonClient {
	return func() *MarathonClient {
		client := mkClient()
		mClient := NewMarathonClient(client)
		return mClient
	}
}

// NewMarathonClient returns a new marathon client
func NewMarathonClient(dcosClient Client) *MarathonClient {
	return &MarathonClient{dcosClient}
}

// AppUpdate updates a marathon app
func (c *MarathonClient) AppUpdate(ctx context.Context, appID string, appConfig string, token *string) error {
	logger.Debugf("marathon app update payload: %s", appConfig)
	query := "?force=true"
	return c.marathonExecute(ctx, "PUT", appID, query, appConfig, token)
}

// AppExists attempts to fetch an app by id and returns false if not found
func (c *MarathonClient) AppExists(ctx context.Context, appID string, token *string) (bool, error) {
	mkMarathonReq := marathonRequestMaker(c, "GET", appID, "", "")
	resp, err := c.WithToken(token).HTTPExecute(ctx, mkMarathonReq, NOOPRetry)
	if err != nil {
		return false, err
	}
	if resp.StatusCode == 404 {
		return false, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}
	msg := fmt.Sprintf("(dcos) marathon client resp from app get: %s", util.ModDebugResp(resp))
	return false, fmt.Errorf(util.MsgWithClose(msg, resp.Body))
}

func (c *MarathonClient) createMarathonRequest(method, app, query, payload string) (*http.Request, error) {
	path := fmt.Sprintf("marathon/v2/apps/%s%s", app, query)
	request, err := c.CreateRequest(method, path, payload)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func (c *MarathonClient) marathonExecute(ctx context.Context, method, app, query, payload string, token *string) error {
	mkMarathonReq := marathonRequestMaker(c, method, app, query, payload)
	resp, err := c.WithToken(token).HTTPExecute(ctx, mkMarathonReq, shouldMarathonRetry)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	return nil
}

func marathonRequestMaker(c *MarathonClient, method, app, query, pload string) func() (*http.Request, error) {
	return func() (*http.Request, error) {
		return c.createMarathonRequest(method, app, query, pload)
	}
}

func shouldMarathonRetry(response *http.Response) bool {
	switch {
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return true
	}
	return false
}
