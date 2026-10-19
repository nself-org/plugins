package planview

// Purpose: the placement block of one job: chosen runner facts, score
// components and the wait, runtime and cost estimate with its confidence.
// Inputs: the world, the chosen runner, the sched placement and explanation.
// Outputs: model.Placement and the cost fact the totals add up.
// Constraints: a component whose confidence is unknown yields a nil estimate,
// never its stand-in value; local and owned runners cost 0 and that is known.

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// costFact is one placed job's cost for the totals.
type costFact struct {
	micro      *int64
	confidence string
}

// costRank orders cost confidences from best to worst.
var costRank = map[string]int{"known": 0, "computed": 1, "estimated": 2, "unknown": 3}

func placement(w sched.World, r sched.Runner, p sched.Placement, x sched.Explanation) (model.Placement, costFact) {
	out := model.Placement{RunnerID: p.RunnerID, Provider: r.Provider, Location: r.Location, CostClass: r.Class}
	for _, a := range x.Alternatives {
		if a.RunnerID == p.RunnerID {
			if a.Score != nil {
				out.Score = *a.Score
			}
			out.Components = a.Components
			break
		}
	}
	conf := costConfidence(w, r)
	cf := costFact{confidence: conf}
	if conf != "unknown" {
		c := p.CostMicro
		cf.micro = &c
	}
	out.Estimate = model.Estimate{
		WaitMs:         known(out.Components["wait"]),
		RuntimeMs:      known(out.Components["runtime"]),
		CostMicro:      cf.micro,
		CostConfidence: conf,
	}
	return out, cf
}

// known returns the component value unless it is only a stand-in.
func known(c model.Component) *int64 {
	if c.Confidence == "" || c.Confidence == "unknown" {
		return nil
	}
	v := c.ValueMs
	return &v
}

func costConfidence(w sched.World, r sched.Runner) string {
	if r.Class == "local" || r.Class == "owned" {
		return "known"
	}
	if c, ok := w.Costs[r.ID]; ok {
		if _, valid := costRank[c.Confidence]; valid {
			return c.Confidence
		}
	}
	return "unknown"
}

// total adds the known costs and reports the worst confidence; with no placed
// job nothing is known about the cost.
func total(costs []costFact) (int64, string) {
	if len(costs) == 0 {
		return 0, "unknown"
	}
	sum, worst := int64(0), "known"
	for _, c := range costs {
		if c.micro != nil {
			sum, _ = sched.Add(sum, *c.micro)
		}
		if costRank[c.confidence] > costRank[worst] {
			worst = c.confidence
		}
	}
	return sum, worst
}
