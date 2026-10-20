package main

// Purpose: link the GitHub forge-runner launcher (P7-HOST-19) into the binary so its init() registers the
// demand handler for source github, the github-jitconfig minter, the transport preparer that mints the
// credential after admission, the orphan sweep Reconciler and the policy key providers.github.jit.max_pending;
// and register the run finalizer that deregisters a run's runners (the launcher package may not import run,
// so the adapter lives here). Dormant unless compat.V15(). The coordinator start must call
// demand.Bind(store) and demand.UsePolicy before the first demand (P7-NODE-20 wiring); until then the
// handler answers E704 and the demand is offered again.

import (
	"context"

	_ "github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// runnerFinalizer adapts runnerjob.Finalize to run.Finalizer.
type runnerFinalizer struct{}

func (runnerFinalizer) Name() string { return "runnerjob" }

func (runnerFinalizer) Finalize(ctx context.Context, info cirun.RunInfo) error {
	attempts := make([]string, 0, len(info.Jobs))
	for _, j := range info.Jobs {
		attempts = append(attempts, j.AttemptID)
	}
	return runnerjob.Finalize(ctx, attempts)
}

func init() { cirun.RegisterFinalizer(runnerFinalizer{}) }
