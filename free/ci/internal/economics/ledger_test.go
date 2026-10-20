package economics

// Tests of the ledger (Epic D11): confidence of every figure, settling at
// finalize, clamping a remote attempt that reports end before start, provider
// rows as the only `known` figures, and the report.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func finalize(t *testing.T, ctx context.Context, attempt string, d time.Duration) {
	t.Helper()
	info := cirun.RunInfo{RunID: "pipe", Jobs: []cirun.JobOutcome{{AttemptID: attempt, Duration: d}}}
	if err := (finalizer{}).Finalize(ctx, info); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerLocalActualIsComputedAndSettlesTheReservation(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "", "cloudco", 300_000, 0)); err != nil { // 5 min = 300 000
		t.Fatal(err)
	}
	finalize(t, c, a, 70*time.Second) // 2 started increments = 120 000
	row, _, _ := st.LedgerAttempt(ctx, a)
	if row.ActMicro == nil || *row.ActMicro != 120_000 || *row.ActMs != 70_000 || row.Confidence != ConfComputed || row.FinalizedAt == 0 {
		t.Fatalf("settled row: %+v act=%v", row, row.ActMicro)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 120_000 {
		t.Fatalf("the reservation takes the actual: %d", held)
	}
	finalize(t, c, a, 5*time.Minute) // a second settle changes nothing
	if again, _, _ := st.LedgerAttempt(ctx, a); *again.ActMicro != 120_000 {
		t.Fatalf("settle is not idempotent: %d", *again.ActMicro)
	}
}

func TestLedgerActualNeverExceedsTheEstimate(t *testing.T) {
	row := store.LedgerRow{Class: ClassMetered, EstMs: 120_000, EstMicro: 120_000, Rate: store.RateSnap{RateMicro: 60_000, RateUnitMs: msPerMinute, IncMs: msPerMinute}}
	ms, micro, conf := Actual(row, 10*time.Minute, 0, 0)
	if ms != 120_000 || micro != 120_000 || conf != ConfEstimated {
		t.Fatalf("an overrun is clamped to the bound and flagged: %d %d %s", ms, micro, conf)
	}
}

func TestLedgerRemoteEndBeforeStartIsClampedToTheEstimate(t *testing.T) {
	row := store.LedgerRow{Class: ClassMetered, Remote: true, EstMs: 120_000, EstMicro: 120_000, Rate: store.RateSnap{RateMicro: 60_000, RateUnitMs: msPerMinute, IncMs: msPerMinute}}
	start := time.Date(2026, 10, 19, 10, 5, 0, 0, time.UTC).UnixNano()
	ms, micro, conf := Actual(row, 0, start, start-int64(time.Hour))
	if ms != 120_000 || micro != 120_000 || conf != ConfEstimated {
		t.Fatalf("end < start: %d %d %s", ms, micro, conf)
	}
	ms, micro, conf = Actual(row, 0, start, start+int64(30*time.Second))
	if ms != 30_000 || micro != 60_000 || conf != ConfComputed {
		t.Fatalf("a sane remote span is computed from the coordinator's stamps: %d %d %s", ms, micro, conf)
	}
}

func TestLedgerUnknownDurationSettlesAtTheEstimate(t *testing.T) {
	row := store.LedgerRow{Class: ClassMetered, EstMs: 60_000, EstMicro: 60_000}
	if ms, micro, conf := Actual(row, 0, 0, 0); ms != 60_000 || micro != 60_000 || conf != ConfUnknown {
		t.Fatalf("no basis: %d %d %s", ms, micro, conf)
	}
	local := store.LedgerRow{Class: ClassOwned}
	if ms, micro, conf := Actual(local, 90*time.Second, 0, 0); ms != 90_000 || micro != 0 || conf != ConfComputed {
		t.Fatalf("local actual: %d %d %s", ms, micro, conf)
	}
}

// walk reports every JSON object that holds a "micro" figure but no confidence or source.
func walk(v any, path string, bad *[]string) {
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
			walk(e, path+"."+k, bad)
		}
	case []any:
		for i, e := range x {
			walk(e, path+"[]"+string(rune('0'+i%10)), bad)
		}
	}
}

func TestLedgerReportLabelsEveryFigure(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{
		"economics.rates.github-hosted": `{"class":"included","rate_per_minute_micro":100}`, "economics.ceilings.github-hosted": `1000000`,
		"economics.ceilings.other-hosted": `5`, "economics.rates.other-hosted": `{"class":"included","rate_per_minute_micro":1}`,
	}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "", "cloudco", 120_000, 0)); err != nil {
		t.Fatal(err)
	}
	finalize(t, c, a, time.Minute)
	b := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(b, "", "cloudco", 120_000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := RecordUsage(c, Usage{Key: "k1", Provider: "github-hosted", Date: "2026-10-18", Minutes: 10, CostMicro: 1000}); err != nil {
		t.Fatal(err)
	}
	rep, err := BuildReport(ctx, st, p, clk.now())
	if err != nil {
		t.Fatal(err)
	}
	var byProvider = map[string]ProviderLine{}
	for _, pl := range rep.Providers {
		byProvider[pl.Provider] = pl
	}
	cl := byProvider["cloudco"]
	if cl.Local.Micro != 60_000+120_000 || cl.Local.Confidence != ConfEstimated { // one settled (computed) plus one pending estimate
		t.Fatalf("cloudco local: %+v", cl.Local)
	}
	gh := byProvider["github-hosted"]
	if gh.Imported == nil || gh.Imported.Confidence != ConfKnown || gh.Allowance == nil || gh.Allowance.Confidence != ConfEstimated || !gh.AllowanceKnown || gh.Allowance.Micro != 999_000 {
		t.Fatalf("github-hosted: %+v imported=%+v allowance=%+v", gh, gh.Imported, gh.Allowance)
	}
	oh := byProvider["other-hosted"]
	if oh.Allowance == nil || oh.AllowanceKnown || oh.Allowance.Confidence != ConfEstimated {
		t.Fatalf("a declared ceiling without imported consumption is estimated but not known: %+v", oh)
	}
	var settled *AttemptLine
	for i := range rep.Recent {
		if rep.Recent[i].Attempt == a {
			settled = &rep.Recent[i]
		}
	}
	if settled == nil || settled.Actual == nil || settled.Actual.Confidence != ConfComputed {
		t.Fatalf("recent: %+v", rep.Recent)
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	_ = json.Unmarshal(raw, &doc)
	var bad []string
	walk(doc, "report", &bad)
	if len(bad) > 0 {
		t.Fatalf("figures without a label: %v\n%s", bad, raw)
	}
	empty := EmptyReport(p, clk.now())
	raw, _ = json.Marshal(empty)
	_ = json.Unmarshal(raw, &doc)
	walk(doc, "empty", &bad)
	if len(bad) > 0 {
		t.Fatalf("empty report: %v", bad)
	}
}

func TestLedgerUnknownAllowanceWithoutACeiling(t *testing.T) {
	a := AllowanceOf(spendPolicy(t, paidKV(nil)), "github-hosted", store.AllowanceInputs{})
	if a.Known || a.CeilingKnown || a.Confidence != ConfUnknown {
		t.Fatalf("%+v", a)
	}
	bal := int64(500)
	a = AllowanceOf(spendPolicy(t, paidKV(nil)), "github-hosted", store.AllowanceInputs{Balance: &bal, Local: 100})
	if !a.Known || a.Remaining != 400 || a.Confidence != ConfKnown {
		t.Fatalf("a reported balance is known: %+v", a)
	}
}

func TestLedgerRecordUsageIsIdempotentAndValidated(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	for i := 0; i < 3; i++ {
		if err := RecordUsage(c, Usage{Key: "same", Provider: "github-hosted", Date: "2026-10-18", Minutes: 5, CostMicro: 500}); err != nil {
			t.Fatal(err)
		}
	}
	in, err := st.Allowance(ctx, "github-hosted", "2026-10")
	if err != nil || !in.HasImport || in.Imported != 500 {
		t.Fatalf("one row, not three: %+v %v", in, err)
	}
	for _, bad := range []Usage{{Provider: "p", Date: "2026-10-18"}, {Key: "k", Date: "2026-10-18"}, {Key: "k", Provider: "p", Date: "10/18"}, {Key: "k", Provider: "p", Date: "2026-10-18", CostMicro: -1}} {
		if RecordUsage(c, bad) == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if RecordUsage(ctx, Usage{Key: "k", Provider: "p", Date: "2026-10-18"}) == nil {
		t.Error("recorded usage with no env bound")
	}
}
