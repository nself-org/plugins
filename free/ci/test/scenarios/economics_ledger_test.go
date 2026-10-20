//go:build scenarios

package scenarios

// Purpose: Epic acceptance 5, the ledger report. Every figure in `ci policy usage` says whether it is
// estimated (a worst-case bound or a declared ceiling), computed (from the coordinator's own durations),
// known (what the provider reported) or unknown (no basis), and a charge is whole billing increments:
// a 50 s job on a one-minute increment is billed 60 s.
// Inputs:  admissions through dispatch.Admit, the real economics finalizer registered in internal/run,
//          economics.RecordUsage and economics.BuildReport.
// Outputs: assertions only.
// Constraints: injected clock; no figure may lack a confidence or a source.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// econUnlabelled lists every JSON object with a "micro" figure that lacks a confidence or a source.
func econUnlabelled(v any, path string, bad *[]string) {
	switch x := v.(type) {
	case map[string]any:
		if _, has := x["micro"]; has {
			if c, _ := x["confidence"].(string); c == "" {
				*bad = append(*bad, path+": no confidence")
			}
			if s, _ := x["source"].(string); s == "" {
				*bad = append(*bad, path+": no source")
			}
		}
		for k, e := range x {
			econUnlabelled(e, path+"."+k, bad)
		}
	case []any:
		for _, e := range x {
			econUnlabelled(e, path+"[]", bad)
		}
	}
}

// econFinalize runs the economics finalizer the run package registers.
func econFinalize(t *testing.T, env economics.Env, attempt string, d time.Duration) {
	t.Helper()
	info := cirun.RunInfo{RunID: "pipe", Jobs: []cirun.JobOutcome{{AttemptID: attempt, Duration: d}}}
	for _, f := range cirun.Registered.Finalizers {
		if f.Name() == "economics" {
			if err := f.Finalize(economics.WithEnv(ctxT(t), env), info); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("the economics finalizer is not registered")
}

func TestEconomicsLedgerLabelsEstimatedComputedKnownAndUnknown(t *testing.T) {
	clk := newClock()
	st := econStore(t, clk)
	kv := econKV(econIncludedKV())
	kv["economics.rates.other-hosted"] = `{"class":"included","rate_per_minute_micro":1}`
	kv["economics.ceilings.other-hosted"] = `5`
	kv["economics.rates.bal-hosted"] = `{"class":"included","rate_per_minute_micro":1}`
	p := econPolicy(t, trust.SourceProject, kv, nil)
	env := econEnv(st, p, clk)
	ctx := economics.WithEnv(ctxT(t), env)

	// billing increments: whole started increments of the declared rate
	snap, ok := economics.SnapOf(p.Rates["cloudco"])
	if !ok || economics.CostFor(p, "", "cloudco").BillingIncrementMs != 60_000 {
		t.Fatalf("rate snapshot %+v ok=%v", snap, ok)
	}
	for ms, want := range map[int64]int64{1: 60_000, 50_000: 60_000, 60_000: 60_000, 60_001: 120_000, 70_000: 120_000} {
		if micro, billed, over := economics.Charge(snap, ms); over || micro != want || billed != want {
			t.Fatalf("Charge(%d ms) = %d micro, %d ms billed (overflow %v), want %d", ms, micro, billed, over, want)
		}
	}

	a := queueAttempt(t, st, "pipe", "project", "build")
	b := queueAttempt(t, st, "pipe", "project", "lint")
	for _, id := range []string{a, b} {
		if err := econAdmit(env, st, econRequest(id, "", "cloudco", 70_000)); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := economics.BuildReport(ctx, st, p, clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Recent) != 2 {
		t.Fatalf("the report must list both admitted attempts before they settle, got %d", len(rep.Recent))
	}
	seen := map[string]bool{}
	for _, line := range rep.Recent {
		seen[line.Attempt] = true
		if line.Estimate.Micro != 120_000 || line.Estimate.Confidence != economics.ConfEstimated || line.Actual != nil {
			t.Fatalf("before settling, a paid attempt is an estimate of the 70 s bound (120 000 micro): %+v", line)
		}
	}
	if !seen[a] || !seen[b] {
		t.Fatalf("the pending report names %v, want both %s and %s", seen, a, b)
	}
	econFinalize(t, env, a, 50*time.Second) // observed: billed as one started minute, computed
	econFinalize(t, env, b, 0)              // no duration anywhere: settles at the estimate, unknown
	if err = economics.RecordUsage(ctx, economics.Usage{Key: "k1", Provider: "github-hosted", Date: "2026-10-17", Minutes: 10, CostMicro: 1000}); err != nil {
		t.Fatal(err)
	}
	bal := int64(777)
	if err = economics.RecordUsage(ctx, economics.Usage{Key: "k2", Provider: "bal-hosted", Date: "2026-10-17", Minutes: 1, CostMicro: 1, BalanceMicro: &bal}); err != nil {
		t.Fatal(err)
	}
	if rep, err = economics.BuildReport(ctx, st, p, clk.Now()); err != nil {
		t.Fatal(err)
	}
	if len(rep.Recent) != 2 {
		t.Fatalf("the settled report must list both attempts, got %d", len(rep.Recent))
	}
	actual := map[string]economics.Figure{}
	for _, line := range rep.Recent {
		if line.Actual == nil {
			t.Fatalf("an unsettled attempt after finalize: %+v", line)
		}
		actual[line.Attempt] = *line.Actual
	}
	if f := actual[a]; f.Micro != 60_000 || f.Confidence != economics.ConfComputed {
		t.Fatalf("50 s on a one-minute increment: %+v, want 60000 computed", f)
	}
	if f := actual[b]; f.Micro != 120_000 || f.Confidence != economics.ConfUnknown {
		t.Fatalf("no recorded duration: %+v, want the estimate with confidence unknown", f)
	}
	prov := map[string]economics.ProviderLine{}
	for _, pl := range rep.Providers {
		prov[pl.Provider] = pl
	}
	if gh := prov["github-hosted"]; gh.Imported == nil || gh.Imported.Confidence != economics.ConfKnown || gh.Allowance == nil || gh.Allowance.Confidence != economics.ConfEstimated || !gh.AllowanceKnown {
		t.Fatalf("github-hosted: imported must be known and the allowance estimated: %+v", gh)
	}
	if oh := prov["other-hosted"]; oh.AllowanceKnown || oh.Allowance == nil || oh.Allowance.Confidence != economics.ConfEstimated {
		t.Fatalf("a ceiling without imported consumption is estimated and not known: %+v", oh)
	}
	if bh := prov["bal-hosted"]; bh.Allowance == nil || bh.Allowance.Confidence != economics.ConfKnown || bh.Allowance.Micro != 777 {
		t.Fatalf("a provider-reported balance is known: %+v", bh)
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var bad []string
	econUnlabelled(doc, "report", &bad)
	if len(bad) > 0 {
		t.Fatalf("figures without a label: %v\n%s", bad, raw)
	}
}
