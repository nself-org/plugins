package run

// Purpose: P7-NODE-11 unit tests of an attempt the lease engine's expiry sweep already failed: it is retryable and
// the store queued the successor row itself. The run adopts that row as its next attempt instead of creating a
// second one; a release or at-most-once attempt sent to needs-operator ends the job without an error.
// Inputs:  a private store with a leased remote attempt, failed through store.LeaseFail.
// Outputs: assertions on the outcome and the attempt rows.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// sweptEnv fails the lease of the env's attempt the way the sweeper does (expired).
func sweptEnv(t *testing.T) (*remoteEnv, *runState, *runDispatch) {
	t.Helper()
	e, rs, rd := lostEnv(t)
	ctx := context.Background()
	rl, err := e.st.LeaseGet(ctx, e.lease)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.st.LeaseFail(ctx, rl, time.Now().Add(2*time.Minute), "lease.expired", true); err != nil {
		t.Fatal(err)
	}
	return e, rs, rd
}

// boundSession is a session over the env's store in which job is the row run1.jobA.
func boundSession(e *remoteEnv, job *model.Job) *Session {
	s := newSession()
	s.Store, s.RunID = e.st, "run1"
	s.bind(job, "run1.jobA")
	return s
}

func TestSweptAttemptIsParkedForTheRunsRetryAndTheEngineRowIsAdopted(t *testing.T) {
	e, rs, rd := sweptEnv(t)
	if e.state(t) != "retryable" {
		t.Fatalf("fixture: the swept attempt is %s, want retryable", e.state(t))
	}
	job := model.Job{Retry: &model.RetryConfig{InfraMax: 1}}
	o := rs.remoteLost(context.Background(), planJob("unit"), job, rd, e.id, errors.New("the carrier ended"), JobOutcome{ID: "unit"}, 1, &tail{})
	if !o.parked || o.FailureClass != "infra" {
		t.Fatalf("the swept attempt was not parked for a retry: %+v", o)
	}
	if e.state(t) != "retryable" {
		t.Fatalf("the swept attempt moved to %s; the run must leave it", e.state(t))
	}

	// the next attempt is the row the engine queued, not a second one
	id, err := AttemptIDFor(WithSession(context.Background(), boundSession(e, &job)), AttemptSpec{Pipeline: &model.Pipeline{}, Job: &job, N: 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := e.id + "-retry-2"; id != want {
		t.Fatalf("attempt 2 is %q, want the engine's row %q", id, want)
	}
	if got, _, found, err := e.st.AttemptAt(context.Background(), "run1.jobA", 2); err != nil || !found || got != id {
		t.Fatalf("AttemptAt(2) = %q %v %v", got, found, err)
	}
}

func TestAttemptIDIsTheRunsOwnWhenNothingWasQueued(t *testing.T) {
	e, _, _ := lostEnv(t)
	job := model.Job{}
	id, err := AttemptIDFor(WithSession(context.Background(), boundSession(e, &job)), AttemptSpec{Pipeline: &model.Pipeline{}, Job: &job, N: 2})
	if err != nil || id != "run1.jobA.a2" {
		t.Fatalf("attempt 2 = %q (%v), want run1.jobA.a2 (attempt 1 is %s)", id, err, e.state(t))
	}
}

func TestSweptAttemptSentToNeedsOperatorEndsTheJobWithoutAnError(t *testing.T) {
	e, rs, rd := lostEnv(t)
	ctx := context.Background()
	for _, to := range []string{"lost", "needs-operator"} {
		from, epoch, _ := e.st.AttemptStateEpoch(ctx, e.id)
		if err := e.st.Transition(ctx, e.id, from, model.JobState(to), store.TransitionDetail{Epoch: epoch, FailureClass: "lost", Reason: "lease.expired"}); err != nil {
			t.Fatal(err)
		}
	}
	job := model.Job{Retry: &model.RetryConfig{InfraMax: 1}}
	o := rs.remoteLost(ctx, planJob("unit"), job, rd, e.id, errors.New("the carrier ended"), JobOutcome{ID: "unit"}, 1, &tail{})
	if o.State != StateFailed || o.parked || o.Code != "" || e.state(t) != "needs-operator" {
		t.Fatalf("outcome %+v, durable %s: want a failed job, no retry, no error code, attempt needs-operator", o, e.state(t))
	}
}
