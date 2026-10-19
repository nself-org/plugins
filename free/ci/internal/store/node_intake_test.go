package store

// Purpose: tests of the durable coordinator intake (P7-NODE-10): enqueue, delivery dedup, semantic
// coalescing, forced enqueue with its audit row, idempotency keys, retention, claim and recovery.
// Inputs:  a temporary state.db and an injected store clock.
// Outputs: pass or fail.
// Constraints: no network; time moves only through the injected clock.

import (
	"context"
	"testing"
	"time"
)

type intakeClock struct{ t time.Time }

func (c *intakeClock) Now() time.Time          { return c.t }
func (c *intakeClock) add(d time.Duration)     { c.t = c.t.Add(d) }
func intakeStore(t *testing.T) (*Store, *intakeClock) {
	t.Helper()
	s, clk := testStore(t), &intakeClock{t: time.Unix(1_000_000, 0)}
	s.now = clk.Now
	return s, clk
}

func req(delivery, key string) IntakeRequest {
	return IntakeRequest{Source: "github", DeliveryID: delivery, Project: "o/r", Revision: "abc1234", TriggerClass: "change",
		PolicyDigest: "p", SemanticKey: key, Facts: []byte(`{}`), Webhook: true}
}

func TestIntakeEnqueueDuplicateAndCoalesce(t *testing.T) {
	ctx := context.Background()
	s, _ := intakeStore(t)
	a, err := s.IntakeEnqueue(ctx, req("d1", "k"))
	if err != nil || a.Kind != IntakeEnqueued {
		t.Fatalf("first enqueue = %+v, %v", a, err)
	}
	if b, _ := s.IntakeEnqueue(ctx, req("d1", "k")); b.Kind != IntakeDuplicate || b.ID != a.ID {
		t.Fatalf("same delivery id = %+v, want duplicate of %s", b, a.ID)
	}
	if c, _ := s.IntakeEnqueue(ctx, req("d2", "k")); c.Kind != IntakeCoalesced || c.ID != a.ID {
		t.Fatalf("same semantic key = %+v, want coalesced into %s", c, a.ID)
	}
	if d, _ := s.IntakeEnqueue(ctx, req("d3", "other")); d.Kind != IntakeEnqueued || d.ID == a.ID {
		t.Fatalf("other key = %+v, want a new row", d)
	}
	// the coalesced delivery id is remembered: a redelivery of d2 is a duplicate, not a new row
	if e, _ := s.IntakeEnqueue(ctx, req("d2", "k")); e.Kind != IntakeDuplicate {
		t.Fatalf("redelivery of a coalesced id = %+v", e)
	}
	if st, _ := s.IntakeStats(ctx); st.Queued != 2 {
		t.Fatalf("queued = %d, want 2", st.Queued)
	}
}

func TestIntakeForceWritesAuditRow(t *testing.T) {
	ctx := context.Background()
	s, _ := intakeStore(t)
	a, _ := s.IntakeEnqueue(ctx, req("d1", "k"))
	before := auditCount(t, s)
	r := req("d2", "k")
	r.Force, r.Actor = true, "api"
	b, err := s.IntakeEnqueue(ctx, r)
	if err != nil || b.Kind != IntakeEnqueued || b.ID == a.ID {
		t.Fatalf("forced enqueue = %+v, %v", b, err)
	}
	if got := auditCount(t, s); got != before+1 {
		t.Fatalf("audit rows = %d, want %d", got, before+1)
	}
	var forced bool
	if err = s.readers.QueryRow(`SELECT forced FROM intake WHERE id=?`, b.ID).Scan(&forced); err != nil || !forced {
		t.Fatalf("forced flag = %v, %v", forced, err)
	}
}

func TestIntakeCompletedWithinHorizonCoalescesWebhooks(t *testing.T) {
	ctx := context.Background()
	s, clk := intakeStore(t)
	a, _ := s.IntakeEnqueue(ctx, req("d1", "k"))
	row, ok, err := s.IntakeClaim(ctx, "me")
	if err != nil || !ok || row.ID != a.ID {
		t.Fatalf("claim = %+v %v %v", row, ok, err)
	}
	if err = s.IntakeFinish(ctx, a.ID, "me", "run1", "exit=0", true); err != nil {
		t.Fatal(err)
	}
	clk.add(71 * time.Hour)
	if b, _ := s.IntakeEnqueue(ctx, req("d2", "k")); b.Kind != IntakeCoalesced || b.ID != a.ID {
		t.Fatalf("webhook 71 h after completion = %+v, want coalesced", b)
	}
	manual := req("", "k")
	manual.Webhook = false
	if c, _ := s.IntakeEnqueue(ctx, manual); c.Kind != IntakeEnqueued {
		t.Fatalf("manual trigger after completion = %+v, want a new run", c)
	}
	r := req("d3", "k")
	r.Force = true
	if d, _ := s.IntakeEnqueue(ctx, r); d.Kind != IntakeEnqueued {
		t.Fatalf("--force = %+v, want a new run", d)
	}
	clk.add(2 * time.Hour) // 73 h: past the horizon
	if e, _ := s.IntakeEnqueue(ctx, req("d4", "k2")); e.Kind != IntakeEnqueued {
		t.Fatalf("unrelated key = %+v", e)
	}
}

func TestIntakeIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	s, _ := intakeStore(t)
	r := req("", "k")
	r.Webhook, r.IdemDigest, r.ReqDigest = false, "key1", "req1"
	a, err := s.IntakeEnqueue(ctx, r)
	if err != nil || a.Kind != IntakeEnqueued {
		t.Fatalf("first = %+v, %v", a, err)
	}
	if b, err := s.IntakeEnqueue(ctx, r); err != nil || b.Kind != IntakeReplay || b.ID != a.ID {
		t.Fatalf("replay = %+v, %v", b, err)
	}
	r.ReqDigest = "req2"
	if _, err = s.IntakeEnqueue(ctx, r); !codeIs(err, "E608") {
		t.Fatalf("reused key = %v, want E608", err)
	}
}

func TestIntakeClaimRecoverRequeueFinish(t *testing.T) {
	ctx := context.Background()
	s, clk := intakeStore(t)
	a, _ := s.IntakeEnqueue(ctx, req("d1", "k"))
	if _, ok, _ := s.IntakeClaim(ctx, "dead"); !ok {
		t.Fatal("nothing to claim")
	}
	if _, ok, _ := s.IntakeClaim(ctx, "other"); ok {
		t.Fatal("a claimed row was claimed twice")
	}
	if n, _ := s.IntakeRecover(ctx, "me", time.Minute); n != 0 {
		t.Fatalf("recovered %d fresh claims", n)
	}
	clk.add(2 * time.Minute)
	if n, err := s.IntakeRecover(ctx, "me", time.Minute); err != nil || n != 1 {
		t.Fatalf("recover = %d, %v", n, err)
	}
	row, ok, _ := s.IntakeClaim(ctx, "me")
	if !ok || row.ID != a.ID || row.Attempts != 2 {
		t.Fatalf("reclaim = %+v %v", row, ok)
	}
	if err := s.IntakeFinish(ctx, a.ID, "dead", "", "x", true); !codeIs(err, "E605") {
		t.Fatalf("finish by a stale owner = %v, want E605", err)
	}
	if err := s.IntakeRequeue(ctx, a.ID, "me"); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.IntakeStats(ctx); st.Queued != 1 || st.Dispatched != 0 {
		t.Fatalf("stats after requeue = %+v", st)
	}
}

func TestDedupPruneForgetsOldRows(t *testing.T) {
	ctx := context.Background()
	s, clk := intakeStore(t)
	r := req("d1", "k")
	r.IdemDigest, r.ReqDigest = "key", "req"
	a, _ := s.IntakeEnqueue(ctx, r)
	_, _, _ = s.IntakeClaim(ctx, "me")
	_ = s.IntakeFinish(ctx, a.ID, "me", "run", "exit=0", true)
	clk.add(73 * time.Hour)
	if n, err := s.DedupPrune(ctx, 0); err != nil || n != 3 {
		t.Fatalf("pruned %d, %v; want the row, its delivery and its key", n, err)
	}
	if b, _ := s.IntakeEnqueue(ctx, req("d1", "k")); b.Kind != IntakeEnqueued {
		t.Fatalf("a forgotten delivery id = %+v, want a fresh run", b)
	}
}
