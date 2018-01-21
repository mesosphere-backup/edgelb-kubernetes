package version

import (
	"testing"
)

func TestVersion(t *testing.T) {
	// This shouldn't be the empty string because a value should be passed in
	// via the `ldflags "-X ..."` flag to go build/test/etc.
	if Version() == "" {
		t.Fatal("version is the empty string")
	}
}
