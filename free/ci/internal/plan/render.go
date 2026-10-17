package plan

// Purpose: the output side of `nself ci plan`: the placement hook, the
// human table and the fail-fast cancellation set.
// Inputs:  a built Plan and, optionally, the PlaceFunc that
// cmd.placePlan holds (set from init() of the SCHED files).
// Outputs: Plan.Placement (the `data.placement` key) and extra human cells;
// with no PlaceFunc nothing is added and every other key is unchanged.
// Constraints: the hook runs after the plan is built and validated and only
// reads the model.Pipeline it is given; Placement is the last JSON key, so
// adding it leaves the bytes before it identical.

import (
	"context"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// PlaceFunc is the type of cmd's placePlan hook.
type PlaceFunc func(ctx context.Context, p *model.Pipeline) (data any, human []string, err error)

// ModelPipeline is the persisted pipeline: Jobs align index for index with
// Plan.Jobs.
func (pl *Plan) ModelPipeline() *model.Pipeline {
	jobs := make([]model.Job, len(pl.Jobs))
	for i, j := range pl.Jobs {
		jobs[i] = cloneJob(j.Job)
	}
	return &model.Pipeline{ID: pl.Pipeline.ID, Name: pl.Pipeline.Name, Kind: pl.Pipeline.Kind, Selection: pl.Selection, Jobs: jobs}
}

// ApplyPlacement calls place (when non-nil) and stores its data under
// Placement. The returned strings are the human placement cells: one per
// job when the count matches, otherwise footer lines.
func (pl *Plan) ApplyPlacement(ctx context.Context, place PlaceFunc) ([]string, error) {
	if place == nil {
		return nil, nil
	}
	data, human, err := place(ctx, pl.ModelPipeline())
	if err != nil {
		return nil, err
	}
	pl.Placement = data
	return human, nil
}

// Human renders the plan as a table; cells (from ApplyPlacement) add a
// PLACEMENT column, or footer lines when they do not match the job count.
func (pl *Plan) Human(cells []string) []string {
	perJob := len(cells) == len(pl.Jobs) && len(cells) > 0
	head := fmt.Sprintf("pipeline %s (%s)  trigger %s  selection %s (%s)  fail-fast %s",
		pl.Pipeline.Name, pl.Pipeline.Kind, pl.Trigger, pl.Selection.Mode, pl.Selection.Reason, pl.FailFast)
	cols := "LAYER  JOB  PATH  REQ  STATE  REASON"
	if perJob {
		cols += "  PLACEMENT"
	}
	out := []string{head, cols}
	for i, j := range pl.Jobs {
		req := "optional"
		if j.Job.Required != nil && *j.Job.Required {
			req = "required"
		}
		line := fmt.Sprintf("%d  %s  %s  %s  %s  %s", j.Layer, j.ID, j.Job.Path, req, j.State, j.Reason)
		if j.Blocked != "" {
			line += " [" + string(j.Blocked) + ": " + j.Detail + "]"
		}
		if perJob {
			line += "  " + cells[i]
		}
		out = append(out, line)
	}
	for _, s := range pl.Skipped {
		out = append(out, fmt.Sprintf("skipped  %s  %s", s.Job, s.Reason))
	}
	if !perJob {
		out = append(out, cells...)
	}
	return out
}

// CancelSet returns the jobs cancelled when job id fails and the reason:
// its transitive dependants (dep.failed) or, with pipeline scope, every
// other job (fail_fast.cancelled). An allow_failure job cancels nothing.
func (pl *Plan) CancelSet(id string) ([]string, model.Reason) {
	var self *PlannedJob
	for i := range pl.Jobs {
		if pl.Jobs[i].ID == id {
			self = &pl.Jobs[i]
		}
	}
	if self == nil || self.Job.AllowFailure {
		return nil, ""
	}
	if pl.FailFast == FailFastPipeline {
		var all []string
		for _, j := range pl.Jobs {
			if j.ID != id {
				all = append(all, j.ID)
			}
		}
		return sortedUnique(all), "fail_fast.cancelled"
	}
	return append([]string(nil), self.Dependants...), "dep.failed"
}
