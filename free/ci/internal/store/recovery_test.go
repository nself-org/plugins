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

// CloseDeadRuns closes a running run whose coordinator is gone and whose
// attempts are all settled (lost, cancelled by a sweep), and never a run whose
// coordinator is alive, even one with a remote-lease "lost" attempt, nor one
// between waves.
func TestCloseDeadRuns(t *testing.T) {
	now := time.Now()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{TTL: time.Second, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	dead, err := s.RegisterCoordinator(ctx, 1, "start", "boot")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	live, err := s.RegisterCoordinator(ctx, 2, "start", "boot")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		seed(t, s, id)
	}
	if _, err = s.writer.Exec("UPDATE pipeline SET status='running'"); err != nil {
		t.Fatal(err)
	}
	owner := map[string]string{"a": dead.ID, "b": live.ID, "c": dead.ID, "d": live.ID}
	for id, c := range owner {
		if _, err = s.writer.Exec("UPDATE attempt SET coordinator_id=? WHERE id=?", c, id); err != nil {
			t.Fatal(err)
		}
	}
	walk := func(id string, to ...model.JobState) {
		from, epoch := model.JobState("queued"), int64(0)
		for _, next := range to {
			if err := s.testTransition(ctx, id, from, next, TransitionDetail{Epoch: epoch, FailureClass: "lost"}); err != nil {
				t.Fatal(err)
			}
			from, epoch = next, epoch+1
		}
	}
	walk("a", "leased", "running", "lost", "retryable") // dead coordinator, settled
	walk("b", "leased", "running", "lost", "retryable") // live coordinator, remote lease failure then retry
	walk("c", "leased", "cancelled")                    // dead coordinator, cancelled by a sweep
	walk("d", "leased", "running", "finalizing", "passed")
	// a run that never got an attempt: closed only once older than 3 x TTL
	if err = s.CreatePipeline(ctx, PipelineRow{ID: "pold", Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Second)
	if err = s.CreatePipeline(ctx, PipelineRow{ID: "pnew", Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Heartbeat(ctx, live.ID); err != nil { // the live coordinator keeps beating
		t.Fatal(err)
	}
	got, err := s.CloseDeadRuns(ctx, "boot")
	if err != nil || len(got) != 3 || got[0] != "pa" || got[1] != "pc" || got[2] != "pold" {
		t.Fatalf("closed %v %v (pb and pd belong to a live coordinator, pnew is young)", got, err)
	}
	if row, _, _ := s.PipelineByID(ctx, "pb"); row.Status != "running" {
		t.Fatalf("a live run was closed: %+v", row)
	}
}
