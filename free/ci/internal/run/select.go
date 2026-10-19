package run

// Purpose: which planned jobs a run covers: --job selection and the pre-run
// check that every required job can be placed.
// Inputs:  the plan and the --job name. Outputs: the kept job ids or an error.
// Constraints: moved out of coordinator.go unchanged (P7-AUTO-27) so that file
// stays under the 300-line limit.

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// subset returns the jobs --job selects with their dependencies; nil means all.
func subset(pl *plan.Plan, name string) (map[string]bool, error) {
	if name == "" {
		return nil, nil
	}
	by := map[string]plan.PlannedJob{}
	var start []string
	for _, pj := range pl.Jobs {
		by[pj.ID] = pj
		if pj.ID == name || pj.Name == name || pj.Check == name {
			start = append(start, pj.ID)
		}
	}
	if len(start) == 0 {
		return nil, fail(CodeOptions, "--job %q matches no planned job", name)
	}
	keep := map[string]bool{}
	for len(start) > 0 {
		id := start[len(start)-1]
		start = start[:len(start)-1]
		if keep[id] {
			continue
		}
		keep[id] = true
		start = append(start, by[id].Deps...)
	}
	return keep, nil
}

// requireRunnable fails at once for a required job no runner can run.
func requireRunnable(pl *plan.Plan, keep map[string]bool) error {
	for _, pj := range pl.Jobs {
		if pj.State == plan.StateUnplaceable && (keep == nil || keep[pj.ID]) && pj.Job.Required != nil && *pj.Job.Required {
			return fail(plan.CodeUnplaceable, "job %s cannot be placed (%s): %s", pj.ID, pj.Blocked, pj.Detail)
		}
	}
	return nil
}

// selectionNote explains an exit code that differs from the evidence result.
const selectionNote = "--job: the selected jobs passed; the evidence result stays fail because jobs outside the selection did not run, so this run proves less than the full gate (D-1091)"

// selectionExit is the exit code of a --job run (D-1091). The evidence stays
// fail-closed (an excluded required job is a skip), but the process answers
// for the jobs the caller selected: it is 0 when they all passed. A run that
// was cancelled, errored or has no selection keeps the evidence's code.
func selectionExit(ev model.Evidence, keep map[string]bool) (int, bool) {
	if keep == nil || ev.Result != "fail" {
		return exitFor(ev.Result), false
	}
	var chosen []model.Check
	for _, c := range ev.Checks {
		if keep[c.JobID] {
			chosen = append(chosen, c)
		}
	}
	if model.Verdict(chosen) != "pass" {
		return exitFor(ev.Result), false
	}
	return exitFor("pass"), true
}
