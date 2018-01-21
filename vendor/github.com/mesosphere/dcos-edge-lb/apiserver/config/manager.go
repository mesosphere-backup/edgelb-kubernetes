package config

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/mesosphere/dcos-edge-lb/apiserver/util"
	"github.com/samuel/go-zookeeper/zk"
)

// Manager is an interface that exists for testing purposes
type Manager interface {
	FileExists(location string) (bool, error)
	InitFile(ctx context.Context, location string, defaultValue []byte) error
	ReadFile(location string) ([]byte, error)
	WriteFile(ctx context.Context, location string, newValue []byte) error
	DeleteFile(ctx context.Context, location string) error
}

// manager represents the current state of the config cache / zookeeper
type manager struct {
	// Zookeeper root node
	zkPath string

	// Base dir for files
	filePath string

	// Map of last received Zookeeper stats for each managed path.
	zkStats map[string]*zk.Stat

	// Zookeeper client
	zk *zk.Conn

	// This lock guards access to Zookeeper
	//
	// The reason for this, is because Zookeeper doesn't support timeouts,
	// so we wrap it in a lock that does
	zkChanMut util.ChanMutex
}

// NewManager creates a new request handler server
func NewManager(filePath, zkPath, zkServers string, zkTimeout time.Duration) (Manager, error) {
	zkConn, _, err := zk.Connect(strings.Split(zkServers, ","), zkTimeout)
	if err != nil {
		return nil, fmt.Errorf("NewManager zookeeper failed to connect %s: %s", zkServers, err)
	}

	m := &manager{
		filePath:  filePath,
		zkPath:    zkPath,
		zk:        zkConn,
		zkChanMut: util.NewChanMutex(),
		zkStats:   map[string]*zk.Stat{},
	}

	if err = m.initZk(m.zkPath, []byte{}); err != nil {
		return nil, fmt.Errorf("NewManager zookeeper failed to create node %s: %s", zkServers, err)
	}

	return m, nil
}

// FileExists checks if a file exists
// XXX: This code is duplicated in lbmgr.go, and we should in general be sharing more code
//      between apiserver and lbmgr. This could be done as part of a to flatten the entire project.
//      For example the calls from lbmgr to the apiserver should be using the generated swagger client
//      code. Apiserver, lbmgr, and the framework cli code should also use the same vendor dir.
func (m *manager) FileExists(location string) (bool, error) {
	fullPath := m.makeFilePath(location)
	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// InitFile create file in zk and cache using context
func (m *manager) InitFile(ctx context.Context, location string, defaultValue []byte) error {
	select {
	case <-ctx.Done():
		return errors.New("write context cancelled before write")
	case <-m.zkChanMut.LockChan():
		defer m.zkChanMut.Unlock()
		fullPath := m.makeZkPath(location)

		// We don't store default values in ZK because they will change upon
		// apiserver upgrades. Write an empty string to ZK and write the
		// defaultValue to local file.
		if err := m.initZk(fullPath, []byte{}); err != nil {
			return err
		}

		v, err := m.readZk(location)
		if err != nil {
			return err
		}

		if len(v) == 0 {
			v = defaultValue
		}

		if err := m.writeFile(location, v); err != nil {
			return err
		}
		return nil
	}
}

// ReadFile attempts to read a local file and errors if not found
func (m *manager) ReadFile(location string) ([]byte, error) {
	fullPath := m.makeFilePath(location)
	exists, err := m.FileExists(location)
	if err != nil {
		return nil, fmt.Errorf("error checking file %s, %s", location, err)
	}
	if !exists {
		return nil, fmt.Errorf("file %s not found", location)
	}
	b, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("error reading from %s: %s", fullPath, err)
	}
	return b, nil
}

// WriteFile to zk and cache using context
func (m *manager) WriteFile(ctx context.Context, location string, newValue []byte) error {
	select {
	case <-ctx.Done():
		return errors.New("write context cancelled before write")
	case <-m.zkChanMut.LockChan():
		defer m.zkChanMut.Unlock()
		if err := m.writeZk(location, newValue); err != nil {
			return err
		}
		if err := m.writeFile(location, newValue); err != nil {
			return err
		}
		return nil
	}
}

// DeleteFile attempts to delete a file from disk and zk and errors if not found
func (m *manager) DeleteFile(ctx context.Context, location string) error {
	select {
	case <-ctx.Done():
		return errors.New("delete context cancelled before write")
	case <-m.zkChanMut.LockChan():
		defer m.zkChanMut.Unlock()
		if err := m.deleteZk(location); err != nil {
			return err
		}
		if err := m.deleteFile(location); err != nil {
			return err
		}
		return nil
	}
}

// Internal

func (m *manager) makeZkPath(location string) string {
	return path.Join(m.zkPath, location)
}

func (m *manager) initZk(zkPath string, value []byte) error {
	exists, _, err := m.zk.Exists(zkPath)
	if err != nil {
		return fmt.Errorf("error checking existence zk data node: %s", err)
	}
	if !exists {
		logger.Debugf("creating new node %s in zookeeper", zkPath)
		acl := zk.WorldACL(zk.PermAll)
		if _, err = m.zk.Create(zkPath, value, int32(0), acl); err != nil {
			return fmt.Errorf("error creating zk data node: %s", err)
		}
	}
	return nil
}

func (m *manager) readZk(location string) ([]byte, error) {
	v, stat, err := m.zk.Get(m.makeZkPath(location))
	if err != nil {
		return nil, fmt.Errorf("read zk error: %s", err)
	}
	m.zkStats[location] = stat
	return v, nil
}

func (m *manager) writeZk(location string, newValue []byte) error {
	stat, err := m.zk.Set(m.makeZkPath(location), newValue, m.zkStats[location].Version)
	if err != nil {
		return fmt.Errorf("write zk error: %s", err)
	}
	m.zkStats[location] = stat
	return nil
}

func (m *manager) deleteZk(location string) error {
	fullPath := m.makeZkPath(location)
	exists, _, err := m.zk.Exists(fullPath)
	if err != nil {
		return fmt.Errorf("error checking existence zk data node: %s", err)
	}
	if !exists {
		return fmt.Errorf("zk node not found: %s", err)
	}

	err = m.zk.Delete(fullPath, m.zkStats[location].Version)
	if err != nil {
		return fmt.Errorf("error deleting zk node: %s", err)
	}
	return nil
}

func (m *manager) makeFilePath(location string) string {
	return path.Join(m.filePath, location)
}

// XXX: This code is duplicated in lbmgr.go, and we should in general be sharing more code
//      between apiserver and lbmgr. See FileExists for more info.
func (m *manager) writeFile(location string, b []byte) error {
	fullPath := m.makeFilePath(location)
	tmpfile := fmt.Sprintf("%s.tmp", fullPath)
	if err := ioutil.WriteFile(tmpfile, b, 0644); err != nil {
		return fmt.Errorf("error writing to %s: %s", tmpfile, err)
	}
	if err := os.Rename(tmpfile, fullPath); err != nil {
		return fmt.Errorf("error renaming %s to %s: %s", tmpfile, fullPath, err)
	}
	return nil
}

// DeleteFile attempts to delete a local file and errors if not found
func (m *manager) deleteFile(location string) error {
	fullPath := m.makeFilePath(location)
	exists, err := m.FileExists(location)
	if err != nil {
		return fmt.Errorf("error checking file %s, %s", location, err)
	}
	if !exists {
		return fmt.Errorf("file %s not found", location)
	}
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("error deleting %s: %s", fullPath, err)
	}
	return nil
}
