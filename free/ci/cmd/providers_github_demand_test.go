package main

import (
	"context"
	"testing"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// TestRunnerFinalizerIsRegisteredAndReadsEveryAttempt: a closed run reaches runnerjob.Finalize through the
// run finalizer registered here; with no ledger bound it is a no-op that succeeds.
func TestRunnerFinalizerIsRegisteredAndReadsEveryAttempt(t *testing.T) {
	var found cirun.Finalizer
	for _, f := range cirun.Registered.Finalizers {
		if f.Name() == "runnerjob" {
			found = f
		}
	}
	if found == nil {
		t.Fatal("the runnerjob finalizer is not registered")
	}
	info := cirun.RunInfo{RunID: "r", Jobs: []cirun.JobOutcome{{AttemptID: "a1"}, {AttemptID: ""}, {AttemptID: "a2"}}}
	if err := found.Finalize(context.Background(), info); err != nil {
		t.Fatalf("an unbound launcher must not fail a run: %v", err)
	}
}
