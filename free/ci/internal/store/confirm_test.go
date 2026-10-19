package store

// Purpose: tests of the store side of --confirm-flaky (P7-CI-40).

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestConfirmationIsExplicitAndSingleUse(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a")
	ctx := context.Background()
	if err := s.AuthorizeConfirmation(ctx, "coordinator", "a", "confirm-flaky 1/1"); !codeIs(err, "E609") {
		t.Fatalf("confirmation of an attempt that has not failed: %v", err)
	}
	for _, step := range []struct {
		from, to model.JobState
		epoch    int64
		class    model.FailureClass
	}{{"queued", "leased", 0, ""}, {"leased", "running", 1, ""}, {"running", "failed", 2, "code"}} {
		if err := s.Transition(ctx, "a", step.from, step.to, TransitionDetail{Epoch: step.epoch, FailureClass: step.class}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "a2", JobID: "ja", N: 2}, ""); !codeIs(err, "E604") {
		t.Fatalf("a failed code attempt was retried without a request: %v", err)
	}
	if err := s.AuthorizeConfirmation(ctx, "coordinator", "a", "confirm-flaky 1/1"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "a2", JobID: "ja", N: 2}, ""); err != nil {
		t.Fatalf("confirmation attempt refused: %v", err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "a3", JobID: "ja", N: 3}, ""); !codeIs(err, "E604") {
		t.Fatalf("one request allowed two attempts: %v", err)
	}
}

func TestConfirmationRefusedForInfraAndOnceOnlyJobs(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a")
	ctx := context.Background()
	for _, step := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "running", 1}} {
		if err := s.Transition(ctx, "a", step.from, step.to, TransitionDetail{Epoch: step.epoch}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Transition(ctx, "a", "running", "failed", TransitionDetail{Epoch: 2, FailureClass: "infra"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeConfirmation(ctx, "coordinator", "a", "x"); !codeIs(err, "E609") {
		t.Fatalf("an infra failure was confirmable: %v", err)
	}
}

func TestForgedConfirmationDoesNotRetryInfraFailure(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a")
	ctx := context.Background()
	for _, step := range []struct {
		from, to model.JobState
		epoch    int64
		class    model.FailureClass
	}{{"queued", "leased", 0, ""}, {"leased", "running", 1, ""}, {"running", "failed", 2, "infra"}} {
		if err := s.Transition(ctx, "a", step.from, step.to, TransitionDetail{Epoch: step.epoch, FailureClass: step.class}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AppendAudit(ctx, "forger", "confirm-flaky", "a", "label only"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "a2", JobID: "ja", N: 2}, ""); !codeIs(err, "E604") {
		t.Fatalf("an audit label authorised a retry of an infra failure: %v", err)
	}
}
