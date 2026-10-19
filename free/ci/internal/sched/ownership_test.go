package sched

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// TestEligibilityUnknownOwnershipIsOperatorOwned pins D-1266: a paired node reports no ownership
// fact, so unknown ownership must be treated as operator-owned, the same fail-closed rule the
// background admission step uses. Unknown power and unknown interactive state then refuse
// background work at the placement stage.
func TestEligibilityUnknownOwnershipIsOperatorOwned(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Decl.Ownership = ""
	r.Capability.Identity.Ownership.Value = nil
	r.Capability.Availability.PluggedIn.Value = nil
	r.Capability.Availability.Interactive.Value = nil
	j := fixtureJob("j")
	j.Pipeline.Band = "background"
	got := eligibility(fixtureWorld(r), j, r, false)
	for _, code := range []model.PlacementReason{model.NodeOnBattery, model.NodeInteractive} {
		if !containsCode(got, string(code)) {
			t.Fatalf("unknown ownership admitted background work: missing %s in %+v", code, got)
		}
	}
}

// TestEligibilityUnpluggedRefusalIsBackgroundOnly pins D-1266: an unplugged node refuses
// background work only; foreground work is not a battery concern at the placement stage.
func TestEligibilityUnpluggedRefusalIsBackgroundOnly(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Capability.Availability.PluggedIn.Value = ptr(false)
	fg := fixtureJob("fg")
	if containsCode(eligibility(fixtureWorld(r), fg, r, false), string(model.NodeOnBattery)) {
		t.Fatal("foreground job refused on battery at placement")
	}
	bg := fixtureJob("bg")
	bg.Pipeline.Band = "background"
	if !containsCode(eligibility(fixtureWorld(r), bg, r, false), string(model.NodeOnBattery)) {
		t.Fatal("background job not refused on battery")
	}
}
