package config

import (
	"context"
	"fmt"
	"time"

	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/models"
	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
)

const (
	defaultTimeout = time.Second * 60
)

// State represents the current state of the request handler server
type State struct {
	manager        Manager
	mkCosmosClient func() *dcos.CosmosClient
	mkPoolClient   func(string, string) *dcos.PoolClient
	mkFileWatcher  func(string, string, FileChangeHandleFn) (Watcher, error)
	mut            util.ChanMutex
	cfgFilename    string

	// Map from pool name to other metadata. This should always be up to date
	// with the pools that are currently tracked by the apiserver.
	pools map[string]*poolInfo
}

var logger = util.Logger

// NewState creates a new request handler server
func NewState(
	manager Manager,
	cfgFilename string,
	mkCosmosClient func() *dcos.CosmosClient,
	mkPoolClient func(string, string) *dcos.PoolClient,
	mkFileWatcher func(string, string, FileChangeHandleFn) (Watcher, error)) (*State, error) {
	// XXX We should have a test where we restart the apiserver and check that
	// we recover all the correct artifacts, watchers, and such from zk.

	s := &State{
		manager:        manager,
		mkCosmosClient: mkCosmosClient,
		mkPoolClient:   mkPoolClient,
		mkFileWatcher:  mkFileWatcher,
		mut:            util.NewChanMutex(),
		cfgFilename:    cfgFilename,
		pools:          make(map[string]*poolInfo),
	}

	ctx := context.Background()

	if err := s.manager.InitFile(ctx, s.cfgFilename, []byte{}); err != nil {
		return nil, fmt.Errorf("NewState filename %s: %s", s.cfgFilename, err)
	}

	config, getErr := s.Get()
	if getErr != nil {
		return nil, getErr
	}

	// Initialize
	// Effects:
	// restarts dcos-teplate
	// changes haproxy.poolName.cfg
	if err := s.putState(ctx, config); err != nil {
		return nil, err
	}

	// XXX: we should simply not delete the pool from zk until it is
	// gone from the cluster. Then we never have this error case. Current
	// blocker on this is that we only wait until cosmos responds to the
	// uninstall rather than checking ourselves that it's actually deleted.

	return s, nil
}

// TryLockState will return an error if it fails to acquire the lock
func (s *State) TryLockState(ctx context.Context) error {
	select {
	case <-s.mut.LockChan():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("could not acquire state lock")
	}
}

// UnlockState will unlock
func (s *State) UnlockState() {
	s.mut.Unlock()
}

// Get gets the current config
func (s *State) Get() (*models.ConfigContainer, error) {
	return s.readConfig()
}

// Put updates the config value in ZK
func (s *State) Put(ctx context.Context, config *models.ConfigContainer, token *string) error {
	// Write to zk before anything else, so if we crash we will try everything
	// over again.
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
	if err := s.putState(ctx, config); err != nil {
		return err
	}
	if err := s.putFrameworks(ctx, config, token); err != nil {
		return err
	}
	return nil
}

// Internal

func (s *State) putState(ctx context.Context, config *models.ConfigContainer) error {
	// This makes modifications to local pool state, and does so
	// without knowledge of what is in zk.

	for _, pool := range config.Pools {
		if err := s.putPoolState(ctx, pool); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) putFrameworks(ctx context.Context, config *models.ConfigContainer, token *string) error {
	// This makes modifications pool frameworks

	for _, pool := range config.Pools {
		if err := s.putPoolFramework(ctx, pool, token); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) writeConfig(ctx context.Context, config *models.ConfigContainer) error {
	b, err := models.ConfigContainerToMixedBytes(config)
	if err != nil {
		return err
	}
	return s.writeFileBytes(ctx, s.cfgFilename, b)
}

func (s *State) readConfig() (*models.ConfigContainer, error) {
	b, err := s.readFileBytes(s.cfgFilename)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		b = []byte(`{}`)
	}
	config, err := models.ConfigContainerFromMixedBytes(b)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (s *State) initFileBytes(ctx context.Context, path string, contents []byte) error {
	return s.manager.InitFile(ctx, path, contents)
}

func (s *State) readFileString(path string) (string, error) {
	b, err := s.readFileBytes(path)
	return string(b), err
}

func (s *State) readFileBytes(path string) ([]byte, error) {
	return s.manager.ReadFile(path)
}

func (s *State) writeFileBytes(ctx context.Context, path string, contents []byte) error {
	return s.manager.WriteFile(ctx, path, contents)
}
