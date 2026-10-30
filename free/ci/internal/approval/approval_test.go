package approval

// Purpose: the operator-typed rule of the approval step (operatorTyped).
// Inputs: the stored approval state of a run and the class it is admitted at.
// Constraints: the step itself runs in the security suite against the real store; this pins the rule.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestOperatorTyped(t *testing.T) {
	cases := []struct {
		name     string
		facts    bool
		recorded string
		admitted model.TrustClass
		want     bool
	}{
		{"ad-hoc untrusted, admitted at owner, no facts", false, "untrusted", "owner", true},
		{"ad-hoc collaborator, admitted at internal", false, "collaborator", "internal", true},
		{"admitted at the recorded class: a stranger's run", false, "untrusted", "untrusted", false},
		{"admitted at collaborator, recorded untrusted", false, "untrusted", "collaborator", false},
		{"source facts stored: the source hook decides", true, "untrusted", "owner", false},
		{"nothing lower recorded", false, "owner", "owner", false},
		{"unknown recorded class", false, "bogus", "owner", false},
		{"no admitted class", false, "untrusted", "", false},
	}
	for _, c := range cases {
		st := store.ApprovalState{SourceTrust: c.recorded, Facts: c.facts}
		if got := operatorTyped(st, c.admitted); got != c.want {
			t.Errorf("%s: operatorTyped = %v, want %v", c.name, got, c.want)
		}
	}
}
