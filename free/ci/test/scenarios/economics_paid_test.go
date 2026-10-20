//go:build scenarios

package scenarios

// Purpose: Epic acceptance 5, paid capacity is off by default. Without `economics.paid_enabled` plus a
// day or month budget (or a provider budget) no metered or cloud-ephemeral placement happens: not in the
// scheduler (sched.Place) and not at admission (dispatch.Admit with the budget step). paid_enabled true
// with no budget is still ineligible (budget.unset). With both set the same job places (positive control).
// Inputs:  a placer seam (sched.Place), spend policies resolved by the real loader.
// Outputs: econCheckNoPaidWithoutBudget, which returns the violations (empty = holds).
// Constraints: ephemeral runners are TEAM coordinator runners (burst.team_only otherwise).

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

// econRates declares one metered provider (cloudco) and one ephemeral provider (vmco), 3.6M micro/hour.
func econRates() map[string]string {
	return map[string]string{
		"economics.rates.cloudco": `{"class":"metered","rate_per_hour_micro":3600000,"billing_increment":"1m"}`,
		"economics.rates.vmco":    `{"class":"ephemeral","rate_per_hour_micro":3600000,"billing_increment":"1m"}`,
	}
}

// econPaidCase is one operator policy under which no paid placement may happen.
type econPaidCase struct {
	name   string
	extra  map[string]string
	reason model.PlacementReason // what the admission step refuses with
}

func econUnpaidCases() []econPaidCase {
	return []econPaidCase{
		{"off, day budget", map[string]string{schedpolicy.KeyPaidEnabled: `false`, schedpolicy.KeyBudgetDay: `1000000`}, model.PaidDisabled},
		{"off, month budget", map[string]string{schedpolicy.KeyPaidEnabled: `false`, schedpolicy.KeyBudgetMonth: `5000000`}, model.PaidDisabled},
		{"off, no budget", map[string]string{schedpolicy.KeyPaidEnabled: `false`}, model.PaidDisabled},
		{"default (key unset), day budget", map[string]string{schedpolicy.KeyBudgetDay: `1000000`}, model.PaidDisabled},
		{"on, no budget", map[string]string{schedpolicy.KeyPaidEnabled: `true`}, model.BudgetUnset},
	}
}

func econPaidRunners() []sched.Runner {
	return []sched.Runner{econPaid("metered", economics.ClassMetered, "cloudco"), econPaid("vm", economics.ClassEphemeral, "vmco")}
}

// econPlacer is the seam that places jobs (sched.Place).
type econPlacer func(sched.World, []sched.Job) sched.Result

// econCheckNoPaidWithoutBudget returns the violations of "no metered or cloud-ephemeral placement
// without paid_enabled and a day or month budget": for each unpaid policy and each paid runner, the job
// must not be placed.
func econCheckNoPaidWithoutBudget(t testing.TB, place econPlacer) []string {
	var bad []string
	for _, c := range econUnpaidCases() {
		p := econPolicy(t, trust.SourceProject, mergeKV(econRates(), c.extra), nil)
		for _, r := range econPaidRunners() {
			j := job("j", "linux/amd64")
			j.TimeoutMs = 60_000
			if res := place(econPlaceWorld(p, r), []sched.Job{j}); len(res.Placements) != 0 {
				bad = append(bad, fmt.Sprintf("%s runner placed under %q: %+v", r.Class, c.name, res.Placements))
			}
		}
	}
	return bad
}

func mergeKV(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range []map[string]string{a, b} {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func TestEconomicsNoPaidPlacementWithoutPaidEnabledAndABudget(t *testing.T) {
	if bad := econCheckNoPaidWithoutBudget(t, sched.Place); len(bad) > 0 {
		t.Fatalf("paid placement without paid_enabled and a budget:\n%s", strings.Join(bad, "\n"))
	}
	// positive control: the same job places once spend is on with a budget, so the check is not vacuous
	on := econPolicy(t, trust.SourceProject, mergeKV(econRates(), map[string]string{schedpolicy.KeyPaidEnabled: `true`, schedpolicy.KeyBudgetMonth: `5000000`}), nil)
	for _, r := range econPaidRunners() {
		j := job("j", "linux/amd64")
		j.TimeoutMs = 60_000
		if res := sched.Place(econPlaceWorld(on, r), []sched.Job{j}); len(res.Placements) != 1 {
			t.Fatalf("%s runner under paid_enabled with a month budget: %+v", r.Class, res.Explanations)
		}
	}
	// the scheduler names the reason
	off := econPolicy(t, trust.SourceProject, mergeKV(econRates(), map[string]string{schedpolicy.KeyPaidEnabled: `true`}), nil)
	j := job("j", "linux/amd64")
	j.TimeoutMs = 60_000
	res := sched.Place(econPlaceWorld(off, econPaid("metered", economics.ClassMetered, "cloudco")), []sched.Job{j})
	if !hasCode(res.Explanations[0].Alternatives[0].Reasons, string(model.BudgetUnset)) {
		t.Fatalf("paid_enabled with no budget: reasons %v, want budget.unset", codes(res.Explanations[0].Alternatives[0].Reasons))
	}
}

// TestEconomicsNoPaidAdmissionWithoutPaidEnabledAndABudget: the admission step refuses the same cases
// with its own reason and reserves nothing.
func TestEconomicsNoPaidAdmissionWithoutPaidEnabledAndABudget(t *testing.T) {
	for _, c := range econUnpaidCases() {
		for _, r := range econPaidRunners() {
			clk := newClock()
			st := econStore(t, clk)
			env := econEnv(st, econPolicy(t, trust.SourceProject, mergeKV(econRates(), c.extra), nil), clk)
			a := queueAttempt(t, st, "pipe", "project", "build")
			req := econRequest(a, r.Class, r.Provider, 60_000)
			if got := econRefusal(t, econAdmit(env, st, req)); got != string(c.reason) {
				t.Fatalf("%s under %q: refusal %q, want %q", r.Class, c.name, got, c.reason)
			}
			if _, found, _ := st.LedgerAttempt(ctxT(t), a); found {
				t.Fatalf("%s under %q: a refused job wrote a ledger row", r.Class, c.name)
			}
		}
	}
	clk := newClock()
	st := econStore(t, clk)
	env := econEnv(st, econPolicy(t, trust.SourceProject, mergeKV(econRates(), map[string]string{schedpolicy.KeyPaidEnabled: `true`, schedpolicy.KeyBudgetDay: `1000000`}), nil), clk)
	a := queueAttempt(t, st, "pipe", "project", "build")
	if err := econAdmit(env, st, econRequest(a, economics.ClassMetered, "cloudco", 60_000)); err != nil {
		t.Fatalf("positive control: %v", err)
	}
}
