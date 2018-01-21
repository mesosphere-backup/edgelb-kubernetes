package dcos

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
)

const (
	cosmosVersion = "v1"
)

// CosmosEnvObj env
type CosmosEnvObj struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// CosmosSecretObj secret obj
type CosmosSecretObj struct {
	Secret string `json:"secret,omitempty"`
	File   string `json:"file,omitempty"`
	Index  int32  `json:"index,omitempty"`
}

// CosmosNetworkObj network obj
type CosmosNetworkObj struct {
	Name   string `json:"name,omitempty"`
	Labels string `json:"labels,omitempty"`
	Index  int32  `json:"index,omitempty"`
}

// CosmosServiceObj service
type CosmosServiceObj struct {
	Name       string `json:"name,omitempty"`
	SecretName string `json:"secretName,omitempty"`
	Principal  string `json:"principal,omitempty"`
}

// CosmosEdgeLbPoolObj edge lb pool
type CosmosEdgeLbPoolObj struct {
	Name        string             `json:"name,omitempty"`
	Count       int32              `json:"count,omitempty"`
	Disk        int32              `json:"disk,omitempty"`
	Mem         int32              `json:"mem,omitempty"`
	MemSidecar  int32              `json:"memSidecar,omitempty"`
	Ports       string             `json:"ports,omitempty"`
	Secrets     []CosmosSecretObj  `json:"secrets,omitempty"`
	Networks    []CosmosNetworkObj `json:"networks,omitempty"`
	Env         []CosmosEnvObj     `json:"env,omitempty"`
	Cpus        float64            `json:"cpus,omitempty"`
	CpusSidecar float64            `json:"cpusSidecar,omitempty"`
	Constraints string             `json:"placementConstraints,omitempty"`
	AutoCert    string             `json:"autocert,omitempty"`
	Role        string             `json:"role,omitempty"`
}

// CosmosOptionsObj options
type CosmosOptionsObj struct {
	Service    CosmosServiceObj    `json:"service"`
	EdgeLbPool CosmosEdgeLbPoolObj `json:"edgelbPool"`
}

// CosmosPackageConfig info for package installs
type CosmosPackageConfig struct {
	Options        CosmosOptionsObj `json:"options"`
	PackageName    string           `json:"packageName,omitempty"`
	PackageVersion string           `json:"packageVersion,omitempty"`
	AppID          string           `json:"appId,omitempty"`
}

// CosmosResponse response from cosmos
type CosmosResponse struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}

// CosmosClient contains a dcos client and allows cosmos specific operations
type CosmosClient struct {
	Client

	MarathonClient    *MarathonClient
	DefaultSecretName string
	DefaultPrincipal  string
}

// MakeCosmosClientFn returns a new CosmosClient func with default secret name and principal configured
func MakeCosmosClientFn(mkClient func() Client, defaultSecretName, defaultPrincipal string) func() *CosmosClient {
	return func() *CosmosClient {
		mkMarathonClient := MakeMarathonClientFn(mkClient)
		mClient := mkMarathonClient()
		client := mkClient()
		cClient := NewCosmosClient(client, mClient, defaultSecretName, defaultPrincipal)
		return cClient
	}
}

// NewCosmosClient returns a new cosmos client
func NewCosmosClient(dcosClient Client, marathonClient *MarathonClient, defaultSecretName, defaultPrincipal string) *CosmosClient {
	return &CosmosClient{dcosClient, marathonClient, defaultSecretName, defaultPrincipal}
}

// PackageInstalled returns true if an edgelb pool framework is installed
func (c *CosmosClient) PackageInstalled(ctx context.Context, appID string, token *string) (bool, error) {
	return c.MarathonClient.AppExists(ctx, appID, token)
}

// PackageInstall installs an edgelb pool framework
func (c *CosmosClient) PackageInstall(ctx context.Context, pkgCfg *CosmosPackageConfig, token *string) error {
	if pkgCfg.Options.Service.SecretName == "" {
		pkgCfg.Options.Service.SecretName = c.DefaultSecretName
	}
	if pkgCfg.Options.Service.Principal == "" {
		pkgCfg.Options.Service.Principal = c.DefaultPrincipal
	}
	payloadMarshal, _ := json.Marshal(pkgCfg)
	payloadStr := string(payloadMarshal)
	return c.cosmosExecute(ctx, "install", payloadStr, token)
}

// PackageUpdate updates and relaunches an edgelb pool framework
func (c *CosmosClient) PackageUpdate(ctx context.Context, pkgCfg *CosmosPackageConfig, token *string) error {
	if pkgCfg.Options.Service.SecretName == "" {
		pkgCfg.Options.Service.SecretName = c.DefaultSecretName
	}
	if pkgCfg.Options.Service.Principal == "" {
		pkgCfg.Options.Service.Principal = c.DefaultPrincipal
	}
	payloadMarshal, _ := json.Marshal(pkgCfg)
	payloadStr := string(payloadMarshal)

	logger.Debugf("cosmos render payload: %s", payloadStr)
	mkCosmosReq := cosmosRequestMaker(c, "render", payloadStr)
	resp, err := c.WithToken(token).HTTPExecute(ctx, mkCosmosReq, shouldCosmosRetry)
	if err != nil {
		return err
	}

	respB, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf(util.MsgWithClose(err.Error(), resp.Body))
	}
	if err = resp.Body.Close(); err != nil {
		return err
	}

	marathonMap := map[string]interface{}{}
	if err = json.Unmarshal(respB, &marathonMap); err != nil {
		return fmt.Errorf("unable to decode current running pool marathon app, err: %s, text: %s", err, string(respB))
	}
	appJSONMap := marathonMap["marathonJson"]
	appJSON, err := json.Marshal(appJSONMap)
	if err != nil {
		return fmt.Errorf("unable to encode updated marathon app: %s", err)
	}
	return c.MarathonClient.AppUpdate(ctx, pkgCfg.AppID, string(appJSON), token)
}

// PackageUninstall uninstalls an edgelb pool framework
func (c *CosmosClient) PackageUninstall(ctx context.Context, appID string, token *string) error {
	// XXX Currently we just do a uninstall operation and stop. As the cosmos
	// uninstall is actually asynchronous, we should block here until it is
	// actually uninstalled and potentially retry if the uninstall fails.
	// Unfortunately it would be extremely difficult to actually determine
	// with certainty that the uninstall was complete, because uninstalling
	// involves the scheduler restarting and then operating for some time
	// while unreserving pool resources.

	payload := new(CosmosPackageConfig)
	payload.PackageName = "edgelb-pool"
	payload.AppID = appID
	payloadMarshal, _ := json.Marshal(payload)
	payloadStr := string(payloadMarshal)
	return c.cosmosExecute(ctx, "uninstall", payloadStr, token)
}

// Internal

func (c *CosmosClient) createCosmosRequest(operation, payload string) (*http.Request, error) {
	path := fmt.Sprintf("package/%s", operation)
	request, err := c.CreateRequest("POST", path, payload)
	if err != nil {
		return nil, err
	}
	contentTypePart := fmt.Sprintf("package.%s", operation)
	request.Header.Set("Content-Type", fmt.Sprintf("application/vnd.dcos.%s-request+json;charset=utf-8;version=%s", contentTypePart, cosmosVersion))
	request.Header.Set("Accept", fmt.Sprintf("application/vnd.dcos.%s-response+json;charset=utf-8;version=%s", contentTypePart, cosmosVersion))
	return request, nil
}

func (c *CosmosClient) cosmosExecute(ctx context.Context, op, pload string, token *string) error {
	logger.Debugf("cosmos execute payload: %s", pload)
	mkCosmosReq := cosmosRequestMaker(c, op, pload)
	resp, err := c.WithToken(token).HTTPExecute(ctx, mkCosmosReq, shouldCosmosRetry)
	if err != nil {
		return err
	}
	return handleCosmosResponse(resp)
}

func handleCosmosResponse(resp *http.Response) error {
	if resp.StatusCode == http.StatusBadRequest {
		respB, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf(util.MsgWithClose(err.Error(), resp.Body))
		}
		if err := resp.Body.Close(); err != nil {
			return err
		}
		cosmosResp := CosmosResponse{}
		if err := json.Unmarshal(respB, &cosmosResp); err != nil {
			return fmt.Errorf("unable to decode cosmos bad request response, err: %s, text: %s", err, string(respB))
		}
		// Attempted to uninstall non-existent framework, exit normal
		if cosmosResp.Type == "UninstallNonExistentAppForPackage" {
			return nil
		}
		return fmt.Errorf("error executing cosmos request: %s: %s", cosmosResp.Type, cosmosResp.Message)
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	return nil
}

func cosmosRequestMaker(c *CosmosClient, op, pload string) func() (*http.Request, error) {
	return func() (*http.Request, error) {
		return c.createCosmosRequest(op, pload)
	}
}

func shouldCosmosRetry(response *http.Response) bool {
	switch {
	case response.StatusCode == http.StatusConflict:
		return false
	case response.StatusCode == http.StatusBadRequest:
		return false
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return true
	}
	return false
}
