package schedtel

// Purpose: the dispatch.DecisionSink. It stores one decision record per attempt
// (the latest word on it: waiting, then admitted, or refused / timed out) in
// the placement-plan document form, so `ci explain <run>/<job>` can read it.
// Inputs:  dispatch.Decision from each tick; the store the dispatcher uses.
// Outputs: a schedtel_decision row; run.NotePlacement for an admitted attempt.
// Constraints: Record never blocks the tick on anything but one store write
// and never fails it: a failure is kept and returned by the finalizer as a run
// note. A decision is written only when it changed since the last one for the
// attempt; the wait is measured from the first decision the sink saw.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Sink is the telemetry DecisionSink of one dispatcher.
type Sink struct {
	st    *store.Store
	clock func() int64 // unix milliseconds

	mu    sync.Mutex
	first map[string]int64  // attempt id -> ms of the first decision seen
	last  map[string]string // attempt id -> signature of the last stored decision
	errs  []error
	more  int // failures past maxErrs, counted not kept
}

// maxErrs bounds the failures kept per run: a broken store fails on every tick.
const maxErrs = 5

// NewSink returns a sink writing to st; clock is unix milliseconds.
func NewSink(st *store.Store, clock func() int64) *Sink {
	return &Sink{st: st, clock: clock, first: map[string]int64{}, last: map[string]string{}}
}

var _ dispatch.DecisionSink = (*Sink)(nil)

// Record stores the decision (see the file comment).
func (s *Sink) Record(ctx context.Context, d dispatch.Decision) {
	id := d.Job.Key
	if id == "" {
		return
	}
	at := s.clock()
	bind(d.Job.Pipeline.ID, s) // before any store call, so the finalizer reports even a first failure
	s.mu.Lock()
	if _, ok := s.first[id]; !ok {
		s.first[id] = at
		if !d.EnqueuedAt.IsZero() { // the real wait starts at enqueue, not at the first tick
			s.first[id] = min(at, d.EnqueuedAt.UnixMilli())
		}
	}
	sig := d.Outcome + "/" + string(d.Reason) + "/" + d.Explanation.Status
	if d.Placement != nil {
		sig += "/" + d.Placement.RunnerID
	}
	if s.last[id] == sig && d.Outcome != dispatch.OutcomeAdmitted {
		s.mu.Unlock()
		return
	}
	waited := at - s.first[id]
	s.mu.Unlock()
	if err := s.store(ctx, d, at, waited); err != nil {
		s.fail(err)
		return
	}
	s.mu.Lock()
	s.last[id] = sig
	if terminal(d.Outcome) {
		delete(s.first, id)
		delete(s.last, id)
	}
	s.mu.Unlock()
}

// terminal outcomes end the attempt's time in the dispatcher's queue.
func terminal(outcome string) bool {
	switch outcome {
	case dispatch.OutcomeAdmitted, dispatch.OutcomeTimeout, dispatch.OutcomeFailed, dispatch.OutcomeUnplaceable:
		return true
	}
	return false
}

func (s *Sink) store(ctx context.Context, d dispatch.Decision, at, waited int64) error {
	id := d.Job.Key
	meta, ok, err := s.st.TelAttempt(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("schedtel: decision for unknown attempt " + id)
	}
	doc, err := json.Marshal(document(meta, d))
	if err != nil {
		return err
	}
	rec := store.TelDecision{ID: id, Run: meta.Run, Job: meta.Job, JobName: meta.JobName, Project: meta.Project, Attempt: meta.N,
		WorldDigest: d.WorldDigest, Outcome: d.Outcome, WaitMs: waited, TS: at, Doc: doc}
	if d.Placement != nil {
		rec.Chosen, rec.CostMicro = d.Placement.RunnerID, d.Placement.CostMicro
		rec.RunnerClass = runnerClassOf(d.World, rec.Chosen)
	}
	if err = s.st.TelPutDecision(ctx, rec); err != nil {
		return err
	}
	if d.Outcome == dispatch.OutcomeAdmitted {
		cirun.NotePlacement(id, RefPrefix+id)
	}
	return nil
}

// document is the placement-plan document holding this decision's job alone.
func document(meta store.TelAttempt, d dispatch.Decision) model.PlacementPlan {
	job := d.Job
	job.Name = meta.JobName
	x := d.Explanation
	if x.JobKey == "" {
		x.JobKey, x.Status = job.Key, "waiting"
		switch d.Outcome {
		case dispatch.OutcomeTimeout, dispatch.OutcomeFailed, dispatch.OutcomeUnplaceable:
			x.Status = "unplaceable"
		}
		if d.Reason != "" {
			x.Reasons = []model.PlacementReasonDetail{{Code: string(d.Reason), Detail: d.Outcome}}
		}
	}
	var w sched.World
	if d.World != nil {
		w = *d.World
	}
	res := sched.Result{Explanations: []sched.Explanation{x}}
	if d.Placement != nil {
		res.Placements = []sched.Placement{*d.Placement}
	}
	run := meta.Run
	return planview.Build(planview.Input{Name: meta.Project, Revision: meta.Revision, Trigger: meta.Trigger, ID: &run,
		PolicyDigest: meta.PolicyDigest, WorldDigest: d.WorldDigest, World: w, Jobs: []sched.Job{job}, Result: res})
}

// runnerClassOf is the class of the runner in the world, else its location.
func runnerClassOf(w *sched.World, id string) string {
	if w == nil {
		return ""
	}
	for _, r := range append(append([]sched.Runner(nil), w.Runners...), w.Hosted...) {
		if r.ID == id {
			return classOf(r)
		}
	}
	return ""
}

func classOf(r sched.Runner) string {
	if r.Class != "" {
		return r.Class
	}
	return r.Location
}

func (s *Sink) fail(err error) {
	s.mu.Lock()
	if len(s.errs) < maxErrs {
		s.errs = append(s.errs, err)
	} else {
		s.more++
	}
	s.mu.Unlock()
}

// takeErr returns and clears the failures since the last call.
func (s *Sink) takeErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := errors.Join(s.errs...)
	if s.more > 0 {
		err = errors.Join(err, fmt.Errorf("schedtel: %d more failures", s.more))
	}
	s.errs, s.more = nil, 0
	return err
}
