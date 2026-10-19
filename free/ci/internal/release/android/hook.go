package android

// Purpose: plug the SBOM step into the run (Epic CACHE D13/D18): a workspace
// hook that runs after the lane's job exited 0 and before the artifacts are
// collected from its workspace.
// Inputs:  the attempt (run.AttemptInfo) and its workspace root.
// Outputs: the SBOM documents inside the workspace, or the step's error, which
// fails the attempt.
// Constraints: only the job named android of kind release is touched; Install
// puts the hook AHEAD of the registered ones (the artifacts hook collects what
// the step wrote, so the order matters) and returns the undo.

import (
	"context"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/run"
)

// Hook is the workspace hook of the lane.
type Hook struct{ Step SBOMStep }

// AfterJob implements run.WorkspaceHook.
func (h Hook) AfterJob(ctx context.Context, a run.AttemptInfo, ws *os.Root) error {
	if !Applies(a.JobName, a.Job) {
		return nil
	}
	return h.Step.Run(ctx, a.Job, ws)
}

// Install registers the hook ahead of the others and returns the function that removes it.
func Install(step SBOMStep) func() {
	old := run.Registered.Workspaces
	run.Registered.Workspaces = append([]run.WorkspaceHook{Hook{step}}, old...)
	return func() { run.Registered.Workspaces = old }
}
