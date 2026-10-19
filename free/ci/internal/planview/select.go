package planview

// Purpose: narrow a placement-plan document to some of its jobs (the explain
// output) and keep its totals true for the jobs that remain.
// Inputs: a document and a predicate. Outputs: a new document.
// Constraints: pure; the placement of the kept jobs is not recomputed, so a
// job keeps the reservations the earlier jobs of the full plan made.

import "github.com/nself-org/plugins/free/ci/internal/model"

// Select returns p with only the jobs keep accepts, in their order.
func Select(p model.PlacementPlan, keep func(model.JobPlacement) bool) model.PlacementPlan {
	out := p
	out.Jobs = []model.JobPlacement{}
	out.Totals = model.Totals{}
	var costs []costFact
	for _, j := range p.Jobs {
		if !keep(j) {
			continue
		}
		out.Jobs = append(out.Jobs, j)
		switch j.Status {
		case "placed":
			out.Totals.Placed++
			if j.Placement != nil {
				costs = append(costs, costFact{micro: j.Placement.Estimate.CostMicro, confidence: j.Placement.Estimate.CostConfidence})
			}
		case "waiting":
			out.Totals.Waiting++
		case "awaiting-approval":
			out.Totals.AwaitingApproval++
		default:
			out.Totals.Unplaceable++
		}
	}
	out.Totals.EstimatedCostMicro, out.Totals.CostConfidence = total(costs)
	return out
}
