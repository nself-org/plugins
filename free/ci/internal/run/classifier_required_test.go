package run

import (
	"context"
	"testing"
)

func TestMissingClassifierRefusesOwnerAdmission(t *testing.T) {
	admitted, recorded, err := (&Coordinator{}).classify(context.Background(), Hooks{}, Revision{})
	if err == nil || admitted != "" || recorded != "" {
		t.Fatalf("missing classifier granted %q/%q: %v", admitted, recorded, err)
	}
}
