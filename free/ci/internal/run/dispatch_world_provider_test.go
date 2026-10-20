package run

// Purpose: a foreground run drops provider-hosted and ephemeral runners from its placement world; a run a
//          coordinator process owns keeps only the hosted runners of its own repository (D-1394): a job
//          must never reach another repository's runner, whose Actions logs may be public.
// Inputs:  hand-built worlds. Outputs: pass or fail. Constraints: no store, no clock.

import (
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func hostedRunner(id, repo string) sched.Runner {
	r := sched.Runner{ID: id, Location: "hosted"}
	r.Capability = model.Capability{}
	r.Capability.Identity.Labels = []string{"arch=amd64", repoLabel + repo}
	return r
}

func ids(rs []sched.Runner) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func TestDropProviderRunners(t *testing.T) {
	node := sched.Runner{ID: "node-1"}
	own := hostedRunner("gh-own", "acme/private-api")
	other := hostedRunner("gh-other", "acme/public-docs")
	unlabelled := sched.Runner{ID: "gh-unlabelled", Location: "hosted"}
	ephemeral := sched.Runner{ID: "vm", Ephemeral: true}
	for _, tc := range []struct {
		name        string
		in          sched.World
		repo        string
		wantRunners []string
		wantHosted  []string
		wantChanged bool
	}{
		{"nodes only: nothing changes", sched.World{Runners: []sched.Runner{node}}, "", []string{"node-1"}, nil, false},
		{"bare run drops every hosted runner", sched.World{Runners: []sched.Runner{node, own, other}}, "", []string{"node-1"}, nil, true},
		{"bare run drops ephemeral", sched.World{Runners: []sched.Runner{ephemeral, node}}, "", []string{"node-1"}, nil, true},
		{"bare run drops the hosted list", sched.World{Runners: []sched.Runner{node}, Hosted: []sched.Runner{own}}, "", []string{"node-1"}, nil, true},
		{"empty world", sched.World{}, "", nil, nil, false},
		{"coordinator run keeps its own repository only", sched.World{Runners: []sched.Runner{node, own, other}}, "acme/private-api", []string{"node-1", "gh-own"}, nil, true},
		{"the other repository's runner is never kept", sched.World{Runners: []sched.Runner{own, other}}, "acme/public-docs", []string{"gh-other"}, nil, true},
		{"repository names compare without case", sched.World{Hosted: []sched.Runner{own}}, "ACME/Private-API", nil, []string{"gh-own"}, false},
		{"a hosted runner with no repository label is dropped", sched.World{Runners: []sched.Runner{node, unlabelled}}, "acme/private-api", []string{"node-1"}, nil, true},
		{"coordinator run still drops ephemeral runners", sched.World{Runners: []sched.Runner{ephemeral, node, own}}, "acme/private-api", []string{"node-1", "gh-own"}, nil, true},
		{"hosted list is filtered too", sched.World{Hosted: []sched.Runner{own, other}}, "acme/private-api", nil, []string{"gh-own"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := dropProviderRunners(tc.in, tc.repo)
			if changed != tc.wantChanged {
				t.Fatalf("changed=%v, want %v", changed, tc.wantChanged)
			}
			if g := ids(got.Runners); !slices.Equal(g, tc.wantRunners) {
				t.Fatalf("runners %v, want %v", g, tc.wantRunners)
			}
			if g := ids(got.Hosted); !slices.Equal(g, tc.wantHosted) {
				t.Fatalf("hosted %v, want %v", g, tc.wantHosted)
			}
		})
	}
}
