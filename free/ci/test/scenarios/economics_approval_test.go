//go:build scenarios

package scenarios

// Purpose: Epic acceptance 5, approval pause. A paid placement whose estimate is over
// economics.approval_threshold_micro stays queued with outcome awaiting-approval until an admin
// approves the spend (a member is refused and writes no approval); then the same queued job admits
// through the real dispatcher and the money is reserved in the same commit.
// Inputs:  a real store, a real dispatcher over econWorld, the bound economics env, roles actors.
// Outputs: assertions only.
// Constraints: in process, injected clock; the dispatcher polls on a 1 ms ticker and the test waits
//          on channels with a context deadline, never a sleep.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// econSink forwards every decision to a channel.
type econSink struct{ ch chan dispatch.Decision }

func (s econSink) Record(_ context.Context, d dispatch.Decision) {
	select {
	case s.ch <- d:
	default:
	}
}

// TestEconomicsApprovalPausesUntilAnAdminApproves holds a 300 000 micro job over a 100 000 threshold.
func TestEconomicsApprovalPausesUntilAnAdminApproves(t *testing.T) {
	clk := newClock()
	st := econStore(t, clk)
	p := econPolicy(t, trust.SourceProject, econKV(map[string]string{schedpolicy.KeyApproval: `100000`}), nil)
	env := econEnv(st, p, clk)
	t.Cleanup(economics.Bind(env))
	act := newEconActors(t, st, clk)
	sink := econSink{ch: make(chan dispatch.Decision, 256)}
	d := newDispatcher(t, dispatch.Config{Store: st, World: econWorld(t, st, p, clk, econPaid("paid", "metered", "cloudco")), Poll: time.Millisecond, Clock: clk.Now, Sink: sink, Transport: &transport{}})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	attempt := queueAttempt(t, st, "pipe", "project", "deploy")
	j := job(attempt, "linux/amd64")
	j.TimeoutMs = 300_000
	j.PersonalOwnRun = false
	type result struct {
		a   dispatch.Admitted
		err error
	}
	done := make(chan result, 1)
	go func() { a, err := d.Submit(ctx, dispatch.Job{AttemptID: attempt, Sched: j}); done <- result{a, err} }()

	for held := false; !held; { // wait for the first awaiting-approval decision
		select {
		case dec := <-sink.ch:
			if dec.Outcome == dispatch.OutcomeAdmitted {
				t.Fatalf("admitted before any approval: %+v", dec)
			}
			held = dec.Outcome == dispatch.OutcomeAwaitingAppr
			if held && dec.Reason != model.ApprovalPending {
				t.Fatalf("awaiting-approval carries reason %q, want %q", dec.Reason, model.ApprovalPending)
			}
		case r := <-done:
			t.Fatalf("submit returned before approval: %+v", r)
		case <-ctx.Done():
			t.Fatal("no awaiting-approval decision")
		}
	}
	if state, _, err := st.AttemptStateEpoch(ctx, attempt); err != nil || state != "queued" {
		t.Fatalf("a held job must stay queued: %q %v", state, err)
	}
	if _, found, _ := st.LedgerAttempt(ctx, attempt); found {
		t.Fatal("a held job wrote a ledger row")
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-18"); held != 0 {
		t.Fatalf("a held job reserved %d micro", held)
	}

	if err := economics.NotePending(ctx, "pipe", 300_000); err != nil {
		t.Fatal(err)
	}
	if _, err := economics.ApproveSpend(ctx, "pipe", act.member); err == nil || roles.ReasonOf(err) != roles.ReasonRole {
		t.Fatalf("a member must be refused with the role reason, got %v", err)
	}
	if ap, _ := st.Approvals(ctx, 10); len(ap) != 0 {
		t.Fatalf("a refused approval wrote a row: %+v", ap)
	}
	select {
	case r := <-done:
		t.Fatalf("a refused approval released the job: %+v", r)
	default:
	}
	ap, err := economics.ApproveSpend(ctx, "pipe", act.admin)
	if err != nil || ap.EstimateMicro != 300_000 {
		t.Fatalf("admin approval: %+v %v", ap, err)
	}
	clk.Advance(time.Second) // the dispatcher re-ticks a waiter on its injected clock, not the wall clock
	select {
	case r := <-done:
		if r.err != nil || r.a.AttemptID != attempt {
			t.Fatalf("after approval: %+v", r)
		}
	case <-ctx.Done():
		for len(sink.ch) > 0 {
			dec := <-sink.ch
			t.Logf("decision %s reason=%s alts=%+v", dec.Outcome, dec.Reason, dec.Explanation.Alternatives)
		}
		t.Fatal("the approved job never admitted")
	}
	row, found, err := st.LedgerAttempt(ctx, attempt)
	if err != nil || !found || row.Class != economics.ClassMetered || row.EstMicro != 300_000 {
		t.Fatalf("ledger after approval: %+v found=%v %v", row, found, err)
	}
	if held, _ := st.Reserved(ctx, "day", "", "2026-10-18"); held != 300_000 {
		t.Fatalf("day reservation %d, want 300000", held)
	}
}
