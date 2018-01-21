package restapi

import (
	"context"
	"fmt"

	middleware "github.com/go-openapi/runtime/middleware"
	models "github.com/mesosphere/dcos-edge-lb/apiserver/models"
	operations "github.com/mesosphere/dcos-edge-lb/apiserver/restapi/operations"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
	"github.com/mesosphere/dcos-edge-lb/apiserver/version"
)

// V1PingHandler ping
func V1PingHandler(params operations.V1PingParams) middleware.Responder {
	return operations.NewV1PingOK().WithPayload("pong")
}

// V1VersionHandler ping
func V1VersionHandler(params operations.V1VersionParams) middleware.Responder {
	return operations.NewV1VersionOK().WithPayload(version.Version())
}

// V1GetConfigHandler GET config
func V1GetConfigHandler(params operations.V1GetConfigParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1GetConfigDefault {
		logCodeErr(code, err)
		return operations.
			NewV1GetConfigDefault(int(code)).
			WithPayload(errorModel(code, err))
	}

	config, err := configHandler.Get()
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV1GetConfigOK().WithPayload(models.V1ConfigFromContainer(config))
}

// V1GetLoadBalancerPoolsHandler GET pools
func V1GetLoadBalancerPoolsHandler(params operations.V1GetLoadBalancerPoolsParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1GetLoadBalancerPoolsDefault {
		logCodeErr(code, err)
		return operations.
			NewV1GetLoadBalancerPoolsDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	config, err := configHandler.Get()
	if err != nil {
		return errorFunc(500, err)
	}
	v1config := models.V1ConfigFromContainer(config)
	return operations.NewV1GetLoadBalancerPoolsOK().WithPayload(v1config.Pools)
}

// V1UpdateConfigHandler update config
func V1UpdateConfigHandler(params operations.V1UpdateConfigParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1UpdateConfigDefault {
		logCodeErr(code, err)
		return operations.
			NewV1UpdateConfigDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	if params.Config == nil {
		return errorFunc(400, fmt.Errorf("config must be present"))
	}
	if err := models.V1CheckConfig(params.Config); err != nil {
		return errorFunc(400, err)
	}

	// XXX When we implement changing namespaces we should have this special
	// case trigger a delete of the old pool and relaunch of the new pool.
	// as well as updating the namespace in the poolInfo map.
	configContainer := models.ContainerFromV1Config(params.Config)
	if err := configPreventNamespaceChange(configHandler, configContainer); err != nil {
		return errorFunc(400, err)
	}

	ctx, cancel := context.WithCancel(params.HTTPRequest.Context())
	msg := "update config"
	go util.CtxTimeout(ctx, cancel, defaultTimeout, timedOut(msg), ctxDone(msg))

	if err := configHandler.TryLockState(ctx); err != nil {
		return errorFunc(500, err)
	}
	defer configHandler.UnlockState()

	// This config put was previously destructive. It would delete pools not included
	// in the new one. This behavior is not supported in V2, and we can no longer
	// safely support it in V1.
	if err := configHandler.Put(ctx, configContainer, params.Token); err != nil {
		return errorFunc(500, err)
	}
	config, err := configHandler.Get()
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV1UpdateConfigOK().WithPayload(models.V1ConfigFromContainer(config))
}

// V1GetLoadBalancerPoolHandler GET loadbalancer
func V1GetLoadBalancerPoolHandler(params operations.V1GetLoadBalancerPoolParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1GetLoadBalancerPoolDefault {
		logCodeErr(code, err)
		return operations.
			NewV1GetLoadBalancerPoolDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	exists, err := configHandler.PoolExists(params.Name)
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("pool %s not found", params.Name))
	}

	pool, err := configHandler.GetPool(params.Name)
	if err != nil {
		errorFunc(500, err)
	}
	if pool.V1 == nil {
		return errorFunc(404, fmt.Errorf("pool %s is not APIVersion V1", params.Name))
	}
	return operations.NewV1GetLoadBalancerPoolOK().WithPayload(pool.V1)
}

// V1CreateLoadBalancerPoolHandler create loadbalancer
func V1CreateLoadBalancerPoolHandler(params operations.V1CreateLoadBalancerPoolParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1CreateLoadBalancerPoolDefault {
		logCodeErr(code, err)
		return operations.
			NewV1CreateLoadBalancerPoolDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	msg := "create load balancer pool"
	if code, err := v1createOrUpdateLoadBalancerPool(params.HTTPRequest.Context(), params.LoadBalancer, params.Token, msg); err != nil {
		return errorFunc(code, err)
	}
	return operations.
		NewV1CreateLoadBalancerPoolOK().
		WithPayload(params.LoadBalancer)
}

// V1UpdateLoadBalancerPoolHandler update loadbalancer
func V1UpdateLoadBalancerPoolHandler(params operations.V1UpdateLoadBalancerPoolParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1UpdateLoadBalancerPoolDefault {
		logCodeErr(code, err)
		return operations.
			NewV1UpdateLoadBalancerPoolDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	msg := "update load balancer pool"
	if code, err := v1createOrUpdateLoadBalancerPool(params.HTTPRequest.Context(), params.LoadBalancer, params.Token, msg); err != nil {
		return errorFunc(code, err)
	}
	return operations.
		NewV1UpdateLoadBalancerPoolOK().
		WithPayload(params.LoadBalancer)
}

// V1DeleteLoadBalancerPoolHandler DELETE loadbalancer
func V1DeleteLoadBalancerPoolHandler(params operations.V1DeleteLoadBalancerPoolParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1DeleteLoadBalancerPoolDefault {
		logCodeErr(code, err)
		return operations.
			NewV1DeleteLoadBalancerPoolDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	exists, err := configHandler.PoolExists(params.Name)
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("pool %s not found", params.Name))
	}

	ctx, cancel := context.WithCancel(params.HTTPRequest.Context())
	msg := "delete load balancer pool"
	go util.CtxTimeout(ctx, cancel, defaultTimeout, timedOut(msg), ctxDone(msg))

	if err := configHandler.TryLockState(ctx); err != nil {
		return errorFunc(500, err)
	}
	defer configHandler.UnlockState()

	if err := configHandler.DeletePool(ctx, params.Name, params.Token); err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV1DeleteLoadBalancerPoolNoContent()
}

// V1GetLoadBalancerArtifactsHandler GET loadbalancer artifacts
func V1GetLoadBalancerArtifactsHandler(params operations.V1GetLoadBalancerArtifactsParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1GetLoadBalancerArtifactsDefault {
		logCodeErr(code, err)
		return operations.
			NewV1GetLoadBalancerArtifactsDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	artifacts, err := configHandler.GetPoolArtifacts(params.Name)
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV1GetLoadBalancerArtifactsOK().WithPayload(artifacts)
}

// V1GetLoadBalancerArtifactHandler GET loadbalancer artifact
func V1GetLoadBalancerArtifactHandler(params operations.V1GetLoadBalancerArtifactParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1GetLoadBalancerArtifactDefault {
		logCodeErr(code, err)
		return operations.
			NewV1GetLoadBalancerArtifactDefault(int(code)).
			WithPayload(err.Error())
	}
	exists, err := configHandler.ArtifactExists(params.Name, params.ArtifactName)
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("artifact %s not found", params.ArtifactName))
	}

	artifact, err := configHandler.GetPoolArtifact(params.Name, params.ArtifactName)
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV1GetLoadBalancerArtifactOK().WithPayload(artifact)
}

// V1UpdateLoadBalancerArtifactHandler update loadbalancer artifact
func V1UpdateLoadBalancerArtifactHandler(params operations.V1UpdateLoadBalancerArtifactParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1UpdateLoadBalancerArtifactDefault {
		logCodeErr(code, err)
		return operations.
			NewV1UpdateLoadBalancerArtifactDefault(int(code)).
			WithPayload(err.Error())
	}
	exists, err := configHandler.ArtifactExists(params.Name, params.ArtifactName)
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("artifact %s not found", params.ArtifactName))
	}
	if !configHandler.ArtifactOverrideable(params.ArtifactName) {
		return errorFunc(405, fmt.Errorf("artifact %s is not overridable", params.ArtifactName))
	}

	ctx, cancel := context.WithCancel(params.HTTPRequest.Context())
	msg := "update load balancer artifact"
	go util.CtxTimeout(ctx, cancel, defaultTimeout, timedOut(msg), ctxDone(msg))
	if err := configHandler.TryLockState(ctx); err != nil {
		return errorFunc(500, err)
	}
	defer configHandler.UnlockState()

	if err := configHandler.PutPoolArtifact(ctx, params.Name, params.ArtifactName, params.Artifact); err != nil {
		return errorFunc(500, err)
	}
	return operations.
		NewV1UpdateLoadBalancerArtifactOK().
		WithPayload(*params.Artifact)
}

// V1DeleteLoadBalancerArtifactHandler DELETE loadbalancer artifact
func V1DeleteLoadBalancerArtifactHandler(params operations.V1DeleteLoadBalancerArtifactParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V1DeleteLoadBalancerArtifactDefault {
		logCodeErr(code, err)
		return operations.
			NewV1DeleteLoadBalancerArtifactDefault(int(code)).
			WithPayload(err.Error())
	}
	exists, err := configHandler.ArtifactExists(params.Name, params.ArtifactName)
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("artifact %s not found", params.ArtifactName))
	}
	if !configHandler.ArtifactOverrideable(params.ArtifactName) {
		return errorFunc(405, fmt.Errorf("artifact %s is not overridable", params.ArtifactName))
	}

	ctx, cancel := context.WithCancel(params.HTTPRequest.Context())
	msg := "delete load balancer artifact"
	go util.CtxTimeout(ctx, cancel, defaultTimeout, timedOut(msg), ctxDone(msg))

	if err = configHandler.TryLockState(ctx); err != nil {
		return errorFunc(500, err)
	}
	defer configHandler.UnlockState()

	newArtifact, err := configHandler.DeletePoolArtifact(ctx, params.Name, params.ArtifactName)
	if err != nil {
		return errorFunc(500, err)
	}
	if newArtifact == "" {
		return operations.NewV1DeleteLoadBalancerArtifactNoContent()
	}
	return operations.NewV1DeleteLoadBalancerArtifactOK().
		WithPayload(newArtifact)
}

func v1createOrUpdateLoadBalancerPool(origCtx context.Context, pool *models.V1Pool, token *string, msg string) (int32, error) {
	errorFunc := func(code int32, err error) (int32, error) {
		logCodeErr(code, err)
		return code, err
	}

	ctx, cancel := context.WithCancel(origCtx)
	go util.CtxTimeout(ctx, cancel, defaultTimeout, timedOut(msg), ctxDone(msg))

	if pool == nil {
		return errorFunc(400, fmt.Errorf("pool must be present"))
	}
	if err := models.V1CheckPool(pool); err != nil {
		return errorFunc(400, err)
	}
	poolContainer := models.ContainerFromV1Pool(pool)
	if err := poolPreventNamespaceChangeWrapper(configHandler, poolContainer); err != nil {
		return errorFunc(400, err)
	}

	if err := configHandler.TryLockState(ctx); err != nil {
		return errorFunc(500, err)
	}
	defer configHandler.UnlockState()

	if err := configHandler.PutPool(ctx, poolContainer, token); err != nil {
		return errorFunc(500, err)
	}
	return 200, nil
}
