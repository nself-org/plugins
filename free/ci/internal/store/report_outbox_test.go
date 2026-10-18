package store

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestOutboxSeqMonotonic(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	a, err := s.OutboxReserve(ctx, "rev", "nself-ci")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.OutboxReserve(ctx, "rev", "nself-ci")
	if err != nil || b != a+1 {
		t.Fatalf("seq %d then %d (%v)", a, b, err)
	}
	other, _ := s.OutboxReserve(ctx, "rev2", "nself-ci")
	if other != 1 {
		t.Fatalf("a new revision restarts at 1, got %d", other)
	}
	if err = s.OutboxReady(ctx, "rev", "nself-ci", b, []byte(`{"state":"success"}`)); err != nil {
		t.Fatal(err)
	}
	r, tb := claim(s, b)
	if r != ClaimOK {
		t.Fatalf("newest seq must post: %v", r)
	}
	if err = s.OutboxPosted(ctx, "rev", "nself-ci", b, tb); err != nil {
		t.Fatal(err)
	}
	if err = s.OutboxReady(ctx, "rev", "nself-ci", a, []byte(`{"state":"failure"}`)); err != nil {
		t.Fatal(err)
	}
	if r, _ := claim(s, a); r != ClaimSuperseded {
		t.Fatal("an older seq must not post after a newer one")
	}
	left, err := s.OutboxUnposted(ctx)
	if err != nil || len(left) != 0 {
		t.Fatalf("superseded entry must leave the unposted list: %v %v", left, err)
	}
}

func TestOutboxSurvivesReopen(t *testing.T) {
	path := t.TempDir() + "/state.db"
	ctx := context.Background()
	s, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	seq, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	if err = s.OutboxReady(ctx, "rev", "nself-ci", seq, []byte(`{"state":"success"}`)); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	left, err := s.OutboxUnposted(ctx)
	if err != nil || len(left) != 1 || left[0].Seq != seq || string(left[0].Payload) != `{"state":"success"}` {
		t.Fatalf("outbox lost across restart: %+v %v", left, err)
	}
	if err = s.OutboxReady(ctx, "rev", "nself-ci", 99, nil); err == nil {
		t.Fatal("an unknown seq must be refused")
	}
}

func TestPipelineReads(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	seed(t, s, "1")
	row, ok, err := s.PipelineByID(ctx, "p1")
	if err != nil || !ok || row.Revision != "rev" {
		t.Fatalf("PipelineByID: %+v %v %v", row, ok, err)
	}
	if _, ok, _ = s.PipelineByID(ctx, "nope"); ok {
		t.Fatal("unknown pipeline found")
	}
	jobs, err := s.PipelineJobs(ctx, "p1")
	if err != nil || len(jobs) == 0 {
		t.Fatalf("PipelineJobs: %v %v", jobs, err)
	}
	if err = s.SetJobStatus(ctx, jobs[0].ID, "passed"); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishPipeline(ctx, "p1", "pass"); err != nil {
		t.Fatal(err)
	}
	row, _, _ = s.PipelineByID(ctx, "p1")
	if row.Status != "pass" {
		t.Fatalf("status %q", row.Status)
	}
	if _, err = s.PipelineAttempts(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if ids, err := s.PendingCancels(ctx, "p1"); err != nil || len(ids) != 0 {
		t.Fatalf("PendingCancels: %v %v", ids, err)
	}
}

func claim(s *Store, seq int64) (int, int64) {
	r, token, _ := s.OutboxClaim(context.Background(), "rev", "nself-ci", seq)
	return r, token
}

// TestOutboxClaimOrdering: a newer seq claimed first supersedes the older one;
// an older seq mid-post makes the newer wait; a released claim can be retaken.
func TestOutboxClaimOrdering(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	a, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	b, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	for _, seq := range []int64{a, b} {
		if err := s.OutboxReady(ctx, "rev", "nself-ci", seq, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	r, ta := claim(s, a)
	if r != ClaimOK {
		t.Fatalf("claim older: %d", r)
	}
	if r, _ = claim(s, a); r != ClaimBusy {
		t.Fatalf("a live claim must not be taken over: %d", r)
	}
	if r, _ = claim(s, b); r != ClaimWait {
		t.Fatalf("newer seq must wait for the older post in flight: %d", r)
	}
	if err := s.OutboxPosted(ctx, "rev", "nself-ci", a, ta); err != nil {
		t.Fatal(err)
	}
	r, tb := claim(s, b)
	if r != ClaimOK {
		t.Fatalf("newer after older posted: %d", r)
	}
	if err := s.OutboxRelease(ctx, "rev", "nself-ci", b, tb); err != nil {
		t.Fatal(err)
	}
	r, tb = claim(s, b)
	if r != ClaimOK {
		t.Fatalf("released claim retaken: %d", r)
	}
	c, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	_ = s.OutboxReady(ctx, "rev", "nself-ci", c, []byte(`{}`))
	if err := s.OutboxPosted(ctx, "rev", "nself-ci", b, tb); err != nil {
		t.Fatal(err)
	}
	if r, _ = claim(s, c); r != ClaimOK {
		t.Fatalf("c: %d", r)
	}
}

// TestOutboxClaimFencing: a poster suspended past the claim TTL loses its
// claim; its late release does nothing, its late posted is refused (E607) and
// re-queues the newest posted status so the host ends on it.
func TestOutboxClaimFencing(t *testing.T) {
	now := time.Now()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	a, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	b, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	for _, seq := range []int64{a, b} {
		_ = s.OutboxReady(ctx, "rev", "nself-ci", seq, []byte(`{"seq":`+strconv.FormatInt(seq, 10)+`}`))
	}
	r, stale := claim(s, a)
	if r != ClaimOK {
		t.Fatalf("a: %d", r)
	}
	now = now.Add(claimTTL + time.Second) // the first poster is suspended; its claim expires
	r, tb := claim(s, b)
	if r != ClaimOK {
		t.Fatalf("b after expiry: %d", r)
	}
	if err = s.OutboxPosted(ctx, "rev", "nself-ci", b, tb); err != nil {
		t.Fatal(err)
	}
	// a failed post of the suspended poster may still have landed after b: release requeues b
	if err = s.OutboxRelease(ctx, "rev", "nself-ci", a, stale); !IsClaimLost(err) {
		t.Fatalf("a release behind a newer posted status must requeue it: %v", err)
	}
	if err = s.OutboxPosted(ctx, "rev", "nself-ci", a, stale); !IsClaimLost(err) {
		t.Fatalf("a stale poster must lose: %v", err)
	}
	left, _ := s.OutboxUnposted(ctx)
	if len(left) != 2 || left[1].Seq != b || string(left[1].Payload) != `{"seq":2}` {
		t.Fatalf("the newest status must be queued again, after the released one, with its payload: %+v", left)
	}
}

// A poster whose claim was never released or taken over still lost the race
// when a newer status was delivered meanwhile: its late delivery is healed.
func TestOutboxLateDeliveryIsHealed(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	a, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	b, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	for _, seq := range []int64{a, b} {
		_ = s.OutboxReady(ctx, "rev", "nself-ci", seq, []byte(`{}`))
	}
	_, ta := claim(s, a)
	if _, err := s.writer.Exec("UPDATE outbox SET state='posted' WHERE seq=?", b); err != nil { // b delivered while a was suspended
		t.Fatal(err)
	}
	if err := s.OutboxPosted(ctx, "rev", "nself-ci", a, ta); !IsClaimLost(err) {
		t.Fatalf("late delivery must be reported: %v", err)
	}
	if left, _ := s.OutboxUnposted(ctx); len(left) != 1 || left[0].Seq != b {
		t.Fatalf("the newest status must be queued again: %+v", left)
	}
}

// The reordered acknowledgement: the older poster resumes and acknowledges
// while the newer one is still posting. Both must end healed: the newer
// status is queued again and the newer poster's own acknowledgement fails.
func TestOutboxReorderedAcknowledgementIsHealed(t *testing.T) {
	now := time.Now()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	a, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	b, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	for _, seq := range []int64{a, b} {
		_ = s.OutboxReady(ctx, "rev", "nself-ci", seq, []byte(`{}`))
	}
	_, ta := claim(s, a)
	now = now.Add(claimTTL + time.Second)
	r, tb := claim(s, b)
	if r != ClaimOK {
		t.Fatalf("b after expiry: %d", r)
	}
	if err = s.OutboxPosted(ctx, "rev", "nself-ci", a, ta); !IsClaimLost(err) { // a acknowledges first, b still posting
		t.Fatalf("a: %v", err)
	}
	if err = s.OutboxPosted(ctx, "rev", "nself-ci", b, tb); !IsClaimLost(err) {
		t.Fatalf("b must also be told, so one of them delivers the newest again: %v", err)
	}
	if left, _ := s.OutboxUnposted(ctx); len(left) != 1 || left[0].Seq != b {
		t.Fatalf("the newest status must be pending: %+v", left)
	}
}
