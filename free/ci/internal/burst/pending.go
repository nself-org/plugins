package burst

// Purpose: records the spend figure of an approval.pending refusal for ApproveSpend (D-1280).
// Inputs:  the Request and the refusal from ReserveResource (it rolled its transaction back).
// Outputs: economics.NotePending with the pipeline's worst case, never less than the refusal's estimate.
// Constraints: the worst case is economics.PipelineWorst (the dispatcher's seam) priced at the
// provider's declared rate; the store keeps the maximum, so a larger earlier figure stays;
// a failure is logged without values and not returned (a missing figure makes ApproveSpend
// refuse, which is the fail-closed outcome); no spend value is changed here.

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// notePending records the estimate of an approval.pending refusal: ReserveResource rolled its
// transaction back, so nothing was noted for the run and ApproveSpend would find no figure (D-1280).
func (e *Engine) notePending(ctx context.Context, req Request, err error) {
	var se *economics.SpendError
	if !errors.As(err, &se) || se.Reason != model.ApprovalPending || se.Estimate <= 0 || req.Pipeline == "" {
		return
	}
	est := se.Estimate
	if len(req.PipelineTimeoutsMs) > 0 {
		if pol, perr := e.o.Spend(ctx); perr == nil {
			if rs, ok := economics.SnapOf(pol.Rates[req.Provider]); ok {
				costs := map[string]sched.Cost{req.Provider: {Class: economics.ClassEphemeral, RateMicro: rs.RateMicro, DivMs: rs.RateUnitMs,
					BillingIncrementMs: rs.IncMs, MinimumChargeMicro: rs.MinMicro, ExtrasMicro: rs.ExtrasMicro}}
				est = max(est, economics.PipelineWorst(costs, req.PipelineTimeoutsMs))
			}
		}
	}
	if nerr := economics.NotePending(e.econCtx(ctx), req.Pipeline, est); nerr != nil {
		slog.Warn("burst: pending spend estimate not recorded", "run", req.Pipeline, "err", nerr)
	}
}
