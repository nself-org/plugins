package economics

// Purpose: hands the dispatcher (internal/run, which this package cannot be imported by) the
// economics calls it needs: Bind before it starts, ApplyPolicy for the spend keys, NotePending
// and the worst case of a whole pipeline for a job held with approval.pending (D-1280).
// Inputs:  run.SetEconomics from init.
// Outputs: PipelineWorst, and the registered hooks.
// Constraints: the worst case prices every job at the dearest paid runner of the world, with
// saturating sums, so it never understates what the pipeline could hold; a world with no
// paid runner gives zero and nothing is noted; no spend value is changed here.

import (
	"context"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func init() {
	cirun.SetEconomics(cirun.EconHooks{
		Bind: func(e cirun.EconEnv) (func(), error) {
			return Bind(Env{Store: e.Store, Spend: e.Spend, Now: e.Now, Coordinator: e.Coordinator})
		},
		Apply:   ApplyPolicy,
		Pending: func(ctx context.Context, run string, est int64) error { return NotePending(ctx, run, est) },
		Worst:   PipelineWorst,
	})
}

// PipelineWorst is the worst-case spend of a pipeline whose jobs may run timeoutsMs each, each at
// the dearest paid runner in costs. A saturated figure is math.MaxInt64.
func PipelineWorst(costs map[string]sched.Cost, timeoutsMs []int64) int64 {
	var total int64
	for _, t := range timeoutsMs {
		var worst int64
		for _, c := range costs {
			if !isPaid(c.Class) || c.RateMicro <= 0 {
				continue
			}
			rs := store.RateSnap{RateMicro: c.RateMicro, RateUnitMs: c.DivMs, IncMs: c.BillingIncrementMs, MinMicro: c.MinimumChargeMicro, ExtrasMicro: c.ExtrasMicro}
			if m, _, _ := Charge(rs, t); m > worst {
				worst = m
			}
		}
		var of bool
		if total, of = addSat(total, worst); of {
			return total
		}
	}
	return total
}
