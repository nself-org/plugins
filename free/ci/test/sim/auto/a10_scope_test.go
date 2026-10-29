package auto

// Purpose: A10 scope proof: the commit-CI pool rule keeps the personal VPS out and the coordinator
// pool in, and a dev run reaches the personal node only when the person lists its pool.
// Inputs: pools.Resolve and pools/record.Excluded. Outputs: pass or fail. Constraints: pure.

import (
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/pools/record"
)

func TestConformanceA10Scope(t *testing.T) {
	runners := []pools.Runner{
		{ID: "vps", Source: pools.SourceRegistry, Assigned: []string{"pool.mine"}, Projects: []string{"private"}},
		{ID: "team-node", Source: pools.SourceCoordinator, Assigned: []string{"pool.team"}, Projects: []string{"private"}},
	}
	excluded := record.Excluded(pools.List{Set: true, IDs: []string{"team"}}, runners)
	if len(excluded["vps"]) == 0 || excluded["vps"][0].Detail != record.Reason || len(excluded["team-node"]) != 0 {
		t.Fatalf("commit CI pool rule: %v", excluded)
	}
	for _, tc := range []struct {
		dev  []string
		want bool
	}{{[]string{"team"}, false}, {[]string{"mine", "team"}, true}} {
		got := pools.Resolve(pools.Input{Runners: runners, Project: "private", Dev: true, DevPools: pools.List{Set: true, IDs: tc.dev}})
		if slices.Contains(got.Eligible, "vps") != tc.want {
			t.Fatalf("dev pools %v: %+v", tc.dev, got)
		}
		if !tc.want && got.Excluded["vps"][0].Detail != pools.DetailDevNotAllowed {
			t.Fatalf("excluded = %v", got.Excluded)
		}
	}
}
