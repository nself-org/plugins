package store

import (
	"context"
	"path/filepath"
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
	ctx, now := context.Background(), time.Now()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	seq, _ := s.OutboxEnqueue(ctx, "rev", "nself-ci@o/r", []byte(`{}`))
	if res, _, _ := s.OutboxClaim(ctx, "rev", "nself-ci@o/r", seq); res != ClaimOK {
		t.Fatalf("claim = %d", res)
	}
	later := now.Add(2 * claimTTL)
	now = later // the store clock moves with the sender clock
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

// TestOutboxClaimCountsAParkedNewerSeq: the claim every engine uses refuses an
// older seq while a newer one is parked for a retry (it may have landed).
func TestOutboxClaimCountsAParkedNewerSeq(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	const k = "nself-ci@o/r"
	older, _ := s.OutboxEnqueue(ctx, "rev", k, []byte(`{}`))
	newer, _ := s.OutboxEnqueue(ctx, "rev", k, []byte(`{}`))
	_, token, _ := s.OutboxClaim(ctx, "rev", k, newer)
	if err := s.OutboxBackoff(ctx, "rev", k, newer, token, 1, time.Now().Add(time.Hour), "offline"); err != nil {
		t.Fatal(err)
	}
	if res, _, _ := s.OutboxClaim(ctx, "rev", k, older); res != ClaimSuperseded {
		t.Fatalf("an older seq claimed past a parked newer one: %d", res)
	}
}

// A live claim is a poster's: Suppress leaves it alone.
func TestOutboxSuppressLeavesALiveClaim(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	seq, _ := s.OutboxEnqueue(ctx, "rev", "nself-ci@o/r", []byte(`{}`))
	_, token, _ := s.OutboxClaim(ctx, "rev", "nself-ci@o/r", seq)
	if err := s.OutboxSuppress(ctx, "rev", "nself-ci@o/r", seq, "team"); err != nil {
		t.Fatal(err)
	}
	if err := s.OutboxPosted(ctx, "rev", "nself-ci@o/r", seq, token); err != nil {
		t.Fatalf("the poster lost its claim: %v", err)
	}
}

func TestOutboxReviveBelow(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	const k = "nself-ci@o/r"
	lo, _ := s.OutboxEnqueue(ctx, "rev", k, []byte(`{"a":1}`))
	hi, _ := s.OutboxEnqueue(ctx, "rev", k, []byte(`{bad`))
	if due, _ := s.OutboxDue(ctx, time.Now()); len(due) != 1 || due[0].Seq != hi { // collapse superseded lo
		t.Fatalf("due: %+v", due)
	}
	if err := s.OutboxSuppress(ctx, "rev", k, hi, "unreadable payload"); err != nil {
		t.Fatal(err)
	}
	if err := s.OutboxReviveBelow(ctx, "rev", k, hi); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.OutboxDue(ctx, time.Now()); len(due) != 1 || due[0].Seq != lo {
		t.Fatalf("lower entry not revived: %+v", due)
	}
	// nothing is revived under a posted newer status
	const k2 = "nself-ci@o/r2"
	_, _ = s.OutboxEnqueue(ctx, "r2", k2, []byte(`{}`))
	posted, _ := s.OutboxEnqueue(ctx, "r2", k2, []byte(`{}`))
	_, tb, _ := s.OutboxClaim(ctx, "r2", k2, posted)
	if err := s.OutboxPosted(ctx, "r2", k2, posted, tb); err != nil {
		t.Fatal(err)
	}
	bad, _ := s.OutboxEnqueue(ctx, "r2", k2, []byte(`{bad`))
	if due, _ := s.OutboxDue(ctx, time.Now()); len(due) != 2 { // lo (pending, revived above) and bad; seq 1 of r2 is superseded
		t.Fatalf("due: %+v", due)
	}
	if err := s.OutboxSuppress(ctx, "r2", k2, bad, "unreadable payload"); err != nil {
		t.Fatal(err)
	}
	if err := s.OutboxReviveBelow(ctx, "r2", k2, bad); err != nil {
		t.Fatal(err)
	}
	for _, it := range mustDue(t, s) {
		if it.Revision == "r2" {
			t.Fatalf("revived under a posted newer status: %+v", it)
		}
	}
}

func mustDue(t *testing.T, s *Store) []OutboxItem {
	t.Helper()
	due, err := s.OutboxDue(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return due
}
