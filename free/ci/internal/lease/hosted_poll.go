package lease

// Purpose: the hosted poll loop of the coordinator (D-1297). Every tick it lists the live hosted leases and runs
//          PollDue on each, so a hosted lease is renewed, expired or finished from the provider's view of the
//          run at its profile's Poll interval (PollDue skips a lease polled more recently than that).
// Inputs:  the engine (profiles registered with RegisterProfile) and a HostedPollConfig.
// Outputs: renewals and transitions through PollHosted; failures go to Logf, each poll to Observe.
// Constraints: one loop per coordinator process, started after the profiles are registered; a failed list or
//          poll is logged and retried on the next tick, never fatal; a lease of a provider with no profile is
//          skipped; the loop stops when its context ends and forgets the poll marks of leases that left the
//          live set (and their renewal marks), so the engine's bookkeeping does not grow with every hosted
//          attempt ever run.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// DefaultHostedTick is how often the hosted poll loop looks for due leases.
const DefaultHostedTick = time.Second

// HostedPollConfig configures RunHostedPoller.
type HostedPollConfig struct {
	// Every is the tick (DefaultHostedTick when <= 0); each lease is still polled at its profile's Poll.
	Every time.Duration
	// List returns the live hosted leases (the store's HostedLeasesActive when nil).
	List func(context.Context) ([]store.HostedLease, error)
	// Logf receives list and poll failures (nil: dropped).
	Logf func(format string, a ...any)
	// Observe, when set, sees every poll that ran (tests and diagnostics).
	Observe func(l store.HostedLease, state LiveState, err error)
}

// RunHostedPoller polls the live hosted leases every tick until ctx ends.
func (e *Engine) RunHostedPoller(ctx context.Context, cfg HostedPollConfig) {
	if cfg.Every <= 0 {
		cfg.Every = DefaultHostedTick
	}
	if cfg.List == nil {
		cfg.List = e.Store.HostedLeasesActive
	}
	t := time.NewTicker(cfg.Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.pollHostedOnce(ctx, cfg)
		}
	}
}

// StartHostedPoller runs RunHostedPoller in the background; the returned stop function ends it and waits.
func (e *Engine) StartHostedPoller(cfg HostedPollConfig) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); e.RunHostedPoller(ctx, cfg) }()
	return func() { cancel(); wg.Wait() }
}

func (e *Engine) pollHostedOnce(ctx context.Context, cfg HostedPollConfig) {
	logf := func(format string, a ...any) {
		if cfg.Logf != nil && ctx.Err() == nil {
			cfg.Logf(format, a...)
		}
	}
	leases, err := cfg.List(ctx)
	if err != nil {
		logf("hosted lease list: %v", err)
		return
	}
	live := make(map[string]bool, len(leases))
	for _, l := range leases {
		live[l.LeaseID] = true
		if !e.hasProfile(l.Provider) {
			continue
		}
		polled, state, err := e.PollDue(ctx, l.Provider, l.LeaseID, l.AttemptID, l.Epoch)
		if err != nil {
			logf("hosted poll of lease %s (%s): %v", l.LeaseID, l.Provider, err)
		}
		if (polled || err != nil) && cfg.Observe != nil {
			cfg.Observe(l, state, err)
		}
	}
	e.forgetHosted(live)
}

func (e *Engine) hasProfile(provider string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.profiles[provider]
	return ok
}

// forgetHosted drops the poll and vanish marks of leases that are no longer live.
func (e *Engine) forgetHosted(live map[string]bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id := range e.polled {
		if !live[id] {
			delete(e.polled, id)
			delete(e.last, id) // a hosted lease's renewal mark; an agent lease is never in polled
		}
	}
	for id := range e.gone {
		if !live[id] {
			delete(e.gone, id)
		}
	}
}
