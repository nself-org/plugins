// Package planview turns a scheduler result into the placement-plan
// document (contract:ci.placement-plan v1) and its human lines.
//
// Purpose: the inspection surface of Epic SCHED D3/D4. It reads a finished
// sched.Result and the world it was computed from and writes nothing.
// Inputs: Input (jobs in the order Place received them, the world, the result).
// Outputs: model.PlacementPlan, per-job table cells and explain lines.
// Constraints: pure (no os, exec, net or database access); deterministic
// (the order of jobs and alternatives is the scheduler's, no clock); unknown
// values are nil with a confidence, never a zero that reads as a fact; secret
// classes appear by name, never a value.
package planview

import (
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// SchemaID is the schema field of every document.
const SchemaID = "ci.placement-plan/v1"

// Sources of an explanation.
const (
	SourcePlan           = "plan"
	SourceDecisionRecord = "decision-record"
)

// Input is everything Build reads.
type Input struct {
	Name, Revision, Trigger string
	ID                      *string // the run's pipeline id; nil for a plan
	PolicyDigest            string
	WorldDigest             string
	World                   sched.World
	Jobs                    []sched.Job // in the order Place received them
	Blocked                 []Blocked   // refused by the planner; listed after the placed jobs, unplaceable
	Result                  sched.Result
}

// Explanation is the explain output: the placement-plan document with the
// jobs asked about and where the facts came from.
type Explanation struct {
	model.PlacementPlan
	Source string `json:"source"`
}

// Build assembles the document. Jobs follow Result.Explanations, which follow
// the caller's job order; the planner's unplaceable jobs come last.
func Build(in Input) model.PlacementPlan {
	runners := map[string]sched.Runner{}
	for _, r := range append(append([]sched.Runner(nil), in.World.Runners...), in.World.Hosted...) {
		runners[r.ID] = r
	}
	placed := map[string]sched.Placement{}
	for _, p := range in.Result.Placements {
		placed[p.JobKey] = p
	}
	byKey := map[string]sched.Job{}
	for _, j := range in.Jobs {
		byKey[j.Key] = j
	}
	out := model.PlacementPlan{
		Schema: SchemaID,
		Pipeline: model.PlacementPipelineRef{ID: in.ID, Name: in.Name, Revision: in.Revision, Trigger: in.Trigger,
			PolicyDigest: in.PolicyDigest, Preset: in.World.Policy.Preset, WorldDigest: in.WorldDigest},
		FastPath: in.Result.FastPath,
		Jobs:     []model.JobPlacement{},
	}
	var costs []costFact
	for _, x := range in.Result.Explanations {
		jp := jobPlacement(byKey[x.JobKey], x)
		if p, ok := placed[x.JobKey]; ok && x.Status == "placed" {
			pl, cf := placement(in.World, runners[p.RunnerID], p, x)
			jp.Placement = &pl
			jp.ExpectedWaitMs = pl.Estimate.WaitMs
			costs = append(costs, cf)
		}
		out.Jobs = append(out.Jobs, jp)
		switch x.Status {
		case "placed":
			out.Totals.Placed++
		case "waiting":
			out.Totals.Waiting++
		case "awaiting-approval":
			out.Totals.AwaitingApproval++
		default:
			out.Totals.Unplaceable++
		}
	}
	for _, b := range in.Blocked {
		out.Jobs = append(out.Jobs, jobPlacement(b.Job, sched.Explanation{JobKey: b.Job.Key, Status: "unplaceable",
			Reasons: []model.PlacementReasonDetail{{Code: b.Code, Detail: b.Detail}}}))
		out.Totals.Unplaceable++
	}
	out.Totals.EstimatedCostMicro, out.Totals.CostConfidence = total(costs)
	return out
}

func jobPlacement(j sched.Job, x sched.Explanation) model.JobPlacement {
	req := j.Requirements
	req.Labels, req.Tools = nonNil(req.Labels), nonNil(req.Tools)
	classes := append([]model.SecretClass{}, j.SecretClasses...)
	sort.Slice(classes, func(a, b int) bool { return classes[a] < classes[b] })
	band := j.Pipeline.Band
	if band == "" {
		band = "normal"
	}
	alts := make([]model.Alternative, len(x.Alternatives))
	for i, a := range x.Alternatives {
		a.Reasons = nonNilReasons(a.Reasons)
		alts[i] = a
	}
	return model.JobPlacement{ID: j.ID, Name: j.Name, MatrixKey: j.MatrixKey, Deps: nonNil(sortedCopy(j.Deps)), Band: band,
		Requirements: req, SecretClasses: classes, Status: x.Status, Reasons: nonNilReasons(x.Reasons), Alternatives: alts}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilReasons(r []model.PlacementReasonDetail) []model.PlacementReasonDetail {
	if r == nil {
		return []model.PlacementReasonDetail{}
	}
	return r
}

func sortedCopy(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}
