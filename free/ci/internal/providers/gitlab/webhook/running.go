package webhook

import (
	"context"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
)

// fromRunning handles an authenticated running Job Hook without enqueuing a demand.
// Inputs: verified project facts and the decoded payload. Outputs: a decline or registrar error.
// Constraints: only a bound project id may address the API numerically; bearer claims use the path.
func (s *Source) fromRunning(ctx context.Context, f model.TriggerFacts, p glPayload) (model.TriggerFacts, error) {
	if p.BuildID <= 0 || p.Runner == nil || p.Runner.ID <= 0 {
		return f, unsupported("running job without a job and runner")
	}
	id, _ := strconv.ParseInt(f.RepoID, 10, 64)
	_, err := runners.HandleRunning(ctx, runners.RunningEvent{
		Project: f.Repo, ProjectID: id, JobID: p.BuildID, RunnerID: p.Runner.ID,
	})
	if err != nil {
		return f, err
	}
	return f, unsupported(`job status "running": handled`)
}
