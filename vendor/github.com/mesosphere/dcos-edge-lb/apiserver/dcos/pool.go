package dcos

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// PoolClient contains a dcos client and allows pool scheduler specific operations
type PoolClient struct {
	Client

	Namespace string
	Name      string
}

// MakePoolClientFn returns a new PoolClient func
func MakePoolClientFn(mkClient func() Client) func(string, string) *PoolClient {
	return func(namespace, name string) *PoolClient {
		client := mkClient()
		pClient := NewPoolClient(client, namespace, name)
		return pClient
	}
}

// NewPoolClient returns a new pool scheduler client
func NewPoolClient(dcosClient Client, namespace, name string) *PoolClient {
	// In the future, if we want to avoid adminrouter, use:
	// dcosClient.WithURL(
	//   fmt.Sprintf("https://%s", PoolSchedulerAddress(namespace, name)))
	return &PoolClient{
		dcosClient,
		namespace,
		name,
	}
}

// PoolAppID creates the App ID
func PoolAppID(poolNamespace, poolName string) string {
	if poolNamespace == "" {
		return poolName
	}
	return fmt.Sprintf("%s/%s", poolNamespace, poolName)
}

// PoolSchedulerAddress returns the VIP for a pool scheduler
func PoolSchedulerAddress(poolNamespace, poolName string) string {
	return fmt.Sprintf("api.%s%s.marathon.l4lb.thisdcos.directory",
		strings.Replace(poolNamespace, "/", "", -1),
		poolName)
}

// ReloadConfig reloads config for a pool
func (c *PoolClient) ReloadConfig(ctx context.Context) error {
	return c.planExecute(ctx, "deploy", "restart", "sidecar-deploy", "{}")
}

func (c *PoolClient) createPlanRequest(plan, operation, phase, payload string) (*http.Request, error) {
	appID := PoolAppID(c.Namespace, c.Name)
	path := fmt.Sprintf("service/%s/v1/plans/%s/%s?phase=%s", appID, plan, operation, phase)
	request, err := c.CreateRequest("POST", path, payload)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func (c *PoolClient) planExecute(ctx context.Context, plan, operation, phase, payload string) error {
	mkPoolReq := planRequestMaker(c, plan, operation, phase, payload)
	resp, err := c.HTTPExecute(ctx, mkPoolReq, shouldPlanRetry)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	return nil
}

func planRequestMaker(c *PoolClient, plan, op, phase, pload string) func() (*http.Request, error) {
	return func() (*http.Request, error) {
		return c.createPlanRequest(plan, op, phase, pload)
	}
}

func shouldPlanRetry(response *http.Response) bool {
	switch {
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return true
	}
	return false
}
