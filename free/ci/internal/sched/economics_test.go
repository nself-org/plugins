package sched

import (
	"math"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestPaidBudgetAndPessimism(t *testing.T) {
	r := fixtureRunner("paid", "lan")
	r.Class = "metered"
	j := fixtureJob("j")
	j.TimeoutMs = 120000
	w := fixtureWorld(r)
	w.Costs[r.ID] = Cost{Class: "metered", RateMicro: 2, BillingIncrementMs: 60000}
	w.Policy.PaidEnabled = true
	got := Place(w, []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.BudgetUnset)) {
		t.Fatalf("budget unset: %+v", got)
	}
	w.Budgets.Day = Budget{Set: true, Remaining: 120000}
	got = Place(w, []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.BudgetExceeded)) {
		t.Fatalf("budget bound: %+v", got)
	}
	w.Budgets.Day.Remaining = 240001
	got = Place(w, []Job{j})
	if len(got.Placements) != 1 || got.Placements[0].CostMicro != 240000 {
		t.Fatalf("pessimistic bound: %+v", got)
	}
	w.Costs[r.ID] = Cost{Class: "metered"}
	got = Place(w, []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.CostUnknownRate)) {
		t.Fatalf("unknown rate: %+v", got)
	}
}
func TestIncludedMarginAndPublic(t *testing.T) {
	r := fixtureRunner("included", "lan")
	r.Class = "included"
	r.Provider = "hosted"
	j := fixtureJob("j")
	w := fixtureWorld(r)
	w.Costs[r.ID] = Cost{Class: "included", RateMicro: 1, BillingIncrementMs: 60000}
	w.Allowances = map[string]Allowance{"hosted": {Known: true, CeilingKnown: true, RemainingEstimated: 65000, MarginMicro: 5000}}
	got := Place(w, []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.AllowanceBelowMargin)) {
		t.Fatalf("margin strictness: %+v", got)
	}
	w.Allowances["hosted"] = Allowance{Known: false, CeilingKnown: true}
	got = Place(w, []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.AllowanceUnknown)) {
		t.Fatalf("unknown: %+v", got)
	}
	r.Capability.Identity.Labels = append(r.Capability.Identity.Labels, "nself.visibility=public")
	w.Runners = []Runner{r}
	got = Place(w, []Job{j})
	if len(got.Placements) != 1 {
		t.Fatalf("public: %+v", got)
	}
}
func TestIncludedMarginSaturation(t *testing.T) {
	r := fixtureRunner("included", "lan")
	r.Class, r.Provider = "included", "hosted"
	w := fixtureWorld(r)
	w.Costs[r.ID] = Cost{Class: "included", RateMicro: 1, BillingIncrementMs: 60000}
	w.Allowances = map[string]Allowance{"hosted": {Known: true, CeilingKnown: true, RemainingEstimated: math.MinInt64, MarginMicro: math.MaxInt64}}
	got := Place(w, []Job{fixtureJob("j")})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.AllowanceBelowMargin)) {
		t.Fatalf("saturated margin admitted: %+v", got)
	}
	w.Allowances["hosted"] = Allowance{Known: true, CeilingKnown: true, RemainingEstimated: 1000000, MarginMicro: -1}
	got = Place(w, []Job{fixtureJob("j")})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.AllowanceBelowMargin)) {
		t.Fatalf("negative margin admitted: %+v", got)
	}
}
func TestEphemeralAndProvider(t *testing.T) {
	r := fixtureRunner("e", "remote")
	r.Ephemeral = true
	r.Used = true
	r.ProviderDown = true
	j := fixtureJob("j")
	rs := Place(fixtureWorld(r), []Job{j}).Explanations[0].Alternatives[0].Reasons
	for _, code := range []model.PlacementReason{model.EphemeralUsed, model.ProviderDown} {
		if !containsCode(rs, string(code)) {
			t.Fatalf("missing %s: %+v", code, rs)
		}
	}
}
func TestCacheLocality(t *testing.T) {
	r := fixtureRunner("r", "lan")
	j := fixtureJob("j")
	w := fixtureWorld(r)
	_, unknown := score(w, j, r)
	if unknown["cache"].Confidence != "unknown" || unknown["cache"].WeightPermille != 1000 {
		t.Fatal(unknown["cache"])
	}
	w.Cache = map[string]map[string]CacheFact{"r": {"j": {MissBytes: ptr(int64(2000000)), Confidence: "known"}}}
	bad, _ := score(w, j, r)
	w.Cache["r"]["j"] = CacheFact{MissBytes: ptr(int64(500000)), Confidence: "known"}
	good, parts := score(w, j, r)
	if good >= bad || parts["cache"].Confidence != "known" {
		t.Fatalf("cache scores %d %d %+v", bad, good, parts["cache"])
	}
}
func TestTrustReasonsPassThrough(t *testing.T) {
	r := fixtureRunner("runner", "local")
	r.Decl.Accepts = []model.TrustClass{"owner"}
	r.Decl.Isolation = "process"
	r.Decl.UIDSeparation = false
	j := fixtureJob("collaborator")
	j.Trust = "collaborator"
	j.Network = "lan"
	w := fixtureWorld(r)
	want := trust.Admissible(trust.JobFacts{Trust: j.Trust, Network: j.Network}, r.Decl, w.Policy.Trust)
	if len(want) == 0 {
		t.Fatal("fixture did not produce a trust refusal")
	}
	got := Place(w, []Job{j})
	if len(got.Placements) != 0 || len(got.Explanations) != 1 || len(got.Explanations[0].Alternatives) != 1 {
		t.Fatalf("expected refused alternative: %+v", got)
	}
	reasons := got.Explanations[0].Alternatives[0].Reasons
	for _, refusal := range want {
		if !containsCode(reasons, string(refusal)) {
			t.Fatalf("trust refusal %q lost: %+v", refusal, reasons)
		}
	}
}
