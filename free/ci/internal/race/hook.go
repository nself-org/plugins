package race

// Purpose: fill the RaceHook slot P7-AUTO-06 leaves nil, so `--where all` is
// accepted once this package is linked, and drive a race for one job of a run.
// Inputs:  run.RaceRequest (run id, options, plan) before the jobs start;
// for RaceJob, the session context, the job's placement world and request.
// Outputs: the per-run race plan (which jobs race, which do not and why) and,
// per raced job, the one posted Evidence.
// Constraints: the hook never fails a run: a run it cannot race runs one
// attempt per job. A release-class or idempotent:false job is never raced.
// The width is the resolved sched.race.width of the placement world.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// Planned says whether one job of a run races.
type Planned struct {
	JobID  string
	Raced  bool
	Reason string // why not, when Raced is false
}

// Hook is the run.RaceHook of this package.
type Hook struct {
	// Enabled says the run asked for a race (`--where all`); nil: the run's
	// policy values carry sched.race.width.
	Enabled func(run.RaceRequest) bool

	mu    sync.Mutex
	plans map[string][]Planned
}

// Default is the hook init() registers.
var Default = &Hook{}

func init() { run.RegisterRace(Default) }

func (h *Hook) enabled(r run.RaceRequest) bool {
	if h.Enabled != nil {
		return h.Enabled(r)
	}
	_, ok := r.Opts.PolicyRun[KeyWidth]
	return ok
}

// Race implements run.RaceHook: it records which jobs of the run may race.
func (h *Hook) Race(_ context.Context, r run.RaceRequest) {
	if r.Plan == nil || !h.enabled(r) {
		return
	}
	plans := make([]Planned, 0, len(r.Plan.Jobs))
	for i := range r.Plan.Jobs {
		pj := &r.Plan.Jobs[i]
		p := Planned{JobID: pj.ID, Raced: run.Raceable(&pj.Job)}
		if !p.Raced {
			p.Reason = ReasonRelease
		}
		plans = append(plans, p)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.plans == nil {
		h.plans = map[string][]Planned{}
	}
	h.plans[r.RunID] = plans
}

// Planned returns the race plan of a run (nil: the run does not race).
func (h *Hook) Planned(runID string) []Planned {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Planned(nil), h.plans[runID]...)
}

// Raced says the job of the run races.
func (h *Hook) Raced(runID, jobID string) bool {
	for _, p := range h.Planned(runID) {
		if p.JobID == jobID {
			return p.Raced
		}
	}
	return false
}

// Forget drops the plan of a finished run.
func (h *Hook) Forget(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.plans, runID)
}

// RaceJob races one job: it selects distinct eligible runners from w for the
// placement request j, then runs the race through run.CreateAttempt. ctx must
// hold the run Session. ErrNotRaced means: run one attempt instead.
func RaceJob(ctx context.Context, spec run.AttemptSpec, w sched.World, j sched.Job, p Poster) (Evidence, error) {
	ra := run.RaceAttempts{Spec: spec}
	release := !run.Raceable(spec.Job)
	c := Select(w, j, Width(w.Policy.Trust), release)
	return Run(ctx, Spec{Job: j.Key, Choice: c, First: max(spec.N, 1)}, StoreAttempts{RA: ra}, p)
}
