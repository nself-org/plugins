package cloudvm_test

// Purpose: the one job of a burst node in the full-cycle test: queue an attempt, admit it on the enrolled
// node through the real admission steps (the `ephemeral` step reads the node's intent binding and
// its runner labels), refuse a second job, and drive the attempt to passed so the engine's Tick ends the intent.
// Inputs:  the rig and the node's registered capability. Outputs: helpers only.

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var jobSeq atomic.Int64

// queueAttempt creates a pipeline (once), a job and its queued attempt.
func queueAttempt(t testing.TB, st *store.Store, pipe string) string {
	t.Helper()
	ctx := context.Background()
	_ = st.CreatePipeline(ctx, store.PipelineRow{ID: pipe, Revision: "rev", Project: "project", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "cfg1", SelectionMode: "full", Status: "running"})
	id := pipe + ".job" + strconv.FormatInt(jobSeq.Add(1), 10)
	if err := st.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: pipe, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	return id
}

func admitRequest(attempt, runner string, labels []string) dispatch.Request {
	r := sched.Runner{ID: runner, Provider: providerID, Ephemeral: true}
	r.Capability.Identity.Labels = labels
	return dispatch.Request{AttemptID: attempt, Placement: sched.Placement{JobKey: attempt, RunnerID: runner, CPU: 1, MemMB: 256}, Runner: r,
		Job: sched.Job{Key: attempt, Trust: "owner", TimeoutMs: 60_000}, Local: true, Host: "host-" + attempt, CapacityCPU: 8, CapacityMemMB: 8192}
}

// runOneJob admits one job on the node, shows a second is refused, and finishes the first.
func (r *rig) runOneJob(node string, labels []string) {
	r.t.Helper()
	ctx := context.Background()
	first, second := queueAttempt(r.t, r.st, "pipe-1"), queueAttempt(r.t, r.st, "pipe-1")
	if _, err := dispatch.Admit(ctx, r.st, admitRequest(first, node, labels)); err != nil {
		r.t.Fatalf("the first job on the enrolled node: %v", err)
	}
	_, err := dispatch.Admit(ctx, r.st, admitRequest(second, node, labels))
	var ref *dispatch.Refusal
	if !errors.As(err, &ref) || ref.Reason != model.EphemeralUsed {
		r.t.Fatalf("a second job on a burst node must be refused as ephemeral.used, got %v", err)
	}
	for _, step := range [][2]model.JobState{{"leased", "running"}, {"running", "finalizing"}, {"finalizing", "passed"}} {
		_, epoch, err := r.st.AttemptStateEpoch(ctx, first)
		if err != nil {
			r.t.Fatal(err)
		}
		if err = r.st.Transition(ctx, first, step[0], step[1], store.TransitionDetail{Epoch: epoch}); err != nil {
			r.t.Fatal(err)
		}
	}
	// a node with a label for another intent is refused (the budget step would charge the wrong reservation)
	third := queueAttempt(r.t, r.st, "pipe-1")
	if _, err = dispatch.Admit(ctx, r.st, admitRequest(third, node, append([]string{"nself.burst.intent=OTHERINTENT"}, labels...))); err == nil {
		r.t.Fatal("a runner naming another intent was admitted")
	}
}
