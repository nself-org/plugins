package hosted

// Purpose: Liveness and Cancel of a hosted run (Epic D5).
// Inputs:  the handle returned by Dispatch.
// Outputs: queued | running | finished | vanished; an idempotent cancel.
// Constraints: queued, requested, pending -> queued; in_progress, waiting -> running; completed ->
//          finished; 404 -> vanished; any other status is E704 (the driver retries a transient
//          error and ends the run on anything else). Cancel is POST .../cancel: 409 (finished) and
//          404 (gone) are success, so a second Cancel never fails.

import (
	"context"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

func intToString(n int64) string { return strconv.FormatInt(n, 10) }

func (p *Provider) handleParams(h providers.Handle) (repoRef, map[string]string, error) {
	repo, run, ok := parseHandle(h.RunID)
	if !ok || h.Provider != p.ID() {
		return repoRef{}, nil, misconfig("handle")
	}
	params := repo.params()
	params["run"] = run
	return repo, params, nil
}

// Liveness implements providers.Dispatcher.
func (p *Provider) Liveness(ctx context.Context, h providers.Handle) (providers.LiveState, error) {
	_, params, err := p.handleParams(h)
	if err != nil {
		return "", err
	}
	c, err := p.client(ctx)
	if err != nil {
		return "", err
	}
	var run runInfo
	if _, err = c.Do(ctx, "GET", runPath, params, nil, &run); err != nil {
		if isStatus(err, 404) {
			return providers.Vanished, nil
		}
		return "", err
	}
	switch run.Status {
	case "queued", "requested", "pending":
		return providers.Queued, nil
	case "in_progress", "waiting":
		return providers.Running, nil
	case "completed":
		return providers.Finished, nil
	}
	return "", failure(providers.Transient, "E704", "run.status")
}

// Cancel implements providers.Dispatcher.
func (p *Provider) Cancel(ctx context.Context, h providers.Handle) error {
	repo, params, err := p.handleParams(h)
	if err != nil {
		return err
	}
	c, err := p.client(ctx)
	if err != nil {
		return err
	}
	id, _ := strconv.ParseInt(params["run"], 10, 64)
	return p.cancelRun(ctx, c, repo, id)
}
