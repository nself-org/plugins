package schedtel

// Purpose: the P7-CI-37 Finalizer. When a run closes it stores the outcome of
// every attempt the dispatcher placed (actual wait, runtime, cost, result,
// failure class) and folds finished ones into the EWMA estimators.
// Inputs:  run.RunInfo; the run's sink (and so its store) from bind.
// Outputs: schedtel_outcome and schedtel_estimator rows; an error naming any
// store failure of the run (the coordinator turns it into a run note).
// Constraints: only a job that ran to a verdict (passed or failed with no
// infrastructure failure) updates the runtime estimate; cold start is the time
// from admission to the attempt's leased -> running transition (started_at). An
// attempt with no recorded start falls back to admission to end minus the job's
// own runtime (which also covers teardown); one with no end is skipped.

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type finalizer struct{}

func (finalizer) Name() string { return "schedtel" }

func (finalizer) Finalize(ctx context.Context, info cirun.RunInfo) error {
	s := sinkFor(info.RunID)
	for _, j := range info.Jobs {
		cirun.ForgetPlacement(j.AttemptID)
	}
	if s == nil {
		return nil
	}
	defer unbind(info.RunID)
	errs := []error{s.takeErr()}
	for _, j := range info.Jobs {
		if j.AttemptID != "" {
			errs = append(errs, s.outcome(ctx, j))
		}
	}
	return errors.Join(errs...)
}

// outcome stores one attempt's outcome; an attempt with no admitted decision was never placed.
func (s *Sink) outcome(ctx context.Context, j cirun.JobOutcome) error {
	dec, ok, err := s.st.TelDecision(ctx, j.AttemptID)
	if err != nil || !ok || dec.Outcome != dispatch.OutcomeAdmitted {
		return err
	}
	at := s.clock()
	runtime := j.Duration.Milliseconds()
	o := store.TelOutcome{AttemptID: j.AttemptID, Project: dec.Project, JobKey: dec.JobName, Runner: dec.Chosen, RunnerClass: dec.RunnerClass,
		Result: j.State, FailureClass: string(j.FailureClass), WaitMs: dec.WaitMs, RuntimeMs: runtime, ColdStartMs: -1, CostMicro: dec.CostMicro, TS: at}
	ran := j.State == cirun.StatePassed || (j.State == cirun.StateFailed && (j.FailureClass == "" || j.FailureClass == model.FailureClass("code")))
	if !ran {
		runtime = -1
	}
	cold := int64(-1)
	if ran {
		var err error
		if cold, err = s.coldStart(ctx, j.AttemptID, dec.TS, runtime); err != nil {
			return err
		}
		o.ColdStartMs = cold
	}
	return s.st.TelRecordOutcome(ctx, o, fold(runtime, cold, at))
}

// coldStart is the milliseconds from admission (admitMs) to the attempt starting
// to run, or -1 when neither a start nor an end of the attempt was recorded.
func (s *Sink) coldStart(ctx context.Context, attempt string, admitMs, runtime int64) (int64, error) {
	if start, started, err := s.st.TelAttemptStart(ctx, attempt); err != nil {
		return -1, err
	} else if started {
		return max(start/1_000_000-admitMs, 0), nil
	}
	end, ended, err := s.st.TelAttemptEnd(ctx, attempt)
	if err != nil || !ended {
		return -1, err
	}
	return max(end/1_000_000-admitMs-runtime, 0), nil
}
