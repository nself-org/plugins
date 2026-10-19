package sched

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// secretsRefusal reports whether the eligibility reasons carry the node-not-secrets-eligible refusal.
func secretsRefusal(rs []model.PlacementReasonDetail) bool {
	for _, r := range rs {
		if r.Code == string(trust.SecretsClassNotDeclared) && r.Detail == "node is not secrets-eligible" {
			return true
		}
	}
	return false
}

// D-1101: a secret-bearing job may reach a node that is not this machine only when the node can take values.
func TestSecretJobNeedsSecretsEligibleNode(t *testing.T) {
	job := fixtureJob("j")
	job.SecretClasses = []model.SecretClass{"project"}
	operator := "operator"
	for name, c := range map[string]struct {
		loc      string
		eligible bool
		owner    *string
		want     bool
	}{
		"lan node, not eligible":     {"lan", false, &operator, true},
		"lan node, eligible":         {"lan", true, &operator, false},
		"lan node, eligible foreign": {"lan", true, ptr("third-party"), true},
		"this machine":               {"local", false, nil, false},
	} {
		r := fixtureRunner("n", c.loc)
		r.Decl.SecretClasses = []model.SecretClass{"project"}
		r.Capability.SecretsEligible = c.eligible
		r.Capability.Identity.Ownership.Value = c.owner
		if got := secretsRefusal(eligibility(fixtureWorld(r), job, r, false)); got != c.want {
			t.Errorf("%s: refused = %v, want %v", name, got, c.want)
		}
	}
	plain := fixtureJob("p")
	r := fixtureRunner("n", "lan")
	if secretsRefusal(eligibility(fixtureWorld(r), plain, r, false)) {
		t.Error("a job without secrets was refused for the secrets rule")
	}
}
