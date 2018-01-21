package config

import (
	"testing"

	"github.com/mesosphere/dcos-edge-lb/apiserver/models"
)

var (
	testConfig = `
{
    "pools": [
        {"name": "test1"},
        {"name": "test2"}
    ]
}`
	testPool = `
{
    "name": "test-http-pool",
    "haproxy": {
        "frontends": [{
            "name": "web_80"
        }],
        "backends": [{
            "name": "host-httpd",
            "rewrite_http": {"host": "myhost"},
            "servers": [{
                "port": {"name": "web"}
            }]
        }]
    }
}`
)

func TestDefaultConfig(t *testing.T) {
	var c1 models.V1Config
	if err := c1.UnmarshalBinary([]byte(testConfig)); err != nil {
		t.Fatal(err)
	}
	if len(c1.Pools) != 2 {
		t.Fatal("length of pools did not match")
	}
	for _, p := range c1.Pools {
		checkPool(t, p)
	}
}

func TestDefaultPool(t *testing.T) {
	var p1 models.V1Pool
	if err := p1.UnmarshalBinary([]byte(testPool)); err != nil {
		t.Fatal(err)
	}
	checkPool(t, &p1)
}

func checkPool(t *testing.T, p1 *models.V1Pool) {
	if *p1.Count < 1 {
		t.Errorf("pool instances were less than 1: %d", p1.Count)
	}
	if p1.Namespace == nil {
		t.Errorf("namespace was nil")
	}
	if *p1.Namespace == "" {
		t.Errorf("pool namespace wasn't set")
	}
}
