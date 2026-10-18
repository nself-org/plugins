package store

import (
	"context"
	"testing"
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
	if r, err := s.OutboxClaim(ctx, "rev", "nself-ci", b); err != nil || r != ClaimOK {
		t.Fatalf("newest seq must post: %v %v", r, err)
	}
	if err = s.OutboxPosted(ctx, "rev", "nself-ci", b); err != nil {
		t.Fatal(err)
	}
	if err = s.OutboxReady(ctx, "rev", "nself-ci", a, []byte(`{"state":"failure"}`)); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.OutboxClaim(ctx, "rev", "nself-ci", a); r != ClaimSuperseded {
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
	if r, err := s.OutboxClaim(ctx, "rev", "nself-ci", a); err != nil || r != ClaimOK {
		t.Fatalf("claim older: %d %v", r, err)
	}
	if r, _ := s.OutboxClaim(ctx, "rev", "nself-ci", a); r != ClaimBusy {
		t.Fatalf("a live claim must not be taken over: %d", r)
	}
	if r, _ := s.OutboxClaim(ctx, "rev", "nself-ci", b); r != ClaimWait {
		t.Fatalf("newer seq must wait for the older post in flight: %d", r)
	}
	if err := s.OutboxPosted(ctx, "rev", "nself-ci", a); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.OutboxClaim(ctx, "rev", "nself-ci", b); r != ClaimOK {
		t.Fatalf("newer after older posted: %d", r)
	}
	if err := s.OutboxRelease(ctx, "rev", "nself-ci", b); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.OutboxClaim(ctx, "rev", "nself-ci", b); r != ClaimOK {
		t.Fatalf("released claim retaken: %d", r)
	}
	c, _ := s.OutboxReserve(ctx, "rev", "nself-ci")
	_ = s.OutboxReady(ctx, "rev", "nself-ci", c, []byte(`{}`))
	if err := s.OutboxPosted(ctx, "rev", "nself-ci", b); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.OutboxClaim(ctx, "rev", "nself-ci", c); r != ClaimOK {
		t.Fatalf("c: %d", r)
	}
}
