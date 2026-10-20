package pipeline

// Purpose: Liveness and Cancel of a pipeline (Epic D11).
// Inputs:  the handle returned by Dispatch.
// Outputs: queued | running | finished | vanished; an idempotent cancel.
// Constraints: created, waiting_for_resource, preparing, waiting_for_callback, pending, scheduled and
//          manual -> queued; running and canceling -> running; success, failed, canceled and skipped
//          -> finished; 404 -> vanished; any other status is E704 (the driver retries a transient
//          error and ends the run on anything else). Cancel is POST .../cancel, which GitLab answers
//          with success whatever the pipeline's state; a 404 or 409 is success too, so a second
//          Cancel never fails.

import (
	"context"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
)

func intToString(n int64) string { return strconv.FormatInt(n, 10) }

// runHandle encodes a project and a pipeline id as the handle's RunID.
func runHandle(p api.Project, id int64) string { return p.String() + "/" + intToString(id) }

// parseHandle splits a handle's RunID back into the project and the numeric pipeline id.
func parseHandle(runID string) (api.Project, int64, bool) {
	i := strings.LastIndex(runID, "/")
	if i < 0 {
		return api.Project{}, 0, false
	}
	project, ok := api.ParseProject(runID[:i])
	n, err := strconv.ParseInt(runID[i+1:], 10, 64)
	if !ok || err != nil || n <= 0 {
		return api.Project{}, 0, false
	}
	return project, n, true
}

func (p *Provider) handleOf(h providers.Handle) (api.Project, int64, error) {
	project, id, ok := parseHandle(h.RunID)
	if !ok || h.Provider != p.ID() {
		return api.Project{}, 0, api.Misconfig("handle")
	}
	return project, id, nil
}

// Liveness implements providers.Dispatcher.
func (p *Provider) Liveness(ctx context.Context, h providers.Handle) (providers.LiveState, error) {
	project, id, err := p.handleOf(h)
	if err != nil {
		return "", err
	}
	c, err := p.client(ctx)
	if err != nil {
		return "", err
	}
	var pl api.Pipeline
	if _, err = c.Do(ctx, "GET", api.PipelinePath, project.With("pipeline", intToString(id)), nil, &pl); err != nil {
		if api.IsStatus(err, 404) {
			return providers.Vanished, nil
		}
		return "", err
	}
	switch pl.Status {
	case "created", "waiting_for_resource", "preparing", "waiting_for_callback", "pending", "scheduled", "manual":
		return providers.Queued, nil
	case "running", "canceling":
		return providers.Running, nil
	case "success", "failed", "canceled", "skipped":
		return providers.Finished, nil
	}
	return "", api.Unavailable("run.status")
}

// Cancel implements providers.Dispatcher.
func (p *Provider) Cancel(ctx context.Context, h providers.Handle) error {
	project, id, err := p.handleOf(h)
	if err != nil {
		return err
	}
	c, err := p.client(ctx)
	if err != nil {
		return err
	}
	return p.cancelPipeline(ctx, c, project, id)
}
