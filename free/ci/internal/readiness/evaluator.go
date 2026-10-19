package readiness

// Purpose: the host-facing entry point. An Evaluator samples this machine, runs the hook
// and the yield_to scan, and returns the Report that the exclusion provider, the watcher
// (P7-AUTO-11) and the demand file (P7-AUTO-09) read.
// Inputs: a Config source (the effective policy), a Sampler and a hook Runner.
// Outputs: Report; Evaluate returns an error only when the configuration cannot be read.
// Constraints: static host facts (core count) come from the CI-32 capacity probe through
// Cores, never re-probed here; every reader is injectable; the evaluator never pauses or
// kills a running job (readiness gates admission only). A world is built on every dispatch
// tick, so an answer is reused for CacheTTL while the configuration is unchanged: a probe
// costs 250 ms on Linux and several process starts on macOS. An answer may therefore be up
// to CacheTTL old; a caller waiting behind a running probe leaves when its context ends. Config is required: a missing
// source would silently drop the operator's hook and yield_to.

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
)

// DefaultCacheTTL is how long an answer is reused when Evaluator.CacheTTL is zero.
const DefaultCacheTTL = 5 * time.Second

// Evaluator answers "may this host take CI work now?". Use it through a pointer.
type Evaluator struct {
	// Config returns the resolved readiness settings (ConfigFrom of the effective policy).
	Config func(ctx context.Context) (Config, error)
	// Sampler reads the live values; its Cores is filled from Cores when zero.
	Sampler sample.Sampler
	// Hook runs readiness.cmd; nil uses the exec funnel.
	Hook sample.Runner
	// Cores returns the core count from the capacity probe; nil uses ciexec.Probe.
	Cores func() int
	// CacheTTL bounds how long an answer is reused; zero means DefaultCacheTTL, negative
	// switches the cache off.
	CacheTTL time.Duration
	// Now is the clock for the cache; nil uses time.Now.
	Now func() time.Time

	coresOnce sync.Once
	cores     int

	semOnce  sync.Once
	sem      chan struct{} // one probe at a time; a channel so a waiting caller can give up
	cachedAt time.Time
	cachedBy Config
	cached   *Report
}

func (e *Evaluator) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Evaluate samples the host and decides.
func (e *Evaluator) Evaluate(ctx context.Context) (Report, error) {
	if e.Config == nil {
		return Report{}, fmt.Errorf("readiness: the Evaluator has no Config source")
	}
	cfg, err := e.Config(ctx)
	if err != nil {
		return Report{}, err
	}
	ttl := e.CacheTTL
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	e.semOnce.Do(func() { e.sem = make(chan struct{}, 1) })
	select { // concurrent callers queue here and share the first one's answer
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return Report{}, ctx.Err()
	}
	if ttl > 0 && e.cached != nil && e.now().Sub(e.cachedAt) < ttl && reflect.DeepEqual(e.cachedBy, cfg) {
		return e.cached.clone(), nil
	}
	r := e.probe(ctx, cfg)
	if ttl > 0 && ctx.Err() == nil {
		cached := r.clone()
		e.cached, e.cachedAt, e.cachedBy = &cached, e.now(), cfg
	}
	return r, nil
}

func (e *Evaluator) probe(ctx context.Context, cfg Config) Report {
	s := e.Sampler
	if s.Cores < 1 {
		e.coresOnce.Do(func() { // the core count does not change; the probe forks several tools on macOS
			probe := e.Cores
			if probe == nil {
				probe = func() int { return ciexec.Probe().CPUs }
			}
			e.cores = probe()
		})
		s.Cores = e.cores
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
	return Decide(cfg, in)
}
