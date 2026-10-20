package run

// Purpose: a foreground run drops provider-hosted and ephemeral runners from its placement world; a run
//          a coordinator process owns keeps them (D-1394).
// Inputs:  hand-built worlds. Outputs: pass or fail. Constraints: no store, no clock.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func TestDropProviderRunners(t *testing.T) {
	node := sched.Runner{ID: "node-1"}
	hosted := sched.Runner{ID: "gh", Location: "hosted"}
	ephemeral := sched.Runner{ID: "vm", Ephemeral: true}
	for _, tc := range []struct {
		name        string
		in          sched.World
		await       bool
		wantRunners []string
		wantChanged bool
	}{
		{"nodes only: nothing changes", sched.World{Runners: []sched.Runner{node}}, false, []string{"node-1"}, false},
		{"hosted runner in Runners", sched.World{Runners: []sched.Runner{node, hosted}}, false, []string{"node-1"}, true},
		{"ephemeral runner", sched.World{Runners: []sched.Runner{ephemeral, node}}, false, []string{"node-1"}, true},
		{"hosted list only", sched.World{Runners: []sched.Runner{node}, Hosted: []sched.Runner{hosted}}, false, []string{"node-1"}, true},
		{"empty world", sched.World{}, false, nil, false},
		{"a coordinator-owned run keeps hosted runners", sched.World{Runners: []sched.Runner{node, hosted}}, true, []string{"node-1", "gh"}, false},
		{"a coordinator-owned run keeps ephemeral runners", sched.World{Runners: []sched.Runner{ephemeral, node}}, true, []string{"vm", "node-1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := dropProviderRunners(tc.in, tc.await)
			if changed != tc.wantChanged || changed && len(got.Hosted) != 0 {
				t.Fatalf("changed=%v hosted=%d", changed, len(got.Hosted))
			}
			var ids []string
			for _, r := range got.Runners {
				ids = append(ids, r.ID)
			}
			if len(ids) != len(tc.wantRunners) {
				t.Fatalf("runners %v, want %v", ids, tc.wantRunners)
			}
			for i := range ids {
				if ids[i] != tc.wantRunners[i] {
					t.Fatalf("runners %v, want %v", ids, tc.wantRunners)
				}
			}
		})
	}
}
