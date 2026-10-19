package run

// Purpose: D-1123 pins the schema rule that an unbound evidence document is
// never `current` (internal/model/schema.go: binding unbound => freshness not
// current). A dirty tree or a missing commit leaves the binding unbound.
// Constraints: pure unit test of buildEvidence.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestBuildEvidenceFreshnessFollowsBinding(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name         string
		in           evidenceInput
		wantBinding  model.BindingState
		wantNotFresh bool
	}{
		{"clean tree is bound and current", evidenceInput{Revision: sha}, "bound", false},
		{"dirty tree is unbound and not current", evidenceInput{Revision: sha, Dirty: true}, "unbound", true},
		{"no commit is unbound and not current", evidenceInput{}, "unbound", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ev, _, err := buildEvidence(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if ev.Binding.State != c.wantBinding {
				t.Fatalf("binding %q, want %q", ev.Binding.State, c.wantBinding)
			}
			if got := ev.Freshness.State == "current"; got == c.wantNotFresh {
				t.Fatalf("binding %q with freshness %q", ev.Binding.State, ev.Freshness.State)
			}
			if c.wantNotFresh && ev.Freshness.Reason == nil {
				t.Fatalf("a non-current freshness must say why: %+v", ev.Freshness)
			}
		})
	}
}
