package commands

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func TestLoadConfigYAMLJSONPoolHTTPHTTPS(t *testing.T) {
	jsonConfig, jsonErr := loadPoolContainer("../../../../../examples/config/pool-http.json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	yamlConfig, yamlErr := loadPoolContainer("../../../../../examples/config/pool-http.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if !reflect.DeepEqual(jsonConfig, yamlConfig) {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(spew.Sdump(yamlConfig), spew.Sdump(jsonConfig), false)
		t.Fatal(dmp.DiffPrettyText(diffs))
	}

	jsonConfig, jsonErr = loadPoolContainer("../../../../../examples/config/pool-https.json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	yamlConfig, yamlErr = loadPoolContainer("../../../../../examples/config/pool-https.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if !reflect.DeepEqual(jsonConfig, yamlConfig) {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(spew.Sdump(yamlConfig), spew.Sdump(jsonConfig), false)
		t.Fatal(dmp.DiffPrettyText(diffs))
	}
}

func TestLoadConfigYAMLJSONMisc(t *testing.T) {
	jsonConfig, jsonErr := loadPoolContainer("../../../../../examples/config/pool-arangodb3.json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	yamlConfig, yamlErr := loadPoolContainer("../../../../../examples/config/pool-arangodb3.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if !reflect.DeepEqual(jsonConfig, yamlConfig) {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(spew.Sdump(yamlConfig), spew.Sdump(jsonConfig), false)
		t.Fatal(dmp.DiffPrettyText(diffs))
	}

	jsonConfig, jsonErr = loadPoolContainer("../../../../../examples/config/sample-certificates.json")
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	yamlConfig, yamlErr = loadPoolContainer("../../../../../examples/config/sample-certificates.yaml")
	if yamlErr != nil {
		t.Fatal(yamlErr)
	}
	if !reflect.DeepEqual(jsonConfig, yamlConfig) {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(spew.Sdump(yamlConfig), spew.Sdump(jsonConfig), false)
		t.Fatal(dmp.DiffPrettyText(diffs))
	}
}
