package storetest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestLeaseAdmitsAndRefusesBareTransition: Lease leaves the same rows admission
// does, and the bare Transition it replaces is refused.
func TestLeaseAdmitsAndRefusesBareTransition(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "ci.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p := store.PipelineRow{ID: "p", Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for _, e := range []error{s.CreatePipeline(ctx, p), s.CreateJob(ctx, store.JobRow{ID: "j", PipelineID: "p", Name: "build", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}), s.CreateAttempt(ctx, store.AttemptRow{ID: "a", JobID: "j", N: 1}, "")} {
		if e != nil {
			t.Fatal(e)
		}
	}
	if err = s.Transition(ctx, "a", "queued", "leased", store.TransitionDetail{}); err == nil {
		t.Fatal("a bare queued -> leased transition was accepted")
	}
	Lease(t, s, "a")
	state, epoch, _ := s.AttemptStateEpoch(ctx, "a")
	if state != "leased" || epoch != 1 {
		t.Fatalf("after Lease: %s epoch %d, want leased epoch 1", state, epoch)
	}
	if err = s.Transition(ctx, "a", "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
		t.Fatalf("a leased fixture still runs: %v", err)
	}
}
