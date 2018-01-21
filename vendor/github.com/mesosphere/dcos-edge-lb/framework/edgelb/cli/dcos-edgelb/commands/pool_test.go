package commands

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"
)

func TestParseLbInfo(t *testing.T) {
	fileB, fileErr := ioutil.ReadFile("podinfo.json")
	if fileErr != nil {
		t.Fatal(fileErr)
	}
	parseI, parseErr := parsePodInfo(fileB, "edgelb-pool-0")
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	parseB, marhshalErr := json.Marshal(parseI)
	if marhshalErr != nil {
		t.Fatal(marhshalErr)
	}
	expectB, expectErr := ioutil.ReadFile("TestParseLbInfo.json")
	if expectErr != nil {
		t.Fatal(expectErr)
	}

	fmt.Printf("Parsed:\n%s\n", parseB)
	fmt.Printf("Expected:\n%s\n", expectB)

	parsed := map[string]interface{}{}
	if err := json.Unmarshal(parseB, &parsed); err != nil {
		t.Fatal(err)
	}
	expected := map[string]interface{}{}
	if err := json.Unmarshal(expectB, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed, expected) {
		t.Fatalf("not equal\nparsed:   %v\nexpected: %v", parsed, expected)
	}
}
