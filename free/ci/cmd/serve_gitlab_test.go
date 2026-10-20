package main

// Purpose: the GitLab provider wiring of the coordinator start (D-1346): after serveGitLab the provider keeps
//          its dispatch journal in the coordinator's store (it survives a restart) and the GitLab commit
//          status reads the run's phase from the store; the undo returns both defaults.
// Inputs:  a real store with a seeded run.
// Outputs: pass or fail.
// Constraints: no network; the status hook is observed through status.PhaseOf, the journal through the
//          default provider's Journal.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	glstatus "github.com/nself-org/plugins/free/ci/internal/providers/gitlab/status"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const glSHA = "0123456789abcdef0123456789abcdef01234567"

func seedGitLabRun(t *testing.T, st *store.Store, project, status string) {
	t.Helper()
	if err := st.CreatePipeline(context.Background(), store.PipelineRow{ID: "run-" + status, Project: project, Revision: glSHA, Trigger: "push", SourceTrust: "owner", PrivacyZone: "hosted-allowed", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: status}); err != nil {
		t.Fatal(err)
	}
}

// TestServeGitLabJournalSurvivesRestart: an intent written before a coordinator restart is still there after
// it, so Collect finds the pipeline and a re-dispatch of the same attempt makes no second one.
func TestServeGitLabJournalSurvivesRestart(t *testing.T) {
	st := serveTestStore(t)
	ctx := context.Background()
	rec := pipeline.Record{Attempt: "att", Lineage: "line", N: 1, Revision: glSHA, SpecDigest: "d", Nonce: "nonce-1", Project: "o/r", Mode: pipeline.ModeInputs}
	undo := serveGitLab(st)
	if _, _, err := pipeline.Default().Journal().Begin(ctx, rec); err != nil {
		t.Fatal(err)
	}
	undo() // the process ends; the next start wires the same store again
	if _, ok, _ := pipeline.Default().Journal().ByNonce(ctx, "nonce-1"); ok {
		t.Fatal("after undo the journal is not back in memory")
	}
	t.Cleanup(serveGitLab(st))
	got, ok, err := pipeline.Default().Journal().ByNonce(ctx, "nonce-1")
	if err != nil || !ok || got.Attempt != "att" {
		t.Fatalf("intent after restart: %+v ok=%v err=%v", got, ok, err)
	}
}

// TestServeGitLabPhase: a pending status of a running run is posted as running, a failed status of a
// cancelled run as canceled; another project's run, an unknown revision and the undone wiring refine nothing.
func TestServeGitLabPhase(t *testing.T) {
	st := serveTestStore(t)
	seedGitLabRun(t, st, "o/r", "running")
	status := func(owner string) report.Status {
		return report.Status{Owner: owner, Repo: "r", Revision: glSHA, State: report.Pending}
	}
	if got := glstatus.PhaseOf(status("o")); got != glstatus.PhaseNone {
		t.Fatalf("before the wiring: phase %q, want none", got)
	}
	undo := serveGitLab(st)
	if got := glstatus.PhaseOf(status("o")); got != glstatus.PhaseRunning {
		t.Fatalf("running run: phase %q, want running", got)
	}
	if got := glstatus.PhaseOf(status("other")); got != glstatus.PhaseNone {
		t.Fatalf("another project's run: phase %q, want none", got)
	}
	unknown := status("o")
	unknown.Revision = "ffffffffffffffffffffffffffffffffffffffff"
	if got := glstatus.PhaseOf(unknown); got != glstatus.PhaseNone {
		t.Fatalf("unknown revision: phase %q, want none", got)
	}
	if err := st.FinishPipeline(context.Background(), "run-running", "cancelled"); err != nil {
		t.Fatal(err)
	}
	failed := status("o")
	failed.State = report.Failure
	if got := glstatus.PhaseOf(failed); got != glstatus.PhaseCanceled {
		t.Fatalf("cancelled run: phase %q, want canceled", got)
	}
	undo()
	if got := glstatus.PhaseOf(failed); got != glstatus.PhaseNone {
		t.Fatalf("after undo: phase %q, want none", got)
	}
}
