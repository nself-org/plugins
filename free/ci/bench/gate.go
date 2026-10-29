package bench

// Purpose: the two pieces of the end-to-end tiny run that the scheduler harness
// (bench/sched, P7-SCHED-14) pairs its own timings with: one V15 gate run of the
// nself-ci binary, and the number of jobs the planner puts in that pipeline.
// Inputs:  the binary, the materialized fixture, the shared NSELF_CI_HOME.
// Outputs: the wall time of one V15 gate run; the planned job count.
// Constraints: the arguments and environment are the ones the cibench loop uses
// for the V15 mode, so the pairing denominator is the same run as ci.e2e_tiny.v15.

import (
	"context"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// GateV15 runs the V15 gate once on repo and returns its wall time.
func GateV15(ctx context.Context, bin, repo, home string) (Timing, error) {
	m := modes[1]
	env := append(baseEnv(), "NSELF_CI_HOME="+home, "NSELF_TELEMETRY_OPT_OUT=1", "NSELF_V15="+m.env)
	r, err := must(ctx, append([]string{bin}, m.args(repo)...), repo, env)
	if err != nil {
		return r, fmt.Errorf("e2e v15 gate: %w", err)
	}
	return r, nil
}

// PlannedJobs is the number of jobs the planner schedules for the fixture's gate pipeline.
func (p *Inproc) PlannedJobs() (int, error) {
	pl, err := plan.Build(plan.Input{Config: p.cfg, Facts: p.facts, World: p.world,
		Revision: plan.Revision{Branch: "main", DefaultBranch: true}, Pipeline: "gate"})
	if err != nil {
		return 0, err
	}
	return len(pl.Jobs), nil
}
