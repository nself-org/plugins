package main

// Tests of `nself ci policy usage` (P7-SCHED-07): the ledger report, with a
// confidence on every money figure, from an empty machine and from a store.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type usageEnvelope struct {
	Data economics.Report `json:"data"`
}

// unlabelled lists every JSON object holding a "micro" figure without a confidence and a source.
func unlabelled(v any, path string, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		if _, has := x["micro"]; has {
			if c, _ := x["confidence"].(string); c == "" {
				*out = append(*out, path)
			}
			if s, _ := x["source"].(string); s == "" {
				*out = append(*out, path)
			}
		}
		for k, e := range x {
			unlabelled(e, path+"."+k, out)
		}
	case []any:
		for _, e := range x {
			unlabelled(e, path+"[]", out)
		}
	}
}

func TestPolicyUsageEmptyMachineShowsTheDeclaredPolicy(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	code, out, errs := capture(t, func() int { return runPolicyUsage([]string{"--json"}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	var env usageEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	if env.Data.PaidEnabled || env.Data.Currency != "USD" || len(env.Data.Providers) != 0 || env.Data.Day == "" || len(env.Data.Month) != 7 {
		t.Fatalf("report: %+v", env.Data)
	}
	code, out, _ = capture(t, func() int { return runPolicyUsage(nil) })
	if code != 0 || !strings.Contains(out, "paid spend disabled") {
		t.Fatalf("text: %d %q", code, out)
	}
}

func TestPolicyUsageNeverPrintsAFigureWithoutItsConfidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	ctx := context.Background()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
		day, err := tx.Window(economics.EmptyReport(economics.Policy{}, time.Now()).Day)
		if err != nil {
			return err
		}
		for _, r := range []store.LedgerRow{
			{Attempt: "a-settled", Pipeline: "p", Project: "x", Provider: "cloudco", Class: "metered", EstMicro: 120_000, Confidence: "estimated", WindowDay: day},
			{Attempt: "a-pending", Pipeline: "p", Project: "x", Provider: "cloudco", Class: "metered", EstMicro: 60_000, Confidence: "estimated", WindowDay: day},
			{Attempt: "a-local", Pipeline: "p", Project: "x", Provider: "operator", Class: "owned", Confidence: "computed", WindowDay: day},
		} {
			if err = tx.PutLedger(r); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.LedgerSettle(ctx, "a-settled", 60_000, 60_000, "computed"); err != nil {
		t.Fatal(err)
	}
	if err = st.PutUsage(ctx, store.UsageRow{Key: "k", Provider: "github-hosted", Period: economics.EmptyReport(economics.Policy{}, time.Now()).Month, Minutes: 3, CostMicro: 300, ReportedAt: 1}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	code, out, errs := capture(t, func() int { return runPolicyUsage([]string{"--json"}) })
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	var doc any
	if err = json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var bad []string
	unlabelled(doc, "usage", &bad)
	if len(bad) != 0 {
		t.Fatalf("figures without a confidence or source: %v\n%s", bad, out)
	}
	var env usageEnvelope
	_ = json.Unmarshal([]byte(out), &env)
	byProvider := map[string]economics.ProviderLine{}
	for _, p := range env.Data.Providers {
		byProvider[p.Provider] = p
	}
	if c := byProvider["cloudco"]; c.Local.Micro != 120_000 || c.Local.Confidence != "estimated" { // 60 000 settled + 60 000 still an estimate
		t.Fatalf("cloudco: %+v", c)
	}
	if o := byProvider["operator"]; o.Local.Micro != 0 || o.Local.Confidence != "computed" {
		t.Fatalf("operator: %+v", o)
	}
	if g := byProvider["github-hosted"]; g.Class != "included" || g.Imported == nil || g.Imported.Confidence != "known" || g.Allowance == nil || g.Allowance.Confidence != "unknown" || g.AllowanceKnown {
		t.Fatalf("github-hosted without a declared ceiling must read unknown: %+v allowance=%+v", g, g.Allowance)
	}
	code, text, _ := capture(t, func() int { return runPolicyUsage(nil) })
	if code != 0 || !strings.Contains(text, "(known: provider-reported usage)") || !strings.Contains(text, "(computed:") || !strings.Contains(text, "allowance 0 micro (unknown:") {
		t.Fatalf("text:\n%s", text)
	}
}

func TestPolicyUsageRejectsStrayArguments(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	if code, _, _ := capture(t, func() int { return runPolicyUsage([]string{"extra"}) }); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if code, _, _ := capture(t, func() int { return runPolicyUsage([]string{"--no-such-flag"}) }); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if _, ok := v15Subcommands["policy usage"]; !ok {
		t.Fatal("`policy usage` is not registered")
	}
}
