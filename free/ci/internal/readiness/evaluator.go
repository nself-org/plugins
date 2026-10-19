package readiness

// Purpose: the host-facing entry point. An Evaluator samples this machine, runs the hook
// and the yield_to scan, and returns the Report that the exclusion provider, the watcher
// (P7-AUTO-11) and the demand file (P7-AUTO-09) read.
// Inputs: a Config source (the effective policy), a Sampler and a hook Runner.
// Outputs: Report; Evaluate returns an error only when the configuration cannot be read.
// Constraints: static host facts (core count) come from the CI-32 capacity probe through
// Cores, never re-probed here; every reader is injectable; the evaluator never pauses or
// kills a running job (readiness gates admission only).

import (
	"context"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
)

// Evaluator answers "may this host take CI work now?".
type Evaluator struct {
	// Config returns the resolved readiness settings; nil means DefaultConfig.
	Config func(ctx context.Context) (Config, error)
	// Sampler reads the live values; its Cores is filled from Cores when zero.
	Sampler sample.Sampler
	// Hook runs readiness.cmd; nil uses the exec funnel.
	Hook sample.Runner
	// Cores returns the core count from the capacity probe; nil uses ciexec.Probe.
	Cores func() int
}

// Evaluate samples the host and decides.
func (e *Evaluator) Evaluate(ctx context.Context) (Report, error) {
	cfg := DefaultConfig()
	if e.Config != nil {
		var err error
		if cfg, err = e.Config(ctx); err != nil {
			return Report{}, err
		}
	}
	s := e.Sampler
	if s.Cores < 1 {
		cores := e.Cores
		if cores == nil {
			cores = func() int { return ciexec.Probe().CPUs }
		}
		s.Cores = cores()
	}
	in := Inputs{Live: s.Sample(ctx)}
	if len(cfg.YieldTo) > 0 {
		running, err := s.Processes(ctx)
		if err != nil {
			// A host that cannot list processes cannot honour yield_to: stay out of the way.
			in.YieldErr = ClipLine(err.Error())
		} else {
			in.YieldedTo = MatchYield(cfg.YieldTo, running)
		}
	}
	if len(cfg.Cmd) > 0 {
		run := e.Hook
		if run == nil {
			run = sample.Funnel{}
		}
		in.Hook = RunHook(ctx, run, cfg.Cmd)
	}
	return Decide(cfg, in), nil
}
