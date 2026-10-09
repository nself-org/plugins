package sched

import (
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"testing"
)

func invariantCase(f invariants.World) (World, Job, Runner) {
	r := fixtureRunner("node", "lan")
	r.Decl.Isolation = f.Isolation
	r.Decl.Network = f.Network
	r.Decl.Ownership = f.Ownership
	r.Decl.Accepts = []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}
	j := fixtureJob("invariant")
	j.Trust = f.Trust
	j.Network = f.Network
	j.SecretClasses = f.Secrets
	w := fixtureWorld(r)
	return w, j, r
}
func checkInvariant(t *testing.T, kind string) {
	invariants.Check(t, func(f invariants.World) error {
		w, j, r := invariantCase(f)
		switch kind {
		case "I03":
			r.Location = "hosted"
			r.Class = "included"
			r.Decl.Hosted = true
			r.Provider = "provider"
			w.Runners = []Runner{r}
			w.Allowances = map[string]Allowance{"provider": {RemainingEstimated: 1000000, Known: true, CeilingKnown: true}}
		case "I10":
			w.PreviousIsolation = map[string]model.Isolation{j.Key: "vm"}
		case "I19":
			r.Class = "metered"
			w.Runners = []Runner{r}
			w.Costs = map[string]Cost{"node": {Class: "metered", RateMicro: 1, BillingIncrementMs: 60000}}
			w.Policy.PaidEnabled = true
		}
		got := Place(w, []Job{j})
		placed := len(got.Placements) > 0
		switch kind {
		case "I01", "I02":
			if placed && len(trust.Admissible(trust.JobFacts{Trust: j.Trust, Network: j.Network, SecretClasses: j.SecretClasses, PersonalOwnRun: j.PersonalOwnRun}, r.Decl, w.Policy.Trust)) > 0 {
				return fmt.Errorf("%s admitted rejected trust facts %+v", kind, f)
			}
		case "I03":
			if placed {
				return fmt.Errorf("hosted private infrastructure admitted %+v", f)
			}
		case "I10":
			if placed && trust.RankIsolation(r.Decl.Isolation) < trust.RankIsolation("vm") {
				return fmt.Errorf("retry lowered isolation %+v", f)
			}
		case "I19":
			if placed {
				return fmt.Errorf("paid admitted with no budget %+v", f)
			}
		}
		return nil
	})
}
func TestInvariant_I01(t *testing.T) { checkInvariant(t, "I01") }
func TestInvariant_I02(t *testing.T) { checkInvariant(t, "I02") }
func TestInvariant_I03(t *testing.T) { checkInvariant(t, "I03") }
func TestInvariant_I10(t *testing.T) { checkInvariant(t, "I10") }
func TestInvariant_I19(t *testing.T) { checkInvariant(t, "I19") }
