package restapi

import (
	"context"
	"fmt"

	middleware "github.com/go-openapi/runtime/middleware"
	models "github.com/mesosphere/dcos-edge-lb/apiserver/models"
	operations "github.com/mesosphere/dcos-edge-lb/apiserver/restapi/operations"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
)

// V2GetConfigHandler GET config
func V2GetConfigHandler(params operations.V2GetConfigParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2GetConfigDefault {
		logCodeErr(code, err)
		return operations.
			NewV2GetConfigDefault(int(code)).
			WithPayload(errorModel(code, err))
	}

	config, err := configHandler.Get()
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV2GetConfigOK().WithPayload(models.V2ConfigFromContainer(config))
}

// V2GetPoolsHandler GET pools
func V2GetPoolsHandler(params operations.V2GetPoolsParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2GetPoolsDefault {
		logCodeErr(code, err)
		return operations.
			NewV2GetPoolsDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	config, err := configHandler.Get()
	if err != nil {
		return errorFunc(500, err)
	}
	v2config := models.V2ConfigFromContainer(config)
	return operations.NewV2GetPoolsOK().WithPayload(v2config.Pools)
}

// V2GetPoolHandler GET loadbalancer
func V2GetPoolHandler(params operations.V2GetPoolParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2GetPoolDefault {
		logCodeErr(code, err)
		return operations.
			NewV2GetPoolDefault(int(code)).
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
	if pool.V2 == nil {
		return errorFunc(404, fmt.Errorf("pool %s is not APIVersion V2", params.Name))
	}
	return operations.NewV2GetPoolOK().WithPayload(pool.V2)
}

// V2CreatePoolHandler create loadbalancer
func V2CreatePoolHandler(params operations.V2CreatePoolParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2CreatePoolDefault {
		logCodeErr(code, err)
		return operations.
			NewV2CreatePoolDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	exists, err := configHandler.PoolExists(params.Pool.Name)
	if err != nil {
		return errorFunc(500, err)
	}
	if exists {
		return errorFunc(409, fmt.Errorf("pool %s already exists", params.Pool.Name))
	}
	msg := "create load balancer pool"
	if code, err := v2createOrUpdatePool(params.HTTPRequest.Context(), params.Pool, params.Token, msg); err != nil {
		return errorFunc(code, err)
	}
	return operations.
		NewV2CreatePoolOK().
		WithPayload(params.Pool)
}

// V2UpdatePoolHandler update loadbalancer
func V2UpdatePoolHandler(params operations.V2UpdatePoolParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2UpdatePoolDefault {
		logCodeErr(code, err)
		return operations.
			NewV2UpdatePoolDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	exists, err := configHandler.PoolExists(params.Pool.Name)
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("pool %s not found", params.Name))
	}
	msg := "update load balancer pool"
	if code, err := v2createOrUpdatePool(params.HTTPRequest.Context(), params.Pool, params.Token, msg); err != nil {
		return errorFunc(code, err)
	}
	return operations.
		NewV2UpdatePoolOK().
		WithPayload(params.Pool)
}

// V2DeletePoolHandler DELETE loadbalancer
func V2DeletePoolHandler(params operations.V2DeletePoolParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2DeletePoolDefault {
		logCodeErr(code, err)
		return operations.
			NewV2DeletePoolDefault(int(code)).
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
	return operations.NewV2DeletePoolNoContent()
}

// V2GetLBConfigHandler GET loadbalancer artifact
func V2GetLBConfigHandler(params operations.V2GetLBConfigParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2GetLBConfigDefault {
		logCodeErr(code, err)
		return operations.
			NewV2GetLBConfigDefault(int(code)).
			WithPayload(err.Error())
	}
	exists, err := configHandler.ArtifactExists(params.Name, "haproxy.cfg")
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("artifact %s not found", "haproxy.cfg"))
	}

	artifact, err := configHandler.GetPoolArtifact(params.Name, "haproxy.cfg")
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV2GetLBConfigOK().WithPayload(artifact)
}

// V2GetLBTemplateHandler GET loadbalancer artifact
func V2GetLBTemplateHandler(params operations.V2GetLBTemplateParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2GetLBTemplateDefault {
		logCodeErr(code, err)
		return operations.
			NewV2GetLBTemplateDefault(int(code)).
			WithPayload(err.Error())
	}
	exists, err := configHandler.ArtifactExists(params.Name, "haproxy.cfg.ctmpl")
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("artifact %s not found", "haproxy.cfg.ctmpl"))
	}

	artifact, err := configHandler.GetPoolArtifact(params.Name, "haproxy.cfg.ctmpl")
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV2GetLBTemplateOK().WithPayload(artifact)
}

// V2GetDefaultLBTemplateHandler GET loadbalancer artifact
func V2GetDefaultLBTemplateHandler(params operations.V2GetDefaultLBTemplateParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2GetDefaultLBTemplateDefault {
		logCodeErr(code, err)
		return operations.
			NewV2GetDefaultLBTemplateDefault(int(code)).
			WithPayload(err.Error())
	}

	artifact, err := configHandler.GetDefaultPoolArtifact("haproxy.cfg.ctmpl")
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV2GetDefaultLBTemplateOK().WithPayload(artifact)
}

// V2UpdateLBTemplateHandler update loadbalancer artifact
func V2UpdateLBTemplateHandler(params operations.V2UpdateLBTemplateParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2UpdateLBTemplateDefault {
		logCodeErr(code, err)
		return operations.
			NewV2UpdateLBTemplateDefault(int(code)).
			WithPayload(err.Error())
	}
	exists, err := configHandler.ArtifactExists(params.Name, "haproxy.cfg.ctmpl")
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("artifact %s not found", "haproxy.cfg.ctmpl"))
	}
	if !configHandler.ArtifactOverrideable("haproxy.cfg.ctmpl") {
		return errorFunc(405, fmt.Errorf("artifact %s is not overridable", "haproxy.cfg.ctmpl"))
	}

	ctx, cancel := context.WithCancel(params.HTTPRequest.Context())
	msg := "update load balancer artifact"
	go util.CtxTimeout(ctx, cancel, defaultTimeout, timedOut(msg), ctxDone(msg))
	if err := configHandler.TryLockState(ctx); err != nil {
		return errorFunc(500, err)
	}
	defer configHandler.UnlockState()

	if err := configHandler.PutPoolArtifact(ctx, params.Name, "haproxy.cfg.ctmpl", params.Template); err != nil {
		return errorFunc(500, err)
	}
	return operations.
		NewV2UpdateLBTemplateOK().
		WithPayload(*params.Template)
}

// V2DeleteLBTemplateHandler DELETE loadbalancer artifact
func V2DeleteLBTemplateHandler(params operations.V2DeleteLBTemplateParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.V2DeleteLBTemplateDefault {
		logCodeErr(code, err)
		return operations.
			NewV2DeleteLBTemplateDefault(int(code)).
			WithPayload(err.Error())
	}
	exists, err := configHandler.ArtifactExists(params.Name, "haproxy.cfg.ctmpl")
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("artifact %s not found", "haproxy.cfg.ctmpl"))
	}
	if !configHandler.ArtifactOverrideable("haproxy.cfg.ctmpl") {
		return errorFunc(405, fmt.Errorf("artifact %s is not overridable", "haproxy.cfg.ctmpl"))
	}

	ctx, cancel := context.WithCancel(params.HTTPRequest.Context())
	msg := "delete load balancer artifact"
	go util.CtxTimeout(ctx, cancel, defaultTimeout, timedOut(msg), ctxDone(msg))

	if err = configHandler.TryLockState(ctx); err != nil {
		return errorFunc(500, err)
	}
	defer configHandler.UnlockState()

	newArtifact, err := configHandler.DeletePoolArtifact(ctx, params.Name, "haproxy.cfg.ctmpl")
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewV2DeleteLBTemplateOK().
		WithPayload(newArtifact)
}

func v2createOrUpdatePool(origCtx context.Context, pool *models.V2Pool, token *string, msg string) (int32, error) {
	errorFunc := func(code int32, err error) (int32, error) {
		logCodeErr(code, err)
		return code, err
	}

	ctx, cancel := context.WithCancel(origCtx)
	go util.CtxTimeout(ctx, cancel, defaultTimeout, timedOut(msg), ctxDone(msg))

	if pool == nil {
		return errorFunc(400, fmt.Errorf("pool must be present"))
	}
	if err := models.V2CheckPool(pool); err != nil {
		return errorFunc(400, err)
	}
	poolContainer := models.ContainerFromV2Pool(pool)
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
