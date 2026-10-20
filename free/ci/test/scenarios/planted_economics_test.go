//go:build scenarios && planted

package scenarios

// Purpose: the planted regressions of the economics scenarios (P7-SCHED-15 acceptance 5). Each test
// runs the SAME check function as the real scenario with a seam that has one defect injected in test
// code, and requires the check to report it. If a check function ever passes a defective seam, the
// scenario would pass for the wrong reason.
// Inputs:  a lying placer, a loader that honours the revision, a cost seam that prices GitHub as metered.
// Outputs: assertions only.
// Constraints: nothing in production code is edited; run with -tags 'scenarios planted' -run TestPlanted.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// TestPlantedProvisionWithoutBudget: a placer whose world says paid spend is on with an open day budget
// marks a metered or ephemeral runner eligible although the operator policy had neither.
func TestPlantedProvisionWithoutBudget(t *testing.T) {
	lying := func(w sched.World, jobs []sched.Job) sched.Result {
		w.Policy.PaidEnabled = true
		w.Budgets.Day = sched.Budget{Set: true, Remaining: 1 << 40}
		return sched.Place(w, jobs)
	}
	if bad := econCheckNoPaidWithoutBudget(t, lying); len(bad) == 0 {
		t.Fatal("planted regression not detected: a paid placement without paid_enabled and a budget passed the check")
	}
}

// TestPlantedRevisionBudgetEnablesSpend: a loader that honours spend keys from the revision (it merges
// them into operator scope) makes a revision-set budget enable spend.
func TestPlantedRevisionBudgetEnablesSpend(t *testing.T) {
	honours := func(kind trust.Source, operator, planted map[string]string) (economics.Policy, error) {
		return econRealLoader(trust.SourceProject, mergeKV(operator, planted), nil)
	}
	if bad := econCheckSpendKeysIgnored(honours, trust.SourceRevision); len(bad) == 0 {
		t.Fatal("planted regression not detected: a revision-set budget enabled spend and the check passed")
	}
	if bad := econCheckSpendKeysIgnored(honours, trust.SourceProtectedBranch); len(bad) == 0 {
		t.Fatal("planted regression not detected for the protected-branch scope")
	}
}

// TestPlantedGitHubMetered: a cost seam that prices the GitHub provider as metered.
func TestPlantedGitHubMetered(t *testing.T) {
	metered := func(p economics.Policy, runnerClass, provider string) sched.Cost {
		c := economics.CostFor(p, runnerClass, provider)
		if c.Class == economics.ClassIncluded {
			c.Class = economics.ClassMetered
		}
		return c
	}
	if bad := econCheckGitHubNeverMetered(metered); len(bad) == 0 {
		t.Fatal("planted regression not detected: GitHub priced as metered and the check passed")
	}
}
