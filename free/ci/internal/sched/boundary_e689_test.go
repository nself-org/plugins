package sched

// Purpose: D-1250. A refusal at the production boundary names E689, so an operator can look up its Fix text.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestBoundaryReasonsCarryE689(t *testing.T) {
	cases := map[string]struct {
		code trust.Reason
		set  func(*Runner, *Job)
	}{
		"deploy host": {trust.BoundaryDeployHost, func(r *Runner, _ *Job) { r.Decl.DeployHost.Matched = true }},
		"override minimum": {trust.BoundaryOverrideMinimum, func(r *Runner, j *Job) {
			r.Decl.DeployHost.Matched, r.Decl.DeployHost.Override = true, true
			j.Trust = "collaborator"
		}},
		"coordinator host": {trust.BoundaryCoordinatorHost, func(r *Runner, _ *Job) {
			r.CoordinatorHost, r.CoordinatorMode, r.Decl.Isolation = true, trust.Team, "container"
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r, j := fixtureRunner("a", "lan"), fixtureJob("job")
			c.set(&r, &j)
			got := Place(fixtureWorld(r), []Job{j})
			for _, q := range got.Explanations[0].Alternatives[0].Reasons {
				if q.Code == string(c.code) {
					if !strings.HasPrefix(q.Detail, "E689") {
						t.Fatalf("%s detail %q does not name E689", c.code, q.Detail)
					}
					return
				}
			}
			t.Fatalf("reason %s missing: %+v", c.code, got.Explanations[0].Alternatives[0].Reasons)
		})
	}
}
