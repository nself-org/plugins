package store

import (
	"context"
	"testing"
	"time"
)

// TestOutboxRetryColumns: a failed delivery is parked with its attempt and
// time, is not due early, becomes deliverable when its time passes, and the
// migration is expand-only (0001 and 0002 rows keep working).
func TestOutboxRetryColumns(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	seq, _ := s.OutboxReserve(ctx, "rev", "nself-ci@o/r")
	if err := s.OutboxReady(ctx, "rev", "nself-ci@o/r", seq, []byte(`{"state":"success"}`)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	due, err := s.OutboxDue(ctx, now)
	if err != nil || len(due) != 1 || due[0].Attempts != 0 || due[0].NextAttemptAt != 0 {
		t.Fatalf("new entry: %+v %v", due, err)
	}
	res, token, _ := s.OutboxClaim(ctx, "rev", "nself-ci@o/r", seq)
	if res != ClaimOK {
		t.Fatalf("claim = %d", res)
	}
	if err = s.OutboxBackoff(ctx, "rev", "nself-ci@o/r", seq, token, 1, now.Add(30*time.Second), "offline"); err != nil {
		t.Fatal(err)
	}
	if due, _ = s.OutboxDue(ctx, now); len(due) != 0 {
		t.Fatalf("a parked entry is not due early: %+v", due)
	}
	list, _ := s.OutboxList(ctx)
	if len(list) != 1 || list[0].State != OutboxFailed || list[0].Attempts != 1 || list[0].LastError != "offline" {
		t.Fatalf("list: %+v", list)
	}
	if due, _ = s.OutboxDue(ctx, now.Add(31*time.Second)); len(due) != 1 || due[0].Attempts != 1 {
		t.Fatalf("due after the backoff: %+v", due)
	}
	if err = s.OutboxBackoff(ctx, "rev", "nself-ci@o/r", seq, token, 2, now, "stale token"); err != nil {
		t.Fatal(err) // a claim that is not ours changes nothing
	}
	if due, _ = s.OutboxDue(ctx, now.Add(31*time.Second)); len(due) != 1 || due[0].Attempts != 1 {
		t.Fatalf("a stale token must not park the entry: %+v", due)
	}
}

// TestOutboxDueCollapses: only the highest undelivered seq of a key stays,
// even when it is parked and a lower one is due.
func TestOutboxDueCollapses(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	const k = "nself-ci@o/r"
	lo, _ := s.OutboxEnqueue(ctx, "rev", k, []byte(`{"state":"pending"}`))
	hi, _ := s.OutboxEnqueue(ctx, "rev", k, []byte(`{"state":"success"}`))
	other, _ := s.OutboxEnqueue(ctx, "rev2", k, []byte(`{"state":"success"}`))
	if lo != 1 || hi != 2 || other != 1 {
		t.Fatalf("seqs %d %d %d", lo, hi, other)
	}
	now := time.Now()
	_, token, _ := s.OutboxClaim(ctx, "rev", k, hi)
	if err := s.OutboxBackoff(ctx, "rev", k, hi, token, 1, now.Add(time.Hour), "x"); err != nil {
		t.Fatal(err)
	}
	due, err := s.OutboxDue(ctx, now)
	if err != nil || len(due) != 1 || due[0].Revision != "rev2" {
		t.Fatalf("the lower seq must not post past a parked higher one: %+v %v", due, err)
	}
	var state string
	if err = s.readers.QueryRowContext(ctx, "SELECT state FROM outbox WHERE revision='rev' AND seq=1").Scan(&state); err != nil || state != OutboxSuperseded {
		t.Fatalf("lower seq state = %q %v", state, err)
	}
}

func TestOutboxSuppress(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	seq, _ := s.OutboxEnqueue(ctx, "rev", "nself-ci@o/r", []byte(`{}`))
	if err := s.OutboxSuppress(ctx, "rev", "nself-ci@o/r", seq, "team owns it"); err != nil {
		t.Fatal(err)
	}
	list, _ := s.OutboxList(ctx)
	if len(list) != 1 || list[0].State != OutboxSuppressed || list[0].Reason != "team owns it" {
		t.Fatalf("list: %+v", list)
	}
	if due, _ := s.OutboxDue(ctx, time.Now()); len(due) != 0 {
		t.Fatalf("a suppressed entry is never delivered here: %+v", due)
	}
}

// TestOutboxSuppressTakesAnExpiredClaim: a posting entry whose claim expired
// (a crashed poster) can be parked, so it stops coming back on every drain.
func TestOutboxSuppressTakesAnExpiredClaim(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	seq, _ := s.OutboxEnqueue(ctx, "rev", "nself-ci@o/r", []byte(`{}`))
	if res, _, _ := s.OutboxClaim(ctx, "rev", "nself-ci@o/r", seq); res != ClaimOK {
		t.Fatalf("claim = %d", res)
	}
	later := time.Now().Add(2 * claimTTL)
	if due, _ := s.OutboxDue(ctx, later); len(due) != 1 || due[0].State != OutboxPosting {
		t.Fatalf("an expired claim is due: %+v", due)
	}
	if err := s.OutboxSuppress(ctx, "rev", "nself-ci@o/r", seq, "team"); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.OutboxDue(ctx, later); len(due) != 0 {
		t.Fatalf("still due after suppress: %+v", due)
	}
	if err := s.OutboxRequeue(ctx, "rev", "nself-ci@o/r", seq); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.OutboxDue(ctx, later); len(due) != 1 || due[0].State != OutboxPending || due[0].Reason != "" {
		t.Fatalf("requeued: %+v", due)
	}
}
