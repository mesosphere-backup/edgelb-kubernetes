package template

import (
	"testing"
)

func TestNewBrain(t *testing.T) {
	b := NewBrain()

	if b.data == nil {
		t.Errorf("expected data to not be nil")
	}

	if b.receivedData == nil {
		t.Errorf("expected receivedData to not be nil")
	}
}
