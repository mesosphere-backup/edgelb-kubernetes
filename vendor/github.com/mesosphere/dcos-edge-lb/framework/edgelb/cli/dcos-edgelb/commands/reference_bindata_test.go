package commands

import (
	"strings"
	"testing"
)

func TestReferenceLength(t *testing.T) {
	// This test is just here to ensure that the generated reference is
	// "probably" correct. And that correctness is just the length. go-bindata
	// may also not have the asset (incorrect path, etc.), which this will
	// catch.

	// Use `wc -l swagger.yml` or something
	expectedLen := 800

	b, err := reference()
	if err != nil {
		t.Fatal(err)
	}
	actualLen := len(strings.Split(string(b), "\n"))

	if actualLen < expectedLen {
		t.Fatalf("expected at least %d lines, only got %d", expectedLen, actualLen)
	}
}
