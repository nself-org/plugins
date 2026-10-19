package store

// Purpose: P7-AUTO-28 (D-1044, retry anti-affinity). The runner that lost an attempt is
// remembered for its retry; a re-queued attempt does not keep the identity of its last
// admission; retention removes the admission and refusal rows with their attempts.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func admittedRows(t *testing.T, s *Store, table, id string) int {
	t.Helper()
	var n int
	if err := s.readers.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table+" WHERE attempt_id=?", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func leaseOn(t *testing.T, s *Store, id, runner string) {
	t.Helper()
	if _, err := s.LeaseGrant(context.Background(), "host", Demand{AttemptID: id, LeaseID: "lease-" + id, RunnerID: runner, CPU: 1, MemMB: 1, CapacityCPU: 10, CapacityMemMB: 10, TTL: time.Minute}, 1); err != nil {
		t.Fatal(err)
	}
}

func move(t *testing.T, s *Store, id, from, to string) {
	t.Helper()
	_, epoch, err := s.AttemptStateEpoch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(context.Background(), id, model.JobState(from), model.JobState(to), TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil {
		t.Fatalf("%s -> %s: %v", from, to, err)
	}
}

func TestSchedAdmissionLostAttemptRemembersItsRunner(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	seed(t, s, "lostrun")
	leaseOn(t, s, "lostrun", "nodeX")
	move(t, s, "lostrun", "leased", "lost")
	if got, err := s.LeasePreviousRunner(ctx, "lostrun"); err != nil || got != "nodeX" {
		t.Fatalf("previous runner of the lost attempt = %q, %v", got, err)
	}
	seed(t, s, "retry")
	if err := s.SetPreviousRunner(ctx, "retry", "nodeX"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LeasePreviousRunner(ctx, "retry"); got != "nodeX" {
		t.Fatalf("a queued retry did not record its previous runner: %q", got)
	}
	snap, err := s.WorldSnapshot(ctx)
	if err != nil || snap.Previous["retry"] != "nodeX" {
		t.Fatalf("the world must carry the retry's anti-affinity: %v %v", snap.Previous, err)
	}
	leaseOn(t, s, "retry", "nodeY")
	if err := s.SetPreviousRunner(ctx, "retry", "nodeZ"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LeasePreviousRunner(ctx, "retry"); got != "nodeX" {
		t.Fatalf("a leased attempt's previous runner was rewritten: %q", got)
	}
}

func TestSchedAdmissionRequeueDropsTheOldIdentity(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	seed(t, s, "requeue")
	leaseOn(t, s, "requeue", "nodeX")
	if _, err := s.writer.ExecContext(ctx, "INSERT INTO sched_admitted(attempt_id,pipeline_id,project,user_id,team_id,provider,runner_id,world_digest,admitted_at) VALUES (?,?,?,?,?,?,?,?,?)", "requeue", "prequeue", "proj", "u", "t", "", "nodeX", "dg", 1); err != nil {
		t.Fatal(err)
	}
	move(t, s, "requeue", "leased", "lost")
	move(t, s, "requeue", "lost", "retryable")
	if admittedRows(t, s, "sched_admitted", "requeue") != 1 {
		t.Fatal("the identity of the admission must survive until the attempt is queued again (telemetry reads it after the run)")
	}
	move(t, s, "requeue", "retryable", "queued")
	if admittedRows(t, s, "sched_admitted", "requeue") != 0 {
		t.Fatal("a re-queued attempt kept the identity of its last admission")
	}
}

func TestSchedAdmissionRowsArePrunedWithTheirAttempts(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	for _, id := range []string{"gone", "stay"} {
		seed(t, s, id)
		if _, err := s.writer.ExecContext(ctx, "INSERT INTO sched_admitted(attempt_id,pipeline_id,project,user_id,team_id,provider,runner_id,world_digest,admitted_at) VALUES (?,?,?,?,?,?,?,?,?)", id, "p"+id, "proj", "", "", "", "local", "dg", 1); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordRefusal(ctx, id, "capacity.busy", "x"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.writer.ExecContext(ctx, "UPDATE pipeline SET status='passed', finished_at=1 WHERE id='pgone'"); err != nil {
		t.Fatal(err)
	}
	if n, err := s.DeletePipelines(ctx, []string{"pgone"}); err != nil || n != 1 {
		t.Fatalf("removed %d, %v", n, err)
	}
	for _, table := range []string{"sched_admitted", "sched_refusal"} {
		if admittedRows(t, s, table, "gone") != 0 || admittedRows(t, s, table, "stay") != 1 {
			t.Errorf("%s: the pruned attempt's row must go and the other's stay", table)
		}
	}
}
