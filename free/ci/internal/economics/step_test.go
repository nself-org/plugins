package economics

// Tests of the `budget` admission step (Epic D11): the paid rule, budgets held
// in the admission transaction, clock-safe windows, the owner coordinator and
// the included allowance rule. The step runs through dispatch.Admit, the real chokepoint.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

func TestBudgetPaidNeedsEnableAndABudget(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	for _, tc := range []struct {
		name string
		kv   map[string]string
		want model.PlacementReason
	}{
		{"paid disabled", map[string]string{schedpolicy.KeyPaidEnabled: `false`}, model.PaidDisabled},
		{"only a per-job budget", map[string]string{schedpolicy.KeyBudgetDay: `null`}, model.BudgetUnset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := openStore(t, clk)
			kv := paidKV(nil)
			delete(kv, schedpolicy.KeyBudgetDay)
			delete(kv, schedpolicy.KeyBudgetMonth)
			if tc.want == model.BudgetUnset {
				kv[schedpolicy.KeyBudgetJob] = `1000000`
			} else {
				kv[schedpolicy.KeyPaidEnabled] = `false`
			}
			p := spendPolicy(t, kv)
			c := WithEnv(ctx, envFor(st, p, clk, "me"))
			att := queueAttempt(t, st, "pipe", "rev", "project")
			ref := refusalOf(t, place(c, st, request(att, "", "cloudco", 60_000, 60_000)))
			if ref.Reason != tc.want {
				t.Fatalf("reason %s (%s), want %s", ref.Reason, ref.Detail, tc.want)
			}
			if _, ok, _ := st.LedgerAttempt(ctx, att); ok {
				t.Fatal("a refused admission left a ledger row")
			}
		})
	}
}

func TestBudgetReservesTheWorstCaseAndRefusesAtTheLimit(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyBudgetDay: `150000`}))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	a1, a2 := queueAttempt(t, st, "pipe", "rev", "project"), queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a1, "", "cloudco", 120_000, 0)); err != nil { // 2 minutes = 120 000 micro
		t.Fatal(err)
	}
	row, ok, err := st.LedgerAttempt(ctx, a1)
	if err != nil || !ok || row.EstMicro != 120_000 || row.Confidence != ConfEstimated || row.Class != ClassMetered || row.WindowDay != "2026-10-19" {
		t.Fatalf("ledger row: %+v %v %v", row, ok, err)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 120_000 {
		t.Fatalf("day held %d", held)
	}
	ref := refusalOf(t, place(c, st, request(a2, "", "cloudco", 60_000, 0)))
	if ref.Reason != model.BudgetExceeded {
		t.Fatalf("second job: %+v", ref)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 120_000 {
		t.Fatalf("a refusal changed the held amount: %d", held)
	}
	state, _, _ := st.AttemptStateEpoch(ctx, a2)
	if state != "queued" {
		t.Fatalf("a refused attempt must stay queued, is %s", state)
	}
}

func TestBudgetPlacementCostAboveTheBoundIsHeld(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(nil))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "", "cloudco", 60_000, 90_000)); err != nil {
		t.Fatal(err)
	}
	if row, _, _ := st.LedgerAttempt(ctx, a); row.EstMicro != 90_000 {
		t.Fatalf("the larger of the bound and the scheduler's cost is held: %+v", row)
	}
}

func TestBudgetUnboundedAndUnpricedAreRefused(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(nil))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	for _, tc := range []struct {
		provider string
		timeout  int64
		want     model.PlacementReason
	}{
		{"cloudco", schedpolicy.DefaultMaxPaidTimeoutMs + 1, model.CostUnbounded},
		{"cloudco", 0, model.CostUnbounded},
		{"unrated", 60_000, model.CostUnknownRate},
	} {
		a := queueAttempt(t, st, "pipe", "rev", "project")
		r := request(a, ClassMetered, tc.provider, tc.timeout, 0)
		if ref := refusalOf(t, place(c, st, r)); ref.Reason != tc.want {
			t.Errorf("%s/%d: %s, want %s", tc.provider, tc.timeout, ref.Reason, tc.want)
		}
	}
}

func TestBudgetEveryScopeIsChecked(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	for _, tc := range []struct {
		name, key, limit string
	}{
		{"job", schedpolicy.KeyBudgetJob, `59999`}, {"pipeline", schedpolicy.KeyBudgetPipeline, `59999`},
		{"day", schedpolicy.KeyBudgetDay, `59999`}, {"month", schedpolicy.KeyBudgetMonth, `59999`},
		{"provider", "economics.budgets.provider.cloudco", `59999`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := openStore(t, clk)
			p := spendPolicy(t, paidKV(map[string]string{tc.key: tc.limit}))
			c := WithEnv(ctx, envFor(st, p, clk, "me"))
			a := queueAttempt(t, st, "pipe", "rev", "project")
			if ref := refusalOf(t, place(c, st, request(a, "", "cloudco", 60_000, 0))); ref.Reason != model.BudgetExceeded {
				t.Fatalf("a 60 000 micro job against a %s budget of 59 999: %+v", tc.name, ref)
			}
		})
	}
}

// TestBudgetWindowSurvivesAClockStepBack: the key is the later of the clock and
// the highest key recorded; a settle charges the window of its admission.
func TestBudgetWindowSurvivesAClockStepBack(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-31T23:59:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(nil))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	a1 := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a1, "", "cloudco", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	clk.set(at("2026-11-02T08:00:00Z"))
	a2 := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a2, "", "cloudco", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	clk.set(at("2026-10-31T23:58:00Z")) // the clock steps back across the month boundary
	a3 := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a3, "", "cloudco", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	r3, _, _ := st.LedgerAttempt(ctx, a3)
	if r3.WindowDay != "2026-11-02" {
		t.Fatalf("a stepped-back clock must keep the later window, got %s", r3.WindowDay)
	}
	if oct, _ := st.Reserved(ctx, "month", "", "2026-10"); oct != 60_000 {
		t.Fatalf("october holds %d, want only the first attempt", oct)
	}
	if nov, _ := st.Reserved(ctx, "month", "", "2026-11"); nov != 120_000 {
		t.Fatalf("november holds %d", nov)
	}
	// Finalize a1 (admitted in October) now, with the clock in November's past: it settles in October.
	if _, err := st.LedgerSettle(ctx, a1, 30_000, 30_000, ConfComputed); err != nil {
		t.Fatal(err)
	}
	if oct, _ := st.Reserved(ctx, "month", "", "2026-10"); oct != 30_000 {
		t.Fatalf("settle must charge the admission window: october %d", oct)
	}
	if nov, _ := st.Reserved(ctx, "month", "", "2026-11"); nov != 120_000 {
		t.Fatalf("settle leaked into november: %d", nov)
	}
}

func TestBudgetProviderOwnedElsewhere(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	declared := spendPolicy(t, paidKV(map[string]string{"economics.providers.cloudco": `{"owner_coordinator":"coord-b"}`}))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	ref := refusalOf(t, place(WithEnv(ctx, envFor(st, declared, clk, "coord-a")), st, request(a, "", "cloudco", 60_000, 0)))
	if ref.Reason != model.ProviderOwnedElsewhere {
		t.Fatalf("declared owner: %+v", ref)
	}
	if err := place(WithEnv(ctx, envFor(st, declared, clk, "coord-b")), st, request(a, "", "cloudco", 60_000, 0)); err != nil {
		t.Fatalf("the owner coordinator: %v", err)
	}
	// With no declared owner, the first coordinator to place work owns the account.
	undeclared := spendPolicy(t, paidKV(nil))
	b := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(WithEnv(ctx, envFor(st, undeclared, clk, "coord-a")), st, request(b, "", "cloudco", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	d := queueAttempt(t, st, "pipe", "rev", "project")
	ref = refusalOf(t, place(WithEnv(ctx, envFor(st, undeclared, clk, "coord-z")), st, request(d, "", "cloudco", 60_000, 0)))
	if ref.Reason != model.ProviderOwnedElsewhere {
		t.Fatalf("recorded owner: %+v", ref)
	}
}

func TestBudgetGitHubIsNeverPaid(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(nil))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	a := queueAttempt(t, st, "pipe", "rev", "project")
	// Whatever the runner says it is, GitHub-hosted capacity is included: with no declared
	// ceiling it is ineligible (allowance.unknown), and it never reserves money.
	ref := refusalOf(t, place(c, st, request(a, ClassMetered, "github-hosted", 60_000, 60_000)))
	if ref.Reason != model.AllowanceUnknown {
		t.Fatalf("%+v", ref)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-19"); held != 0 {
		t.Fatalf("GitHub reserved money: %d", held)
	}
}

func TestBudgetIncludedNeedsAKnownAllowanceAboveTheMargin(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, map[string]string{
		"economics.rates.github-hosted":    `{"class":"included","rate_per_minute_micro":100,"billing_increment":"1m"}`,
		"economics.ceilings.github-hosted": `1000000`, schedpolicy.KeyMargin: `50000`,
	})
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	a1 := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(a1, "", "github-hosted", 60_000, 0))); ref.Reason != model.AllowanceUnknown {
		t.Fatalf("no imported consumption: %+v", ref)
	}
	if err := RecordUsage(c, Usage{Key: "acct/2026-10-18/linux/r", Provider: "github-hosted", Date: "2026-10-18", Minutes: 100, CostMicro: 940_000}); err != nil {
		t.Fatal(err)
	}
	// 1 000 000 - 940 000 = 60 000 left; margin 50 000; the job's worst case is 100 micro: it fits.
	if err := place(c, st, request(a1, "", "github-hosted", 60_000, 0)); err != nil {
		t.Fatalf("above the margin: %v", err)
	}
	if row, _, _ := st.LedgerAttempt(ctx, a1); row.Class != ClassIncluded || row.EstMicro != 100 {
		t.Fatalf("ledger: %+v", row)
	}
	if err := RecordUsage(c, Usage{Key: "acct/2026-10-18/linux/s", Provider: "github-hosted", Date: "2026-10-18", Minutes: 100, CostMicro: 9_950}); err != nil {
		t.Fatal(err)
	}
	// 60 000 - 9 950 - 100 (local) = 49 950 left, below the 50 000 margin.
	a2 := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(c, st, request(a2, "", "github-hosted", 60_000, 0))); ref.Reason != model.AllowanceBelowMargin {
		t.Fatalf("below the margin: %+v", ref)
	}
}

func TestBudgetLocalWorkCostsNothingAndNeedsNoPolicy(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	env := Env{Store: st, Now: clk.now, Coordinator: "me"} // no spend policy bound at all
	c := WithEnv(ctx, env)
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, ClassOwned, "operator", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	row, ok, _ := st.LedgerAttempt(ctx, a)
	if !ok || row.EstMicro != 0 || row.Class != ClassOwned || row.Confidence != ConfComputed {
		t.Fatalf("local ledger row: %+v", row)
	}
	// No env at all: the step is inert and writes nothing.
	b := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(ctx, st, request(b, ClassOwned, "operator", 60_000, 0)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.LedgerAttempt(ctx, b); ok {
		t.Fatal("an unbound process wrote a ledger row")
	}
}

// TestBudgetPlantedSpendKeyIsIgnored: a spend key set from a revision never
// turns paid spend on; economics sees only what schedpolicy.Spend returns.
func TestBudgetPlantedSpendKeyIsIgnored(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	kv := paidKV(nil)
	kv[schedpolicy.KeyPaidEnabled] = `false`
	p := spendPolicy(t, kv, map[string]string{schedpolicy.KeyPaidEnabled: `true`, schedpolicy.KeyBudgetDay: `999999999`})
	if p.PaidEnabled || p.Budgets.Day.Micro != 1_000_000 {
		t.Fatalf("a planted revision changed the spend policy: %+v", p)
	}
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if ref := refusalOf(t, place(WithEnv(ctx, envFor(st, p, clk, "me")), st, request(a, "", "cloudco", 60_000, 0))); ref.Reason != model.PaidDisabled {
		t.Fatalf("%+v", ref)
	}
}
