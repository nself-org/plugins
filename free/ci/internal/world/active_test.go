package world

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// TestMakeRunnerActiveMeansObservedUse pins D-1266: Runner.Active is interactive use observed by the
// agent, not the online flag, so an idle online runner does not read as background contention.
func TestMakeRunnerActiveMeansObservedUse(t *testing.T) {
	str := func(s string) *string { return &s }
	for _, tc := range []struct {
		name        string
		state, used *string
		want        bool
	}{
		{"online-idle", str("online"), str("idle"), false},
		{"online-unknown", str("online"), nil, false},
		{"online-active", str("online"), str("active"), true},
		{"offline-active", str("offline"), str("active"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c model.Capability
			c.Availability.State.Value = tc.state
			c.Availability.Interactive.Value = tc.used
			if got := makeRunner(c, false, trust.CoordinatorMode(""), store.SchedWorld{}).Active; got != tc.want {
				t.Fatalf("Active = %v, want %v", got, tc.want)
			}
		})
	}
}
