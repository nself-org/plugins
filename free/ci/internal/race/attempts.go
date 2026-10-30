package race

// Purpose: Attempts backed by the run store: siblings are made by
// run.CreateAttempt (through run.RaceAttempts), stopped by the epoch-fenced
// lease cancel intent and awaited by reading their state.
// Inputs:  a context holding the run Session.
// Outputs: Sibling values and their final Result.
// Constraints: Wait only reads; it returns when the attempt is terminal or
// the context is done (then the result is a cancel). The poll clock is
// injected so tests never sleep.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/run"
)

// DefaultPoll is how often Wait reads a sibling's state.
const DefaultPoll = 100 * time.Millisecond

// StoreAttempts implements Attempts on the run store.
type StoreAttempts struct {
	RA run.RaceAttempts
	// Tick returns the channel Wait reads between polls (nil: a DefaultPoll ticker).
	Tick func() (<-chan time.Time, func())
}

// Create implements Attempts.
func (s StoreAttempts) Create(ctx context.Context, n int, runner string) (Sibling, error) {
	rs, err := s.RA.Create(ctx, n, runner)
	if err != nil {
		return Sibling{}, err
	}
	return Sibling{N: rs.N, AttemptID: rs.AttemptID, LeaseID: rs.LeaseID, RunnerID: rs.RunnerID, Epoch: rs.Epoch}, nil
}

// Cancel implements Attempts.
func (s StoreAttempts) Cancel(ctx context.Context, sib Sibling, reason string) error {
	return s.RA.Cancel(ctx, sib.AttemptID, sib.Epoch, reason)
}

// Wait implements Attempts.
func (s StoreAttempts) Wait(ctx context.Context, sib Sibling) Result {
	tick, stop := s.ticker()
	defer stop()
	for {
		state, class, err := s.RA.State(context.WithoutCancel(ctx), sib.AttemptID)
		if err == nil {
			switch state {
			case StatePassed, StateFailed, StateCancelled, "lost", "needs-operator":
				return Result{State: state, FailureClass: class}
			}
		}
		select {
		case <-ctx.Done():
			return Result{State: StateCancelled, FailureClass: StateCancelled}
		case <-tick:
		}
	}
}

func (s StoreAttempts) ticker() (<-chan time.Time, func()) {
	if s.Tick != nil {
		return s.Tick()
	}
	t := time.NewTicker(DefaultPoll)
	return t.C, t.Stop
}
