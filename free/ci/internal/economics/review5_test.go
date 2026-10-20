package economics

// Tests added for the cross-family review of P7-SCHED-07: usage streams that lag
// by different amounts, burst VMs in the monthly report, provider ids spelled
// oddly, and the policy-outage fallback that must stay on this machine.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestLedgerAllowanceCountsFromTheMostLaggingUsageStream(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	put := func(attempt, day string, micro int64) {
		_ = st.LedgerWrite(ctx, func(tx store.LedgerTx) error {
			return tx.PutLedger(store.LedgerRow{Attempt: attempt, Pipeline: "p", Provider: "gh", Class: ClassIncluded, EstMicro: micro, Confidence: ConfEstimated, WindowDay: day})
		})
	}
	put("d15", "2026-10-15", 1)
	put("d16", "2026-10-16", 10)
	put("d17", "2026-10-17", 100)
	put("d18", "2026-10-18", 1000)
	reported := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC).UnixNano()
	// storage is reported through 10-18, compute only through 10-15: the days between are in neither the import nor, if the cut-off took the newest day, the local count.
	for _, u := range []store.UsageRow{{Key: "storage/18", Provider: "gh", Period: "2026-10", Day: "2026-10-18", Series: "storage", ReportedAt: reported}, {Key: "compute/15", Provider: "gh", Period: "2026-10", Day: "2026-10-15", Series: "compute", ReportedAt: reported}} {
		if err := st.PutUsage(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	in, err := st.Allowance(ctx, "gh", "2026-10")
	if err != nil || in.Local != 1+10+100+1000 {
		t.Fatalf("local spend counted from the lagging stream's newest day (10-15): %d %v", in.Local, err)
	}
}

func TestLedgerMonthlyReportIncludesBurstVMs(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, ephemeralKV(nil))
	c := WithEnv(ctx, envFor(st, p, clk, "me"))
	if err := reserveVMFor(c, st, "vm1", "pipe", 10*time.Minute); err != nil { // 600 000, open
		t.Fatal(err)
	}
	if err := reserveVMFor(c, st, "vm2", "pipe", 5*time.Minute); err != nil { // 300 000, torn down after 2 minutes
		t.Fatal(err)
	}
	clk.set(clk.now().Add(2 * time.Minute))
	if err := releaseVM(c, st, "vm2", clk.now()); err != nil {
		t.Fatal(err)
	}
	rep, err := BuildReport(ctx, st, p, clk.now())
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range rep.Providers {
		if pl.Provider == "cloudco" {
			if pl.Local.Micro != 600_000+120_000 || pl.Local.Confidence != ConfEstimated {
				t.Fatalf("cloudco local: %+v (an open VM at its reservation, a torn-down one at its actual)", pl.Local)
			}
			return
		}
	}
	t.Fatalf("no cloudco line in the report: %+v", rep.Providers)
}

func TestBudgetOddlySpelledProviderStillMeetsTheRateTable(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	c := WithEnv(ctx, envFor(st, spendPolicy(t, paidKV(nil)), clk, "me"))
	for _, spelled := range []string{"Cloudco", "cloudco ", " CLOUDCO\t"} {
		a := queueAttempt(t, st, "pipe", "rev", "project")
		if err := place(c, st, request(a, ClassOwned, spelled, 60_000, 0)); err != nil {
			t.Fatalf("%q: %v", spelled, err)
		}
		row, _, _ := st.LedgerAttempt(ctx, a)
		if row.Class != ClassMetered || row.EstMicro != 60_000 || row.Provider != "cloudco" {
			t.Fatalf("%q is the rated provider, not a free one: %+v", spelled, row)
		}
	}
	if c := CostFor(spendPolicy(t, paidKV(nil)), ClassOwned, " Cloudco "); c.Class != ClassMetered {
		t.Fatalf("the world prices it the same way: %+v", c)
	}
}

func TestEconomicsClassOfNormalisesTheProviderItself(t *testing.T) {
	p := spendPolicy(t, paidKV(nil))
	for _, spelled := range []string{"Cloudco", " cloudco", "CLOUDCO\t"} {
		if got := classOf(p, ClassOwned, spelled); got != ClassMetered {
			t.Errorf("classOf(%q) = %s, want metered", spelled, got)
		}
	}
}

func TestBudgetPolicyOutageFallbackStaysOnThisMachine(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	env := Env{Store: st, Now: clk.now, Coordinator: "me", Spend: func(context.Context) (Policy, error) { return Policy{}, errors.New("policy store unreadable") }}
	c := WithEnv(ctx, env)
	// A runner on this machine claiming `owned` but with a paid provider's id is not this machine.
	a := queueAttempt(t, st, "pipe", "rev", "project")
	r := request(a, ClassOwned, "cloudco", 60_000, 0)
	if err := place(c, st, r); err == nil || errors.As(err, new(*SpendError)) {
		t.Fatalf("a claimed-free paid provider during a policy outage must fail closed: %v", err)
	}
	for _, provider := range []string{"local", "operator", ""} {
		b := queueAttempt(t, st, "pipe", "rev", "project")
		if err := place(c, st, request(b, ClassLocal, provider, 60_000, 0)); err != nil {
			t.Fatalf("this machine (%q) must still run: %v", provider, err)
		}
	}
	_ = model.PaidDisabled
}
