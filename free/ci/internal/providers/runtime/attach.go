package runtime

// Purpose: the handle bookkeeping of Driver.Run (Epic D4): attach to the run this lease already
// started or dispatch a new one, record the handle so it survives a lease cancel, and clean up after
// a failed Run. See driver.go for the rules.
// Inputs:  the Book, the Dispatcher, the node and the lease.
// Outputs: the providers.Handle to poll; cancelled and forgotten runs.
// Constraints: cancels and book writes run on a context a lease cancel cannot cut (bounded by
//          cancelTimeout); errors from a best-effort cancel are dropped, the lease expiry is the net.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// attach returns the handle to poll: the run this lease already started, or a new one.
func attach(ctx context.Context, book Book, disp providers.Dispatcher, provider string, node model.Capability, l protocol.Lease, facts FactsSource) (providers.Handle, error) {
	epoch := int64(l.Epoch)
	if e, found, err := book.Get(ctx, l.AttemptID); err != nil {
		return providers.Handle{}, err
	} else if found {
		switch {
		case e.Handle.Provider != provider:
			return providers.Handle{}, refused("E708", "attempt was dispatched to another provider")
		case e.Epoch == epoch:
			return e.Handle, nil
		case e.Epoch > epoch:
			return providers.Handle{}, refused("E708", "lease superseded by a newer epoch")
		}
		cancelRun(ctx, disp, e.Handle) // the run of a lost predecessor: stop it before the retry starts its own
		_ = book.Delete(context.WithoutCancel(ctx), l.AttemptID, e.Handle.RunID)
	}
	attempt, err := buildAttempt(ctx, node, l, facts)
	if err != nil {
		return providers.Handle{}, err
	}
	h, err := disp.Dispatch(ctx, attempt)
	if err != nil {
		return providers.Handle{}, err
	}
	// The run exists now: record it on a context a lease cancel cannot cut, or stop the run.
	pc, stop := context.WithTimeout(context.WithoutCancel(ctx), cancelTimeout)
	defer stop()
	won, err := book.Put(pc, l.AttemptID, Entry{Handle: h, Epoch: epoch})
	if err != nil {
		cancelRun(ctx, disp, h)
		return providers.Handle{}, err
	}
	if won.Handle.RunID != h.RunID { // another carrier of this lease dispatched first: keep its run, stop ours
		cancelRun(ctx, disp, h)
		return won.Handle, nil
	}
	return h, nil
}

// forget drops the handle of a failed Run and, when the run may still be alive, cancels it.
func forget(ctx context.Context, book Book, disp providers.Dispatcher, attemptID string, h providers.Handle, cancel bool) {
	if cancel {
		cancelRun(ctx, disp, h)
	}
	c, stop := context.WithTimeout(context.WithoutCancel(ctx), cancelTimeout)
	defer stop()
	_ = book.Delete(c, attemptID, h.RunID)
}

// cancelRun asks the provider to stop a run; the lease context may already be done, so it gets its own.
func cancelRun(ctx context.Context, disp providers.Dispatcher, h providers.Handle) {
	c, stop := context.WithTimeout(context.WithoutCancel(ctx), cancelTimeout)
	defer stop()
	_ = disp.Cancel(c, h)
}
