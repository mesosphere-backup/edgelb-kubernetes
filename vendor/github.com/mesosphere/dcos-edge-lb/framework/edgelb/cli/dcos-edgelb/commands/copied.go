package commands

import (
	"fmt"

	sdkClient "github.com/mesosphere/dcos-commons/cli/client"

	// sdkConfig is a terrible creation that is a global package where they
	// flip global variables in order to set and retrieve config values.
	// We unfortunately have to modify this and it makes all of our code
	// inherently thread-unsafe.
	sdkConfig "github.com/mesosphere/dcos-commons/cli/config"
)

type sdkAPIState struct{}

// This interface is for testing (mocking) purposes
type sdkAPI interface {
	podHandleList(serviceName string) ([]byte, error)
	podHandleInfo(serviceName, podName string) ([]byte, error)
	endpointsHandleEndpoints(serviceName, endpointName string) ([]byte, error)
}

func newSDKAPI() sdkAPI {
	return sdkAPIState{}
}

// These are functions were initially copied from dcos-commons, and then
// modified to have them work. One day we should upstream changes that make
// these public functions.

func (s sdkAPIState) podHandleList(serviceName string) ([]byte, error) {
	// An unfortunate dance with global variables
	origServiceName := sdkConfig.ServiceName
	sdkConfig.ServiceName = serviceName
	defer func() {
		sdkConfig.ServiceName = origServiceName
	}()

	return sdkClient.HTTPServiceGet("v1/pod")
}

func (s sdkAPIState) podHandleInfo(serviceName, podName string) ([]byte, error) {
	// An unfortunate dance with global variables
	origServiceName := sdkConfig.ServiceName
	sdkConfig.ServiceName = serviceName
	defer func() {
		sdkConfig.ServiceName = origServiceName
	}()

	return sdkClient.HTTPServiceGet(fmt.Sprintf("v1/pod/%s/info", podName))
}

func (s sdkAPIState) endpointsHandleEndpoints(serviceName, endpointName string) ([]byte, error) {
	// An unfortunate dance with global variables
	origServiceName := sdkConfig.ServiceName
	sdkConfig.ServiceName = serviceName
	defer func() {
		sdkConfig.ServiceName = origServiceName
	}()

	path := "v1/endpoints"
	if endpointName != "" {
		path += "/" + endpointName
	}
	responseBytes, err := sdkClient.HTTPServiceGet(path)
	if err != nil {
		return nil, err
	}
	return responseBytes, nil
}
