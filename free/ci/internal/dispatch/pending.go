package dispatch

// Purpose: tells the economics layer what a run waits to spend (D-1280).
// Inputs:  the pipelines of one tick with their placement explanations.
// Outputs: one Config.PendingSpend call per pipeline that has jobs awaiting approval.
// Constraints: the figure is the sum of the worst-case costs of the pipeline's held jobs, so one
// approval of it releases the whole run; the call repeats every tick while jobs stay held and the
// store keeps the largest figure, so a repeat is harmless. A failure never fails the tick.

import (
	"context"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func (d *Dispatcher) notePending(ctx context.Context, runs map[string]*pipelineRun) {
	if d.cfg.PendingSpend == nil {
		return
	}
	ids := make([]string, 0, len(runs))
	for id := range runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var total int64
		for _, x := range runs[id].explained {
			if x.Status != "awaiting-approval" {
				continue
			}
			var of bool
			if total, of = sched.Add(total, x.EstimateMicro); of {
				break
			}
		}
		if total > 0 {
			_ = d.cfg.PendingSpend(ctx, id, total)
		}
	}
}
