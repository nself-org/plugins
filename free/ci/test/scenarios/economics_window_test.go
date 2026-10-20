//go:build scenarios

package scenarios

// Purpose: Epic acceptance 5, window discipline. A clock stepped back across a month boundary never
// charges an earlier window: the store keeps a high-water mark of the window key, so work admitted
// after the step back is still charged to the later window and the earlier window's total is unchanged.
// Inputs:  the store clock (store.Options.Clock) and the Env clock moved with clock.Advance.
// Outputs: assertions only.
// Constraints: both clocks are the one injected clock; no wall clock is read.

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestEconomicsClockStepBackNeverChargesAnEarlierWindow(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC)}
	st := econStore(t, clk)
	env := econEnv(st, econPolicy(t, trust.SourceProject, econKV(nil), nil), clk)
	ctx := ctxT(t)
	admit := func() string {
		t.Helper()
		a := queueAttempt(t, st, "pipe", "project", "build")
		if err := econAdmit(env, st, econRequest(a, "", "cloudco", 60_000)); err != nil {
			t.Fatal(err)
		}
		row, _, err := st.LedgerAttempt(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		return row.WindowDay
	}
	if day := admit(); day != "2026-10-31" {
		t.Fatalf("first charge window %s", day)
	}
	clk.Advance(32 * time.Hour) // 2026-11-02 07:59
	if day := admit(); day != "2026-11-02" {
		t.Fatalf("second charge window %s", day)
	}
	octDay, _ := st.Reserved(ctx, "day", "", "2026-10-31")
	octMonth, _ := st.Reserved(ctx, "month", "", "2026-10")
	clk.Advance(-33 * time.Hour) // back across the month boundary: 2026-10-31 22:59
	if day := admit(); day != "2026-11-02" {
		t.Fatalf("after a step back the charge went to the earlier window %s", day)
	}
	if d, _ := st.Reserved(ctx, "day", "", "2026-10-31"); d != octDay {
		t.Fatalf("the earlier day window changed: %d -> %d", octDay, d)
	}
	if m, _ := st.Reserved(ctx, "month", "", "2026-10"); m != octMonth || m != 60_000 {
		t.Fatalf("the earlier month window changed: %d -> %d", octMonth, m)
	}
	if m, _ := st.Reserved(ctx, "month", "", "2026-11"); m != 120_000 {
		t.Fatalf("the later month holds %d, want 120000", m)
	}
	rep, err := economics.BuildReport(ctx, st, mustSpend(t, env), clk.Now())
	if err != nil || rep.Month != "2026-11" {
		t.Fatalf("the report after a step back reads window %q (%v), want 2026-11", rep.Month, err)
	}
}
