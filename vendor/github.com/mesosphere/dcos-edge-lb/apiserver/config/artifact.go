package config

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/models"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
)

const (
	defaultHaproxyCfgTmplPath = "templates/haproxy.cfg.ctmpl"
	poolHaproxyCfgTmplFmt     = "haproxy.%s.cfg.ctmpl"
	poolHaproxyCfgFmt         = "haproxy.%s.cfg"
	haproxyCfgTmplName        = "haproxy.cfg.ctmpl"
	haproxyCfgName            = "haproxy.cfg"
	defaultRetry              = time.Second * 3
)

// GetPoolArtifacts gets pool artifacts
func (s *State) GetPoolArtifacts(poolName string) ([]string, error) {
	// These are the only supported pool artifacts currently
	return []string{haproxyCfgName, haproxyCfgTmplName}, nil
}

// ArtifactExists checks if an artifact exists
func (s *State) ArtifactExists(poolName, artifactName string) (bool, error) {
	filename := poolArtifactPath(poolName, artifactName)
	if filename == "" {
		return false, nil
	}
	return s.manager.FileExists(filename)
}

// GetPoolArtifact gets a pool artifact
func (s *State) GetPoolArtifact(poolName, artifactName string) (string, error) {
	return s.readArtifactString(poolName, artifactName)
}

// GetDefaultPoolArtifact gets a pool artifact
func (s *State) GetDefaultPoolArtifact(artifactName string) (string, error) {
	if artifactName == "haproxy.cfg.ctmpl" {
		tmpl, tmplErr := s.defaultHaproxyCfgCtmplBytes()
		if tmplErr != nil {
			return "", tmplErr
		}
		return string(tmpl), nil
	}
	return "", nil
}

// ArtifactOverrideable checks if artifact can be modified
func (s *State) ArtifactOverrideable(artifactName string) bool {
	switch artifactName {
	case haproxyCfgTmplName:
		return true
	default:
		return false
	}
}

// PutPoolArtifact updates a pool artifact value in ZK
func (s *State) PutPoolArtifact(ctx context.Context, poolName, artifactName string, artifact *string) error {
	return s.writeArtifactString(ctx, poolName, artifactName, *artifact)
}

// DeletePoolArtifact deletes a pool config value in ZK
func (s *State) DeletePoolArtifact(ctx context.Context, poolName, artifactName string) (string, error) {
	switch artifactName {
	case haproxyCfgTmplName:
		// Delete the file from ZK and local file
		filename := poolArtifactPath(poolName, haproxyCfgTmplName)
		if err := s.manager.DeleteFile(ctx, filename); err != nil {
			return "", err
		}
		// Re-init the file as default
		tmpl, tmplErr := s.defaultHaproxyCfgCtmplBytes()
		if tmplErr != nil {
			return "", tmplErr
		}
		if initErr := s.initArtifactBytes(ctx, poolName, haproxyCfgTmplName, tmpl); initErr != nil {
			return "", initErr
		}
		return s.GetPoolArtifact(poolName, artifactName)
	default:
		return "", fmt.Errorf("artifact %s cannot be deleted", artifactName)
	}
}

// Internal

func (s *State) initArtifactBytes(ctx context.Context, poolName, artifactName string, artifact []byte) error {
	filename := poolArtifactPath(poolName, artifactName)
	cleanedArtifact := cleanArtifactBytes(poolName, artifactName, artifact)
	return s.initFileBytes(ctx, filename, cleanedArtifact)
}

func (s *State) readArtifactString(poolName, artifactName string) (string, error) {
	filename := poolArtifactPath(poolName, artifactName)
	return s.readFileString(filename)
}

func (s *State) writeArtifactString(ctx context.Context, poolName, artifactName, artifact string) error {
	return s.writeArtifactBytes(ctx, poolName, artifactName, []byte(artifact))
}

func (s *State) writeArtifactBytes(ctx context.Context, poolName, artifactName string, artifact []byte) error {
	filename := poolArtifactPath(poolName, artifactName)
	cleanedArtifact := cleanArtifactBytes(poolName, artifactName, artifact)
	return s.writeFileBytes(ctx, filename, cleanedArtifact)
}

// initPoolArtifacts creates or loads necessary artifact files for a pool
func (s *State) initPoolArtifacts(ctx context.Context, pool *models.PoolContainer) error {
	// XXX: writing the default haproxy.cfg for every pool. The default config template
	//      for a pool should be done with a strategy pattern that switches based on
	//      the type of load balancer pool in the future (haproxy, nginx, etc)

	if err := s.runArtifactWatcher(*pool.Namespace, pool.Name); err != nil {
		return err
	}

	tmpl, tmplErr := s.defaultHaproxyCfgCtmplBytes()
	if tmplErr != nil {
		return tmplErr
	}
	return s.initArtifactBytes(ctx, pool.Name, haproxyCfgTmplName, tmpl)
}

func (s *State) runArtifactWatcher(poolNamespace, poolName string) error {
	client := s.mkPoolClient(poolNamespace, poolName)
	watch, watchErr := s.mkFileWatcher(
		fmt.Sprintf(poolHaproxyCfgFmt, poolName),
		fmt.Sprintf(poolHaproxyCfgFmt, poolName),
		mkPoolReloadConfigFn(client))
	if watchErr != nil {
		return watchErr
	}
	watchCtx, watchCancel := context.WithCancel(context.Background())
	go watchWrapper(watchCtx, watch)
	s.pools[poolName].cancel = watchCancel
	return nil
}

// XXX: In the future, the logic to communicate with a pool's scheduler and retry those
// calls when it fails or the scheduler is temporarily unreachable should be handled by
// a pool monitor / fsm which can intelligently reason about when it should and should not
// retry. Additionally, we should not be able to queue up multiple contexts trying to reach
// a single pool.
func mkPoolReloadConfigFn(client *dcos.PoolClient) FileChangeHandleFn {
	return func(origCtx context.Context) error {
		ctx, cancel := context.WithCancel(origCtx)
		defer cancel()
		go util.CtxTimeout(
			ctx,
			cancel,
			defaultTimeout,
			func() { logger.Debug("context timed out: config") },
			func() { logger.Debug("context completed before timeout: config") })
		if err := client.ReloadConfig(ctx); err != nil {
			// XXX Once we have infinite retry we can return the error
			// properly. Today, we just give up on the sidecar reload as
			// it wouldn't help to return an error and kill the watcher here.
			logger.Infof("failed config sidecar-reload: %s", err)
		}
		return nil
	}
}

func watchWrapper(ctx context.Context, watch Watcher) {
	for {
		if err := watch.Run(ctx); err != nil {
			logger.Errorf("artifact watchWrapper failed, will retry: %s", err)
			time.Sleep(defaultRetry)
			continue
		}
		logger.Infof("artifact watchWrapper done: %s", watch.String())
		return
	}
}

// The above `DeletePoolArtifact` is used to delete / reset an artifact which is overridable while a pool
// is still running. `deletePoolArtifacts` is used when deleting / uninstalling a pool to actually remove
// all pool artifacts. This should essentially undo `initPoolArtifacts`.
func (s *State) deletePoolArtifacts(ctx context.Context, poolName string) error {
	filename := poolArtifactPath(poolName, haproxyCfgTmplName)
	if err := s.manager.DeleteFile(ctx, filename); err != nil {
		return err
	}
	s.pools[poolName].cancel()
	return nil
}

func (s *State) defaultHaproxyCfgCtmplBytes() ([]byte, error) {
	tmpl, err := s.readFileBytes(defaultHaproxyCfgTmplPath)
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

func cleanArtifactBytes(poolName, artifactName string, artifact []byte) []byte {
	switch artifactName {
	case haproxyCfgTmplName:
		return cleanhaproxyCfgTmplBytes(poolName, artifact)
	default:
		return artifact
	}
}

func cleanhaproxyCfgTmplBytes(poolName string, artifact []byte) []byte {
	return bytes.Replace(artifact, []byte("__ELB_POOL_NAME__"), []byte(poolName), 1)
}

// Returns "" if artifact is not supported
func poolArtifactPath(poolName, artifactName string) string {
	switch artifactName {
	case haproxyCfgName:
		return poolHaproxyCfgPath(poolName)
	case haproxyCfgTmplName:
		return poolHaproxyCfgTmplPath(poolName)
	default:
		return ""
	}
}

func poolHaproxyCfgTmplPath(poolName string) string {
	return fmt.Sprintf(poolHaproxyCfgTmplFmt, poolName)
}

func poolHaproxyCfgPath(poolName string) string {
	return fmt.Sprintf(poolHaproxyCfgFmt, poolName)
}
