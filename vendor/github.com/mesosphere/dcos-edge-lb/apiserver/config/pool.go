package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/models"
)

type poolInfo struct {
	ns     string             // Namespace of pool
	cancel context.CancelFunc // Cancel for pool file watcher
}

// PoolExists checks if a pool exists
func (s *State) PoolExists(name string) (bool, error) {
	config, err := s.Get()
	if err != nil {
		return false, err
	}
	found, _, _ := findPool(config, name)
	return found, nil
}

// GetPool gets a current pool config
func (s *State) GetPool(name string) (*models.PoolContainer, error) {
	config, err := s.Get()
	if err != nil {
		return nil, err
	}
	found, _, pool := findPool(config, name)
	if !found {
		return nil, fmt.Errorf("pool %s not found", name)
	}
	return pool, nil
}

// PutPool updates a pool config value in ZK
// token is the core.dcos_acs_token from user's CLI session. It is used to authenticate
// the requests to adminrouter when installing / updating pool frameworks
func (s *State) PutPool(ctx context.Context, pool *models.PoolContainer, token *string) error {
	config, getErr := s.Get()
	if getErr != nil {
		return getErr
	}

	found, poolIndex, _ := findPool(config, pool.Name)

	if !found {
		config.Pools = append(config.Pools, pool)
	} else {
		config.Pools[poolIndex] = pool
	}

	// Write before anything else, so if we crash we will try again.
	//
	// Effects:
	// changes cfgcache.json
	// restarts dcos-teplate
	// changes haproxy.poolName.cfg
	// requests sidecar reload but if PackageInstall fails,
	//   no scheduler will be there to take request
	if err := s.writeConfig(ctx, config); err != nil {
		return err
	}
	if err := s.putPoolState(ctx, pool); err != nil {
		return err
	}
	if err := s.putPoolFramework(ctx, pool, token); err != nil {
		return err
	}
	return nil
}

// Modify apiserver and cluster state
func (s *State) putPoolState(ctx context.Context, pool *models.PoolContainer) error {
	// This makes modifications to everything except zk, and does so
	// without knowledge of what is in zk.

	// XXX We should consider doing nothing if nothing changes in the pool
	// config. Or perhaps we are relying on the pool scheduler to decide
	// whether or not anything has changed?
	_, poolTracked := s.pools[pool.Name]

	// Effects:
	// changes haproxy.poolName.cfg.ctmpl
	// restarts dcos-teplate but if write to cfgcache.json fails,
	//   request for latest pools won't have this pool
	// changes haproxy.poolName.cfg
	// requests sidecar reload but if PackageInstall fails,
	//   no scheduler will be there to take request
	if !poolTracked {
		// XXX When we implement changing namespace we should have this special
		// case trigger a delete of the old pool and relaunch of the new pool.
		// as well as updating the namespace in the poolInfo map.
		s.pools[pool.Name] = &poolInfo{ns: *pool.Namespace}
		if err := s.initPoolArtifacts(ctx, pool); err != nil {
			return err
		}
	} else if s.pools[pool.Name].ns != *pool.Namespace {
		// XXX This check can go away once we implement namespace changes
		s := fmt.Sprintf("pool namespace went out of sync %s %s", s.pools[pool.Name].ns, *pool.Namespace)
		logger.Error(s)
		return fmt.Errorf(s)
	}
	return nil

}

// putPoolFramework Install / Update pool framework instances
func (s *State) putPoolFramework(ctx context.Context, pool *models.PoolContainer, token *string) error {
	pkgCfg, err := createDcosConfig(pool)
	if err != nil {
		s := fmt.Sprintf("unable to create package config from pool %s", pool.Name)
		logger.Error(s)
		return fmt.Errorf(s)
	}
	client := s.mkCosmosClient()

	installed, err := client.PackageInstalled(ctx, pkgCfg.AppID, token)
	if err != nil {
		s := fmt.Sprintf("unable to check if package installed for pool %s, error: %+v", pool.Name, err)
		logger.Error(s)
		return fmt.Errorf(s)
	}
	if !installed {
		return client.PackageInstall(ctx, pkgCfg, token)
	}
	return client.PackageUpdate(ctx, pkgCfg, token)
}

// DeletePool deletes a pool config value in ZK
func (s *State) DeletePool(ctx context.Context, name string, token *string) error {
	config, getErr := s.Get()
	if getErr != nil {
		return getErr
	}

	found, poolIndex, pool := findPool(config, name)
	if !found {
		return fmt.Errorf("pool %s not found", name)
	}

	client := s.mkCosmosClient()
	appID := dcos.PoolAppID(*pool.Namespace, name)
	// XXX: We should consider making this idempotent by not failing
	// on certain errors. It is possible for the package uninstall to
	// fail, but we may actually still want to force removal of artifacts, etc.
	if err := client.PackageUninstall(ctx, appID, token); err != nil {
		return err
	}
	if err := s.deletePoolArtifacts(ctx, name); err != nil {
		return err
	}
	delete(s.pools, name)

	// Delete from zk after everything else, so we never lose track of pools.
	config.Pools = append(config.Pools[:poolIndex], config.Pools[poolIndex+1:]...)
	return s.writeConfig(ctx, config)
}

// Internal

func createDcosConfig(poolContainer *models.PoolContainer) (*dcos.CosmosPackageConfig, error) {
	// Converting pool to V2 because all of the relevant fields are identical
	pool, err := models.V2PoolFromContainer(poolContainer)
	if err != nil {
		return nil, err
	}

	parsedEnv := []dcos.CosmosEnvObj{}
	for k, v := range pool.EnvironmentVariables {
		parsedEnv = append(parsedEnv, dcos.CosmosEnvObj{Key: k, Value: v})
	}

	sPorts := models.V2PoolBindPortsStr(pool)
	parsedPorts := strings.Join(sPorts, ",")
	poolPrincipal := ""
	poolSecretName := ""
	if pool.Principal != nil {
		poolPrincipal = *pool.Principal
	}
	if pool.SecretName != nil {
		poolSecretName = *pool.SecretName
	}

	pkg := &dcos.CosmosPackageConfig{
		PackageName:    pool.PackageName,
		PackageVersion: pool.PackageVersion,
		AppID:          poolAppID(pool),
		Options: dcos.CosmosOptionsObj{
			Service: dcos.CosmosServiceObj{
				Name:       poolAppID(pool),
				Principal:  poolPrincipal,
				SecretName: poolSecretName,
			},
			EdgeLbPool: dcos.CosmosEdgeLbPoolObj{
				Name:        pool.Name,
				Ports:       parsedPorts,
				Secrets:     poolSecrets(pool),
				Networks:    poolNetworks(pool),
				Env:         parsedEnv,
				Count:       *pool.Count,
				Constraints: *pool.Constraints,
				Role:        pool.Role,
				Cpus:        pool.Cpus,
				CpusSidecar: pool.CpusAdminOverhead,
				Disk:        pool.Disk,
				Mem:         pool.Mem,
				MemSidecar:  pool.MemAdminOverhead,
				AutoCert:    boolToStr(pool.AutoCertificate),
			},
		},
	}

	return pkg, nil
}

func poolAppID(pool *models.V2Pool) string {
	return dcos.PoolAppID(*pool.Namespace, pool.Name)
}

func poolSecrets(pool *models.V2Pool) []dcos.CosmosSecretObj {
	var secrets []dcos.CosmosSecretObj
	for i, sc := range pool.Secrets {
		name := sc.Secret
		file := sc.File

		// Start indexes at 1 because mustache turns 0 into ""
		parsedSecret := dcos.CosmosSecretObj{
			Secret: name,
			File:   file,
			Index:  int32(i) + 1,
		}
		secrets = append(secrets, parsedSecret)
	}
	return secrets
}

func poolNetworks(pool *models.V2Pool) []dcos.CosmosNetworkObj {
	var networks []dcos.CosmosNetworkObj
	for i, net := range pool.VirtualNetworks {
		name := net.Name
		parsedLabels := []string{}
		for k, v := range net.Labels {
			parsedLabels = append(parsedLabels, fmt.Sprintf("%s:%s", k, v))
		}
		labelStr := strings.Join(parsedLabels, ",")

		// Start indexes at 1 because mustache turns 0 into ""
		parsedNetwork := dcos.CosmosNetworkObj{
			Name:   name,
			Labels: labelStr,
			Index:  int32(i) + 1,
		}
		networks = append(networks, parsedNetwork)
	}
	return networks
}

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func findPool(config *models.ConfigContainer, name string) (bool, int, *models.PoolContainer) {
	found := false
	var poolIndex int
	var pool models.PoolContainer
	for i, p := range config.Pools {
		if p.Name == name {
			pool = *p
			poolIndex = i
			found = true
		}
	}
	return found, poolIndex, &pool
}
