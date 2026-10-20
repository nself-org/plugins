package economics

// D-1280 regression: the run dispatcher reaches economics only through the entry points this
// package registers with internal/run, so a lost registration leaves every hook inert.

import (
	"testing"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func TestEconomicsRegistersWithTheRunDispatcher(t *testing.T) {
	if !cirun.SpendRegistered() {
		t.Fatal("economics did not register its entry points with the run dispatcher")
	}
}
