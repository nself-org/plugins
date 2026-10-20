//go:build scenarios

package scenarios

// Purpose: Epic acceptance 6, teardown-retry bullets: a machine whose teardown failed for 2 h (the
// provider API down, or the delete refused) is destroyed by the FIRST sweep after the API recovers,
// the retry really happens, the operator is told once, and the ledger charges the machine exactly once.
// Inputs:  burst.Engine over fakecloud (Outage / FailTeardown) and a real store, clock advanced by 2 h.
// Outputs: assertions only.
// Constraints: no sleeps; the sweep cadence is the injected clock stepping every SweepEvery.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func burstCalls(r *burstRig) int { return r.cloud.Calls["List"] + r.cloud.Calls["Teardown"] }

func TestBurstTeardownFailedTwoHoursIsDestroyedByFirstSweepAfterRecovery(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"outage", "teardown-refused"} {
		t.Run(mode, func(t *testing.T) {
			r := burstNew(t)
			res := r.provision("job-1")
			r.join(res.Token, "node-1")
			r.clk.Advance(30 * time.Minute)
			fail := func(on bool) {
				switch {
				case mode == "outage":
					r.cloud.Outage(on)
				case on:
					r.cloud.FailTeardown(burst.ErrUnavailable)
				default:
					r.cloud.FailTeardown(nil)
				}
			}
			fail(true)
			if err := r.eng.Teardown(ctx, res.Token, "job done"); burst.CodeOf(err) != burst.CodeTeardownFailed {
				t.Fatalf("want E697: %v", err)
			}
			if in := r.intent(res.Token); in.State != store.BurstTearingDown {
				t.Fatalf("a failed teardown leaves the intent tearing-down: %+v", in)
			}
			for i := 0; i < 24; i++ { // two hours of five-minute sweeps, all failing
				before := burstCalls(r)
				r.clk.Advance(5 * time.Minute)
				if _, err := r.eng.Tick(ctx); err == nil {
					t.Fatalf("sweep %d reported success during the failure", i)
				}
				if burstCalls(r) <= before {
					t.Fatalf("sweep %d did not retry the teardown", i)
				}
				if row := r.resourceRow(res.Token); row.ConfirmedTeardownAt != nil {
					t.Fatalf("sweep %d released the reservation while the machine was still there", i)
				}
			}
			if len(r.alerts) != 1 || r.alerts[0].Kind != burst.AlertTeardownFailed {
				t.Fatalf("exactly one burst.teardown_failed alert for the whole 2 h: %v", r.alerts)
			}
			fail(false)
			teardowns := r.cloud.Calls["Teardown"]
			if _, err := r.eng.Tick(ctx); err != nil {
				t.Fatalf("the first sweep after recovery: %v", err)
			}
			if r.cloud.Calls["Teardown"] <= teardowns {
				t.Fatal("the retry did not call the provider's teardown")
			}
			if len(r.cloud.All()) != 0 || len(r.labelled(burstCoord)) != 0 {
				t.Fatalf("the machine survived the first sweep after recovery: %+v", r.cloud.All())
			}
			if in := r.intent(res.Token); in.State != store.BurstTerminal {
				t.Fatalf("intent %+v", in)
			}
			if len(r.alerts) != 1 || r.auditCount("burst.teardown_failed") != 1 {
				t.Fatalf("recovery must not alert again: alerts %v audit %d", r.alerts, r.auditCount("burst.teardown_failed"))
			}
		})
	}
}

// TestBurstRetriedTeardownDoesNotDoubleChargeTheLedger: the 2 h of retries bill the machine once, at
// its real cost (150 min: 30 min of life plus the 2 h outage), and no later sweep, tick or teardown moves it.
func TestBurstRetriedTeardownDoesNotDoubleChargeTheLedger(t *testing.T) {
	ctx := context.Background()
	r := burstNew(t)
	res := r.provision("job-1")
	r.join(res.Token, "node-1")
	reserved := r.resourceRow(res.Token).ReservedMicro
	r.clk.Advance(30 * time.Minute)
	r.cloud.Outage(true)
	_ = r.eng.Teardown(ctx, res.Token, "job done")
	for i := 0; i < 24; i++ {
		r.clk.Advance(5 * time.Minute)
		_, _ = r.eng.Tick(ctx)
		if held := r.dayHeld(res.Token); held != reserved {
			t.Fatalf("sweep %d: the day budget holds %d, want the unchanged reservation %d", i, held, reserved)
		}
	}
	r.cloud.Outage(false)
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	const want = int64(150 * 60_000) // 150 minutes at 3.6e6 micro an hour, 60 s increments
	check := func(when string) {
		t.Helper()
		row := r.resourceRow(res.Token)
		if row.ActualMicro == nil || *row.ActualMicro != want {
			t.Fatalf("%s: actual %v, want %d once", when, row.ActualMicro, want)
		}
		if held := r.dayHeld(res.Token); held != want {
			t.Fatalf("%s: the day budget holds %d, want %d", when, held, want)
		}
		month, err := r.st.LedgerMonth(ctx, r.clk.Now().Format("2006-01"))
		if err != nil {
			t.Fatal(err)
		}
		if m := month["fakecloud"]; m.Attempts != 1 || m.Settled != 1 || m.ActualMicro != want || m.PendingMicro != 0 {
			t.Fatalf("%s: month totals %+v, want one settled resource at %d", when, m, want)
		}
		if open, err := r.st.ResourcesOpen(ctx); err != nil || len(open) != 0 {
			t.Fatalf("%s: open resources %+v %v", when, open, err)
		}
	}
	check("after recovery")
	r.clk.Advance(time.Hour)
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.Sweep(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := r.eng.Teardown(ctx, res.Token, "again"); err != nil {
		t.Fatalf("a second teardown of a terminal intent is a no-op: %v", err)
	}
	r.restart()
	if err := r.eng.Start(ctx); err != nil {
		t.Fatal(err)
	}
	check("after a later tick, sweep, teardown and restart")
}
