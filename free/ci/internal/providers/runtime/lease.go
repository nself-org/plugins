package runtime

// Purpose: hosted lease profiles (Epic D4, NODE-09). A hosted attempt has no agent heartbeat; the
// lease engine keeps it alive from the provider's own view of the run. RegisterProfiles gives the
// engine, for every registered Dispatcher, a LeaseProfile whose Liveness reads the run through the
// handle book and the provider's Liveness, and whose TTL and Poll come from the Dispatcher's Profile.
// Inputs:  the lease Engine, the handle Book, the registered providers.
// Outputs: one lease profile per Dispatcher, keyed by provider id (the runner's identity.provider).
// Constraints: a queued or running run renews the lease (a run that waits ten minutes for a hosted
//          machine is not re-dispatched); a run the provider no longer knows, or an attempt with no
//          handle, reports vanished, which the engine turns into a lost lease one TTL later and not
//          before. Registering twice for one engine is refused by the engine and returned here.

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// RegisterProfiles registers a lease profile for every registered Dispatcher. It returns the joined
// errors of the providers it could not register; the others are registered.
func RegisterProfiles(e *lease.Engine, book Book) error {
	var errs []error
	for _, p := range providers.All() {
		disp, ok := p.(providers.Dispatcher)
		if !ok {
			continue
		}
		prof := disp.Profile()
		err := e.RegisterProfile(p.ID(), lease.LeaseProfile{Liveness: liveness(p.ID(), disp, book), TTL: prof.TTL, Poll: prof.Poll})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func liveness(provider string, disp providers.Dispatcher, book Book) func(context.Context, string, int64) (lease.LiveState, error) {
	return func(ctx context.Context, attemptID string, epoch int64) (lease.LiveState, error) {
		e, ok, err := book.Get(ctx, attemptID)
		if err != nil {
			return "", err
		}
		if !ok || e.Handle.Provider != provider || e.Epoch != epoch {
			return lease.LiveVanished, nil
		}
		state, err := disp.Liveness(ctx, e.Handle)
		if err != nil {
			return "", err
		}
		switch state {
		case providers.Queued:
			return lease.LiveQueued, nil
		case providers.Running:
			return lease.LiveRunning, nil
		case providers.Finished:
			return lease.LiveFinished, nil
		case providers.Vanished:
			return lease.LiveVanished, nil
		}
		return "", providers.NewError(providers.Config, "E704", "provider reported an unknown run state", 0, time.Time{})
	}
}
