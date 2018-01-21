package commands

import (
	"testing"

	models "github.com/mesosphere/dcos-edge-lb/apiserver/models"
)

func TestCheckExamplesPoolHTTPHTTPS(t *testing.T) {
	jsonConfig, jsonErr := loadPoolContainer("../../../../../examples/config/pool-http.json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	yamlConfig, yamlErr := loadPoolContainer("../../../../../examples/config/pool-http.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if err := models.CheckPoolContainer(jsonConfig); err != nil {
		t.Fatal(err)
	}
	if err := models.CheckPoolContainer(yamlConfig); err != nil {
		t.Fatal(err)
	}

	jsonConfig, jsonErr = loadPoolContainer("../../../../../examples/config/pool-https.json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	yamlConfig, yamlErr = loadPoolContainer("../../../../../examples/config/pool-https.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if err := models.CheckPoolContainer(jsonConfig); err != nil {
		t.Fatal(err)
	}
	if err := models.CheckPoolContainer(yamlConfig); err != nil {
		t.Fatal(err)
	}
}

func TestCheckExamplesPoolMisc(t *testing.T) {
	jsonConfig, jsonErr := loadPoolContainer("../../../../../examples/config/pool-arangodb3.json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	yamlConfig, yamlErr := loadPoolContainer("../../../../../examples/config/pool-arangodb3.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if err := models.CheckPoolContainer(jsonConfig); err != nil {
		t.Fatal(err)
	}
	if err := models.CheckPoolContainer(yamlConfig); err != nil {
		t.Fatal(err)
	}

	jsonConfig, jsonErr = loadPoolContainer("../../../../../examples/config/sample-certificates.json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	yamlConfig, yamlErr = loadPoolContainer("../../../../../examples/config/sample-certificates.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if err := models.CheckPoolContainer(jsonConfig); err != nil {
		t.Fatal(err)
	}
	if err := models.CheckPoolContainer(yamlConfig); err != nil {
		t.Fatal(err)
	}
}

func TestCheckExamplesSampleMinimal(t *testing.T) {
	yamlConfig, yamlErr := loadPoolContainer("../../../../../examples/config/sample-minimal.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if err := models.CheckPoolContainer(yamlConfig); err != nil {
		t.Fatal(err)
	}
}

func TestCheckExamplesVirtualNetworks(t *testing.T) {
	yamlConfig, yamlErr := loadPoolContainer("../../../../../examples/config/pool-virtual-networks.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if err := models.CheckPoolContainer(yamlConfig); err != nil {
		t.Fatal(err)
	}
}
