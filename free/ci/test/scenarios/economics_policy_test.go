//go:build scenarios

package scenarios

// Purpose: Epic acceptance 5, untrusted spend keys. A budget, paid_enabled, rate or approval threshold
// declared in the revision under test (or on the protected branch, a pipeline or a job scope) enables
// nothing: spend keys come from operator scope only, the value is ignored with E671 and the policy the
// scheduler and the admission step read is the operator's.
// Inputs:  a policy loader seam (operator values, planted values, the planted scope kind), the typed
//          spend policy it returns.
// Outputs: econCheckSpendKeysIgnored, which returns the violations (empty = holds); the planted
//          regression in planted_economics_test.go feeds it a loader that honours the revision.
// Constraints: the real loader is schedpolicy.Spend over policy.Resolved built with trust.Registry.Merge.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// econLoader resolves the spend policy of operator values with planted values at scope kind.
type econLoader func(kind trust.Source, operator, planted map[string]string) (economics.Policy, error)

// econRealLoader is the production path.
func econRealLoader(kind trust.Source, operator, planted map[string]string) (economics.Policy, error) {
	res, err := econResolve(kind, operator, planted)
	if err != nil {
		return economics.Policy{}, err
	}
	return schedpolicy.Spend(res)
}

// econPlantedKV is what a hostile revision would declare to turn spend on.
func econPlantedKV() map[string]string {
	return map[string]string{
		schedpolicy.KeyPaidEnabled: `true`, schedpolicy.KeyBudgetDay: `999999999`, schedpolicy.KeyBudgetMonth: `999999999`,
		schedpolicy.KeyBudgetJob: `999999999`, schedpolicy.KeyApproval: `1`, schedpolicy.KeyMargin: `7`,
		"economics.rates.evilco": `{"class":"metered","rate_per_hour_micro":1,"billing_increment":"1m"}`,
	}
}

// econCheckSpendKeysIgnored returns the violations of "a spend key outside operator scope changes
// nothing", for each planted scope kind: a policy with spend off and no budget stays off with no budget
// and allows no provider; a policy with budgets keeps exactly its operator limits, threshold and rates.
func econCheckSpendKeysIgnored(load econLoader, kinds ...trust.Source) []string {
	var bad []string
	for _, kind := range kinds {
		off, err := load(kind, map[string]string{schedpolicy.KeyPaidEnabled: `false`, "economics.rates.cloudco": econKV(nil)["economics.rates.cloudco"]}, econPlantedKV())
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: loader failed: %v", kind, err))
			continue
		}
		if off.PaidEnabled || off.Budgets.Day.Set || off.Budgets.Month.Set || off.Budgets.Job.Set || off.ApprovalThresholdMicro.Set {
			bad = append(bad, fmt.Sprintf("%s: a planted scope turned spend on or set a budget: %+v", kind, off))
		}
		if schedpolicy.PaidAllowed(off, "cloudco") || schedpolicy.PaidAllowed(off, "evilco") {
			bad = append(bad, fmt.Sprintf("%s: a planted scope made a paid placement allowed", kind))
		}
		if _, ok := off.Rates["evilco"]; ok {
			bad = append(bad, fmt.Sprintf("%s: a planted rate entered the rate table", kind))
		}
		on, err := load(kind, econKV(map[string]string{schedpolicy.KeyMargin: `5000`}), econPlantedKV())
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: loader failed: %v", kind, err))
			continue
		}
		if on.Budgets.Day.Micro != 1_000_000 || on.Budgets.Month.Micro != 5_000_000 || on.Budgets.Job.Set || on.ApprovalThresholdMicro.Set || on.MarginMicro != 5000 || len(on.Rates) != 1 {
			bad = append(bad, fmt.Sprintf("%s: a planted scope changed the operator's budgets or rates: %+v", kind, on))
		}
	}
	return bad
}

// TestEconomicsRevisionBudgetEnablesNothing: a budget (and paid_enabled, rate, threshold) declared in
// the revision under test enables nothing, in the policy, in the scheduler and at admission.
func TestEconomicsRevisionBudgetEnablesNothing(t *testing.T) {
	if bad := econCheckSpendKeysIgnored(econRealLoader, trust.SourceRevision); len(bad) > 0 {
		t.Fatalf("revision-set budget enables spend:\n%s", strings.Join(bad, "\n"))
	}
	// operator: spend off, no budget; revision: everything on.
	operator := map[string]string{"economics.rates.cloudco": econKV(nil)["economics.rates.cloudco"]}
	p := econPolicy(t, trust.SourceRevision, operator, econPlantedKV())
	w := econPlaceWorld(p, econPaid("paid", "metered", "cloudco"))
	j := job("j", "linux/amd64")
	j.TimeoutMs = 60_000
	res := sched.Place(w, []sched.Job{j})
	if len(res.Placements) != 0 || !hasCode(res.Explanations[0].Alternatives[0].Reasons, string(model.PaidDisabled)) {
		t.Fatalf("the scheduler placed paid work on a revision budget: %+v", res)
	}
	clk := newClock()
	st := econStore(t, clk)
	a := queueAttempt(t, st, "pipe", "project", "build")
	if got := econRefusal(t, econAdmit(econEnv(st, p, clk), st, econRequest(a, "", "cloudco", 60_000))); got != string(model.PaidDisabled) {
		t.Fatalf("admission refusal %q, want %q", got, model.PaidDisabled)
	}
	if held, _ := st.Reserved(ctxT(t), "day", "", "2026-10-18"); held != 0 {
		t.Fatalf("a refused paid job reserved %d micro", held)
	}
}

// TestEconomicsSpendKeysOnProtectedBranchChangeNothing: the protected branch (and a pipeline or job
// scope) is not the operator either: its spend keys are ignored, and each ignored key is recorded as E671.
func TestEconomicsSpendKeysOnProtectedBranchChangeNothing(t *testing.T) {
	kinds := []trust.Source{trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob}
	if bad := econCheckSpendKeysIgnored(econRealLoader, kinds...); len(bad) > 0 {
		t.Fatalf("a spend key outside operator scope applied:\n%s", strings.Join(bad, "\n"))
	}
	for _, kind := range kinds {
		res, err := econResolve(kind, econKV(nil), econPlantedKV())
		if err != nil {
			t.Fatal(err)
		}
		ignored := map[string]bool{}
		for _, ig := range res.Effective.Ignored() {
			if ig.Scope == kind && strings.HasPrefix(ig.Reason, "E671") {
				ignored[ig.Key] = true
			}
		}
		for key := range econPlantedKV() {
			if !ignored[key] {
				t.Errorf("%s: %s was not recorded as an ignored change (E671): %v", kind, key, res.Effective.Ignored())
			}
		}
	}
}
