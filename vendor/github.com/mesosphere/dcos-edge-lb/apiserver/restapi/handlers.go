package restapi

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	middleware "github.com/go-openapi/runtime/middleware"
	config "github.com/mesosphere/dcos-edge-lb/apiserver/config"
	dcos "github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	models "github.com/mesosphere/dcos-edge-lb/apiserver/models"
	operations "github.com/mesosphere/dcos-edge-lb/apiserver/restapi/operations"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
	"github.com/mesosphere/dcos-edge-lb/apiserver/version"

	"github.com/Sirupsen/logrus"
)

const (
	defaultTimeout   = time.Second * 10
	defaultRetry     = time.Second * 3
	tmplRegexPattern = `haproxy\..*\.cfg\.ctmpl`
	tmplGlobPattern  = "haproxy.*.cfg.ctmpl"
)

var (
	configHandler *config.State
)

func initHandlers() {
	zkTimeout, parseErr := time.ParseDuration(extraFlags.ZkTimeoutStr)
	if parseErr != nil {
		logger.WithFields(logrus.Fields{
			"err":       parseErr,
			"zkTimeout": extraFlags.ZkTimeoutStr,
		}).Fatal("failed to parse zk timeout")
	}

	configManager, manErr := config.NewManager(
		extraFlags.LbWorkDir,
		extraFlags.ZkConfigPath,
		extraFlags.ZkString,
		zkTimeout)
	if manErr != nil {
		logger.WithError(manErr).Fatal("Failed to create Manager")
	}

	mkFileWatcher := config.MakeFileWatcherFn(extraFlags.LbWorkDir)

	// Restart dcos-template if cfgcache.json changes
	configCacheTemplateSvcWatcher, confWatchErr := mkFileWatcher(
		extraFlags.CfgCacheFilename,
		extraFlags.CfgCacheFilename,
		templateRestartSvcFunc(extraFlags.DcosTemplateSv))
	if confWatchErr != nil {
		logger.WithError(confWatchErr).Fatal("Failed to start config watcher")
	}
	go infiniteWatchWrapper(configCacheTemplateSvcWatcher)

	// Restart dcos-template if haproxy.*.cfg.ctmpl changes
	poolTemplateSvcWatcher, tmplWatchErr := mkFileWatcher(
		tmplRegexPattern,
		tmplGlobPattern,
		templateRestartSvcFunc(extraFlags.DcosTemplateSv))
	if tmplWatchErr != nil {
		logger.WithError(tmplWatchErr).Fatal("Failed to start template watcher")
	}
	go infiniteWatchWrapper(poolTemplateSvcWatcher)

	mkClient := dcos.MakeClientFn(extraFlags.DcosAuthCreds, extraFlags.DcosAddr, extraFlags.DcosProt)
	mkCosmosClient := dcos.MakeCosmosClientFn(mkClient, extraFlags.DcosAuthSecretName, extraFlags.DcosPrincipal)
	mkPoolClient := dcos.MakePoolClientFn(mkClient)
	var stateErr error
	configHandler, stateErr = config.NewState(
		configManager,
		extraFlags.CfgCacheFilename,
		mkCosmosClient,
		mkPoolClient,
		mkFileWatcher)
	if stateErr != nil {
		logger.WithError(stateErr).Fatal("Failed to create new state")
	}
}

// PingHandler ping
func PingHandler(params operations.PingParams) middleware.Responder {
	return operations.NewPingOK().WithPayload("pong")
}

// VersionHandler ping
func VersionHandler(params operations.VersionParams) middleware.Responder {
	return operations.NewVersionOK().WithPayload(version.Version())
}

// GetConfigContainerHandler GET config container
func GetConfigContainerHandler(params operations.GetConfigContainerParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.GetConfigContainerDefault {
		logCodeErr(code, err)
		return operations.
			NewGetConfigContainerDefault(int(code)).
			WithPayload(errorModel(code, err))
	}

	configContainer, err := configHandler.Get()
	if err != nil {
		return errorFunc(500, err)
	}
	return operations.NewGetConfigContainerOK().WithPayload(configContainer)
}

// GetPoolContainerHandler GET loadbalancer
func GetPoolContainerHandler(params operations.GetPoolContainerParams) middleware.Responder {
	errorFunc := func(code int32, err error) *operations.GetPoolContainerDefault {
		logCodeErr(code, err)
		return operations.
			NewGetPoolContainerDefault(int(code)).
			WithPayload(errorModel(code, err))
	}
	exists, err := configHandler.PoolExists(params.Name)
	if err != nil {
		return errorFunc(500, err)
	}
	if !exists {
		return errorFunc(404, fmt.Errorf("pool %s not found", params.Name))
	}
	poolContainer, err := configHandler.GetPool(params.Name)
	if err != nil {
		errorFunc(500, err)
	}
	return operations.NewGetPoolContainerOK().WithPayload(poolContainer)
}

// ServerShutdown is empty
func ServerShutdown() {
	panic("ServerShutdown unimplemented")
}

func templateRestartSvcFunc(dcosTemplateSv string) config.FileChangeHandleFn {
	return func(unusedCtx context.Context) error {
		logger.Debugf("killing %s", dcosTemplateSv)
		cmd := exec.Command("sv", "kill", dcosTemplateSv)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error restarting dcos-template: %s", err)
		}
		return nil
	}
}

func errorModel(code int32, err error) *models.Error {
	return &models.Error{
		Code:    code,
		Message: err.Error()}
}

func timedOut(s string) util.CtxTimeoutCallbackFn {
	return func() {
		logger.Debugf("context timed out: %s", s)
	}
}

func ctxDone(s string) util.CtxDoneCallbackFn {
	return func() {
		logger.Debugf("context completed before timeout: %s", s)
	}
}

func infiniteWatchWrapper(watch config.Watcher) {
	for {
		if err := watch.Run(context.Background()); err != nil {
			logger.WithError(err).Error("handler watchWrapper failed")
		} else {
			logger.Errorf("handler watchWrapper terminated")
		}
		time.Sleep(defaultRetry)
	}
}

func configPreventNamespaceChange(s *config.State, newConfig *models.ConfigContainer) error {
	oldConfig, getErr := s.Get()
	if getErr != nil {
		return getErr
	}

	oldMap := make(map[string]*models.PoolContainer)
	for _, pool := range oldConfig.Pools {
		oldMap[pool.Name] = pool
	}

	for _, pool := range newConfig.Pools {
		oldPool, exists := oldMap[pool.Name]
		if !exists {
			continue
		}
		if err := poolPreventNamespaceChange(oldPool, pool); err != nil {
			return err
		}
	}
	return nil
}

func poolPreventNamespaceChangeWrapper(s *config.State, newPool *models.PoolContainer) error {
	config, getErr := s.Get()
	if getErr != nil {
		return getErr
	}
	for _, pool := range config.Pools {
		if pool.Name != newPool.Name {
			continue
		}
		if err := poolPreventNamespaceChange(pool, newPool); err != nil {
			return err
		}
	}
	return nil
}

func poolPreventNamespaceChange(oldPool, newPool *models.PoolContainer) error {
	// XXX The correct behavior for namespace changes is to delete the pool
	// with the old namespace, and create a new pool with the new namespace.
	//
	// However, we currently do not block until uninstall is actually complete.
	// As such, we are not able to do a uninstall+reinstall directly in here.
	// Because of this, we simply prevent namespace changes.

	oldName := oldPool.Name
	newName := newPool.Name
	oldNS := *oldPool.Namespace
	newNS := *newPool.Namespace

	if oldName != newName {
		return fmt.Errorf("namespace check not matching pool name %s %s", oldName, newName)
	}

	if oldNS != newNS {
		return fmt.Errorf("changing namespace currently not supported: please uninstall pool %s with namespace %s, wait for the pool to completely uninstall, and then re-install pool %s with the new namespace %s", oldName, oldNS, newName, newNS)
	}
	return nil
}

func logCodeErr(code int32, err error) {
	// Log 5XX as error
	if code/100 == 5 {
		logger.Error(err)
	}

	// Log 4XX as debug
	if code/100 == 4 {
		logger.Debug(err)
	}
}
