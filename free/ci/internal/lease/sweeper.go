package lease

// Purpose: the expiry sweeper of Epic D6. A remote lease whose deadline passed without a heartbeat (a node that
//          froze, lost its network or died) is failed through the store transition API, so the attempt goes
//          lost and its retry is scheduled. Nothing else moves such a lease: the carrier of an agent node only
//          waits for the attempt to end.
// Inputs:  the engine (store, clock) and the sweep period.
// Outputs: Sweep runs every period until the context ends; failures are reported to logf and retried on the next
//          period.
// Constraints: one sweeper per coordinator process; the sweep is idempotent (a lease renewed meanwhile is skipped
//          by the store's compare-and-swap), so a second sweeper or a late tick does no harm.

import (
	"context"
	"sync"
	"time"
)

// DefaultSweepEvery is how often the coordinator sweeps for expired leases.
const DefaultSweepEvery = time.Second

// RunSweeper sweeps expired leases every period (DefaultSweepEvery when <= 0) until ctx ends, and returns
// when the last sweep is done.
func (e *Engine) RunSweeper(ctx context.Context, every time.Duration, logf func(format string, a ...any)) {
	if every <= 0 {
		every = DefaultSweepEvery
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := e.Sweep(ctx); err != nil && ctx.Err() == nil && logf != nil {
				logf("lease sweep: %v", err)
			}
		}
	}
}

// StartSweeper runs RunSweeper in the background; the returned stop function ends it and waits.
func (e *Engine) StartSweeper(every time.Duration, logf func(format string, a ...any)) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); e.RunSweeper(ctx, every, logf) }()
	return func() { cancel(); wg.Wait() }
}
