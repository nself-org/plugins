package main

// D-1188: `ci logs -f` stops following when the attempt cannot write output any
// more. A retryable attempt is parked until the coordinator queues it again, so
// after a coordinator crash it must end the follow, as lost and failed do.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

func TestAttemptEndedForParkedAndLiveStates(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.CreatePipeline(ctx, store.PipelineRow{ID: "run-1", Revision: sealRev, Project: "proj", Trigger: "local", SourceTrust: "owner",
		PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "in1", SelectionMode: "full", Status: "running"}))
	must(st.CreateJob(ctx, store.JobRow{ID: "run-1.test-1", PipelineID: "run-1", Name: "test", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}))
	const attempt = "run-1.test-1.a1"
	must(st.CreateAttempt(ctx, store.AttemptRow{ID: attempt, JobID: "run-1.test-1", N: 1}, ""))

	step := func(from, to model.JobState) {
		t.Helper()
		_, epoch, err := st.AttemptStateEpoch(ctx, attempt)
		must(err)
		if to == "leased" {
			must(storetest.Admit(ctx, st, attempt))
			return
		}
		must(st.Transition(ctx, attempt, from, to, store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}))
	}
	if attemptEnded(ctx, st, attempt) {
		t.Fatal("a queued attempt can still write output")
	}
	step("queued", "leased")
	step("leased", "running")
	if attemptEnded(ctx, st, attempt) {
		t.Fatal("a running attempt can still write output")
	}
	step("running", "lost")
	if !attemptEnded(ctx, st, attempt) {
		t.Fatal("a lost attempt cannot write output")
	}
	step("lost", "retryable")
	if !attemptEnded(ctx, st, attempt) {
		t.Fatal("a retryable attempt is parked; following it would wait forever after a coordinator crash")
	}
}
