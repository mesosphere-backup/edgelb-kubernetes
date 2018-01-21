package config

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"runtime/debug"
	"testing"

	"github.com/mesosphere/dcos-edge-lb/apiserver/dcos"
	"github.com/mesosphere/dcos-edge-lb/apiserver/models"
)

type testManager struct{ cache map[string]*[]byte }

func (m *testManager) FileExists(location string) (bool, error) {
	_, ok := m.cache[location]
	return ok, nil
}
func (m *testManager) InitFile(ctx context.Context, location string, defaultValue []byte) error {
	m.cache[location] = &defaultValue
	return nil
}
func (m *testManager) ReadFile(location string) ([]byte, error) {
	return *m.cache[location], nil
}
func (m *testManager) WriteFile(ctx context.Context, location string, newValue []byte) error {
	m.cache[location] = &newValue
	return nil
}
func (m *testManager) DeleteFile(ctx context.Context, location string) error {
	delete(m.cache, location)
	return nil
}

type testDcosClient struct{}

func (c *testDcosClient) WithToken(token *string) dcos.Client {
	return c
}
func (c *testDcosClient) WithAuth(auth *dcos.AuthCreds) dcos.Client {
	return c
}
func (c *testDcosClient) WithURL(dcosURL string) dcos.Client {
	return c
}
func (c *testDcosClient) HTTPExecute(ctx context.Context, mkReq func() (*http.Request, error), shouldRetry func(*http.Response) bool) (*http.Response, error) {
	return &http.Response{
		StatusCode: 404,
		Body:       nopCloser{bytes.NewBufferString("")},
	}, nil
}
func (c *testDcosClient) CreateRequest(method, path, payload string) (*http.Request, error) {
	return nil, nil
}
func makeTestClientFn() func() dcos.Client {
	return func() dcos.Client {
		return &testDcosClient{}
	}
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

const (
	config1 = `
{
	"pools": [{
                "name": "test-pool",
		"count": 1,
		"haproxy": {
			"frontends": [{
				"bindPort": 80
			}]
		}
	}]
}`
	pool1 = `
{
        "name": "test-pool2",
	"count": 2,
	"haproxy": {
		"frontends": [{
			"bindPort": 80
		}]
	}
}`
	pool2 = `
{
        "name": "test-pool2",
	"count": 3,
	"haproxy": {
		"frontends": [{
			"bindPort": 80
		}]
	}
}`
)

func makeTestState(ctx context.Context, t *testing.T, dirname string) (*State, error) {
	manager := &testManager{cache: map[string]*[]byte{}}
	if err := manager.InitFile(ctx, "templates/haproxy.cfg.ctmpl", []byte{}); err != nil {
		t.Fatalf("error initializing config manager: %s", err)
	}
	mkClient := makeTestClientFn()
	mkCosmosClient := dcos.MakeCosmosClientFn(mkClient, "", "")
	mkPoolClient := dcos.MakePoolClientFn(mkClient)
	mkFileWatcher := MakeFileWatcherFn(dirname)
	return NewState(manager, "cfgcache.json", mkCosmosClient, mkPoolClient, mkFileWatcher)
}

func cleanupTestState(dirname string) error {
	return os.RemoveAll(dirname)
}

func TestPutConfig(t *testing.T) {
	origConf := &models.V1Config{}
	origP2 := &models.V1Pool{}

	err := origConf.UnmarshalBinary([]byte(config1))
	checkError(t, err, "error parsing test config")
	ctx := context.Background()
	dirname, err := ioutil.TempDir(os.TempDir(), "config_test-")
	if err != nil {
		t.Fatal(err)
	}
	state, err := makeTestState(ctx, t, dirname)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanErr := cleanupTestState(dirname); cleanErr != nil {
			t.Fatal(cleanErr)
		}
	}()
	conf := models.ContainerFromV1Config(origConf)
	checkError(t, state.Put(ctx, conf, nil), "error storing config")

	conf, _ = state.Get()
	checkBool(t, len(conf.Pools) != 1, "length of pools was incorrect")

	err = origP2.UnmarshalBinary([]byte(pool1))
	checkError(t, err, "error parsing test pool")

	p2 := models.ContainerFromV1Pool(origP2)
	err = state.PutPool(ctx, p2, nil)
	checkError(t, err, "error storing config")

	conf, _ = state.Get()
	checkBool(t, len(conf.Pools) != 2, "length of pools was incorrect")

	p2, _ = state.GetPool(p2.Name)
	checkBool(t, *p2.V1.Count != 2, "num instances was incorrect")

	err = origP2.UnmarshalBinary([]byte(pool2))
	checkError(t, err, "error parsing test pool")

	p2 = models.ContainerFromV1Pool(origP2)
	err = state.PutPool(ctx, p2, nil)
	checkError(t, err, "error storing config")

	conf, _ = state.Get()
	checkBool(t, len(conf.Pools) != 2, "length of pools was incorrect")

	p2, _ = state.GetPool(p2.Name)
	checkBool(t, *p2.V1.Count != 3, "num instances was incorrect")

	err = state.DeletePool(ctx, "test-pool2", nil)
	checkError(t, err, "error deleting config")

	conf, _ = state.Get()
	checkBool(t, len(conf.Pools) != 1, "length of pools was incorrect")
	checkBool(t, conf.Pools[0].Name != "test-pool", "pools after delete were incorrect")
}

func checkError(t *testing.T, err error, message string) {
	if err != nil {
		t.Fatalf("%s: %s\n%s", message, err, string(debug.Stack()))
	}
}

func checkBool(t *testing.T, b bool, message string) {
	if b {
		t.Fatalf("%s\n%s", message, string(debug.Stack()))
	}
}
