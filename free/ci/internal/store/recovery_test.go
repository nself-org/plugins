package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// A committed cancel intent on an attempt of a dead coordinator must not make
// SweepStale fail (E605) and strand every other attempt: it ends cancelled.
func TestSweepStaleHonoursCancelIntent(t *testing.T) {
	now := time.Now()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{TTL: time.Second, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	seed(t, s, "a")
	seed(t, s, "b")
	c, err := s.RegisterCoordinator(ctx, 1, "start", "boot")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.writer.Exec("UPDATE attempt SET coordinator_id=?,state='running' WHERE id IN ('a','b')", c.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestCancel(ctx, "a", "operator", 0); err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Second)
	n, err := s.SweepStale(ctx, "boot")
	if err != nil || n != 2 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	for id, want := range map[string]model.JobState{"a": "cancelled", "b": "lost"} {
		if got, _, _ := s.AttemptStateEpoch(ctx, id); got != want {
			t.Fatalf("attempt %s is %s, want %s", id, got, want)
		}
	}
}

// DeadRuns names a running run whose only attempts were lost and settled, and
// never a run that still has a live attempt.
func TestDeadRuns(t *testing.T) {
	s, ctx := testStore(t), context.Background()
	seed(t, s, "a")
	seed(t, s, "b")
	if _, err := s.writer.Exec("UPDATE pipeline SET status='running'"); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		to    model.JobState
		epoch int64
	}{{"leased", 0}, {"running", 1}, {"lost", 2}, {"retryable", 3}} {
		from := map[model.JobState]model.JobState{"leased": "queued", "running": "leased", "lost": "running", "retryable": "lost"}[step.to]
		if err := s.Transition(ctx, "a", from, step.to, TransitionDetail{Epoch: step.epoch, FailureClass: "lost"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.DeadRuns(ctx)
	if err != nil || len(got) != 1 || got[0] != "pa" {
		t.Fatalf("dead runs %v %v (pb still has a queued attempt)", got, err)
	}
}
