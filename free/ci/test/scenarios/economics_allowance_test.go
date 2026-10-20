//go:build scenarios

package scenarios

// Purpose: Epic acceptance 5, included capacity and GitHub. (a) `included` private hosted capacity is
// used only while remaining - margin is strictly above the job's worst case, in the scheduler and at
// admission, and an unknown allowance is ineligible; (b) GitHub-hosted capacity is never `metered` or
// `ephemeral`, whatever the rate table, the runner's own class claim or the spelling of its id says.
// Inputs:  economics.AllowanceOf, economics.CostFor, sched.Place, dispatch.Admit with the budget step.
// Outputs: econCheckGitHubNeverMetered, which returns the violations (empty = holds).
// Constraints: a 60 s job at 100 micro per minute costs 100 micro; margin 50 000, ceiling 1 000 000.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const (
	econIncludedCeiling = 1_000_000
	econMargin          = 50_000
	econIncludedJob     = 100 // 60 s at 100 micro per minute
)

func econIncludedKV() map[string]string {
	return map[string]string{
		"economics.rates.github-hosted":    `{"class":"included","rate_per_minute_micro":100,"billing_increment":"1m"}`,
		"economics.ceilings.github-hosted": fmt.Sprint(econIncludedCeiling), schedpolicy.KeyMargin: fmt.Sprint(econMargin),
	}
}

// TestEconomicsIncludedStopsAtTheMargin: remaining - margin must exceed the worst case; equal refuses.
func TestEconomicsIncludedStopsAtTheMargin(t *testing.T) {
	p := econPolicy(t, trust.SourceProject, econIncludedKV(), nil)
	cases := []struct {
		remaining int64
		want      bool
	}{{econMargin + econIncludedJob - 1, false}, {econMargin + econIncludedJob, false}, {econMargin + econIncludedJob + 1, true}, {0, false}}
	for _, c := range cases {
		a := economics.AllowanceOf(p, "github-hosted", store.AllowanceInputs{HasImport: true, Imported: econIncludedCeiling - c.remaining})
		if !a.Known || a.Remaining != c.remaining || a.Confidence != economics.ConfEstimated || a.ToSched(p.MarginMicro).MarginMicro != econMargin {
			t.Fatalf("allowance for remaining %d: %+v", c.remaining, a)
		}
		r := econPaid("inc", "included", "github-hosted")
		w := econPlaceWorld(p, r)
		w.Allowances = map[string]sched.Allowance{"github-hosted": a.ToSched(p.MarginMicro)}
		j := job("j", "linux/amd64")
		j.TimeoutMs = 60_000
		res := sched.Place(w, []sched.Job{j})
		if placed := len(res.Placements) == 1; placed != c.want {
			t.Fatalf("remaining %d (margin %d, job %d): placed=%v want %v", c.remaining, econMargin, econIncludedJob, placed, c.want)
		}
		if !c.want && !hasCode(res.Explanations[0].Alternatives[0].Reasons, string(model.AllowanceBelowMargin)) {
			t.Fatalf("remaining %d: reasons %v, want allowance.below_margin", c.remaining, codes(res.Explanations[0].Alternatives[0].Reasons))
		}
	}
	// no ceiling or no imported consumption: unknown, ineligible
	for _, a := range []sched.Allowance{{Known: false, CeilingKnown: true, RemainingEstimated: econIncludedCeiling}, {Known: true, CeilingKnown: false}} {
		w := econPlaceWorld(p, econPaid("inc", "included", "github-hosted"))
		w.Allowances = map[string]sched.Allowance{"github-hosted": a}
		j := job("j", "linux/amd64")
		j.TimeoutMs = 60_000
		if res := sched.Place(w, []sched.Job{j}); len(res.Placements) != 0 || !hasCode(res.Explanations[0].Alternatives[0].Reasons, string(model.AllowanceUnknown)) {
			t.Fatalf("unknown allowance %+v placed or lacks allowance.unknown: %+v", a, res)
		}
	}
}

// TestEconomicsIncludedAdmissionStopsAtTheMargin does the same at the admission step, with usage the
// provider reported (imported) and the coordinator's own ledger as local consumption.
func TestEconomicsIncludedAdmissionStopsAtTheMargin(t *testing.T) {
	for _, c := range []struct {
		imported int64
		want     string
	}{{econIncludedCeiling - econMargin - econIncludedJob - 1, ""}, {econIncludedCeiling - econMargin - econIncludedJob, string(model.AllowanceBelowMargin)}, {econIncludedCeiling, string(model.AllowanceBelowMargin)}} {
		clk := newClock()
		st := econStore(t, clk)
		env := econEnv(st, econPolicy(t, trust.SourceProject, econIncludedKV(), nil), clk)
		a := queueAttempt(t, st, "pipe", "project", "build")
		req := econRequest(a, "", "github-hosted", 60_000)
		if got := econRefusal(t, econAdmit(env, st, req)); got != string(model.AllowanceUnknown) {
			t.Fatalf("no imported consumption: refusal %q, want %q", got, model.AllowanceUnknown)
		}
		if err := economics.RecordUsage(economics.WithEnv(ctxT(t), env), economics.Usage{Key: "acct/2026-10-17/r", Provider: "github-hosted", Date: "2026-10-17", Minutes: 100, CostMicro: c.imported}); err != nil {
			t.Fatal(err)
		}
		if got := econRefusal(t, econAdmit(env, st, req)); got != c.want {
			t.Fatalf("imported %d: refusal %q, want %q", c.imported, got, c.want)
		}
	}
}

// econCost is the seam that prices a runner (economics.CostFor).
type econCost func(p economics.Policy, runnerClass, provider string) sched.Cost

// econCheckGitHubNeverMetered returns the violations of "a GitHub provider is never metered or
// ephemeral": under policies that declare it metered or ephemeral at a price, that enable paid spend
// with budgets, for every runner class claim and every spelling of the provider id.
func econCheckGitHubNeverMetered(cost econCost) []string {
	var bad []string
	spend := func(class string) economics.Policy {
		rate := schedpolicy.Rate{Class: class, PerHourMicro: 3_600_000, BillingIncrementMs: 60_000}
		ids := []string{"github-hosted", "github-jit", "github", "gh"}
		p := economics.Policy{PaidEnabled: true, Budgets: schedpolicy.Budgets{Day: schedpolicy.Budget{Micro: 1_000_000, Set: true}}, Rates: map[string]schedpolicy.Rate{}}
		for _, id := range ids {
			p.Rates[id] = rate
		}
		return p
	}
	for _, rateClass := range []string{"", "metered", "ephemeral", "included"} {
		for _, claim := range []string{"", "metered", "ephemeral", "included", "owned"} {
			for _, id := range []string{"github-hosted", "GitHub-Hosted", " github-jit ", "github", "gh"} {
				got := cost(spend(rateClass), claim, id)
				if got.Class == economics.ClassMetered || got.Class == economics.ClassEphemeral {
					bad = append(bad, fmt.Sprintf("provider %q (rate class %q, runner claims %q) priced as %s", id, rateClass, claim, got.Class))
				}
			}
		}
	}
	return bad
}

// TestEconomicsGitHubHostedIsNeverMetered: the cost seam, the policy validator and the admission step
// all hold the line.
func TestEconomicsGitHubHostedIsNeverMetered(t *testing.T) {
	if bad := econCheckGitHubNeverMetered(economics.CostFor); len(bad) > 0 {
		t.Fatalf("GitHub priced as paid:\n%s", strings.Join(bad, "\n"))
	}
	for _, id := range []string{"github-hosted", "github-jit"} { // the policy refuses to declare it
		for _, class := range []string{"metered", "ephemeral"} {
			raw := json.RawMessage(fmt.Sprintf(`{"class":%q,"rate_per_hour_micro":3600000}`, class))
			if err := schedpolicy.ValidateSpendKey("economics.rates."+id, raw); err == nil {
				t.Fatalf("economics.rates.%s class %s was accepted", id, class)
			}
		}
		if err := schedpolicy.ValidateSpendKey("economics.budgets.provider."+id, json.RawMessage(`1000`)); err == nil {
			t.Fatalf("a spend budget for %s was accepted", id)
		}
	}
	// a runner that claims metered, under a policy with spend on and a budget, still reserves no money
	clk := newClock()
	st := econStore(t, clk)
	env := econEnv(st, econPolicy(t, trust.SourceProject, econKV(nil), nil), clk)
	a := queueAttempt(t, st, "pipe", "project", "build")
	if got := econRefusal(t, econAdmit(env, st, econRequest(a, economics.ClassMetered, "github-hosted", 60_000))); got != string(model.AllowanceUnknown) {
		t.Fatalf("GitHub claiming metered: refusal %q, want %q (included, no ceiling)", got, model.AllowanceUnknown)
	}
	if held, _ := st.Reserved(ctxT(t), "day", "", "2026-10-18"); held != 0 {
		t.Fatalf("GitHub reserved %d micro", held)
	}
	// and in the scheduler the claim does not make it paid either
	r := econPaid("gh", "metered", "github-hosted")
	w := econPlaceWorld(econPolicy(t, trust.SourceProject, econKV(nil), nil), r)
	if c := w.Costs["gh"]; c.Class != economics.ClassIncluded {
		t.Fatalf("world cost class %q, want included", c.Class)
	}
}
