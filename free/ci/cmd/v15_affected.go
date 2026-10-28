package main

// Purpose: wires the package-graph affected selection (P7-SCHED-13, Epic
// D15) into the planner: the pkggraph detector joins the detect registry that
// planChecks uses, and the affected selector replaces the path-glob default
// through plan.SetSelector.
// Inputs:  the detect registry (planDetectors) and plan.SelectionInput.
// Outputs: pkggraph facts in the plan input; a model.Selection and job ids.
// Constraints: v1.5 behaviour only: with compat.V15() false both hooks leave
// v1.4 untouched (the stock registry and plan.DefaultSelector); the logic
// lives in internal/affected, which is pure; no flags, no subcommand.

import (
	"github.com/nself-org/cli/sdk/go/v2/compat"

	"github.com/nself-org/plugins/free/ci/internal/affected"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func init() { plan.SetSelector(affectedSelector) }

// affectedSelector picks the selector per call, so the mode is read when the
// plan is built, not when the process starts.
func affectedSelector(in plan.SelectionInput) (model.Selection, []string) {
	// compat.V15(P7-SCHED-13): path-glob selection -> package-graph selection
	if compat.V15() {
		return affected.Selector(in)
	}
	return plan.DefaultSelector(in)
}

// planDetectors is the detect registry planChecks runs.
func planDetectors(full ...bool) *detect.Registry {
	r := detect.DefaultRegistry()
	// compat.V15(P7-SCHED-13): no package graph -> pkggraph detector registered
	if compat.V15() && (len(full) == 0 || !full[0]) {
		_ = affected.Register(r)
	}
	return r
}

// fullSelection avoids building a graph when revision rules require every job.
func fullSelection(rev plan.Revision) bool {
	return !rev.BaseKnown || rev.DefaultBranch || rev.Release || len(rev.Changed) == 0
}
