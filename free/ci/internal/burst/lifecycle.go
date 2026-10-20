package burst

// Purpose: the moving parts of the lifecycle around Provision and Teardown
// (Epic D12): a node joining or failing to join, the triggers that end an
// intent (job done, idle, join timeout, lease expiry), recovery on dispatcher
// start, and the five-minute loop.
// Inputs:  node events from enrollment, the clock, the store.
// Outputs: state moves and teardowns.
// Constraints: one job per node (the `ephemeral` step enforces it; Tick ends
// the intent once the job's attempt is no longer active); a start tears down
// every open intent of this coordinator first, because a restarted dispatcher
// has lost its jobs; a retry of a failed teardown happens at every Tick; the
// loop is idempotent to start and stops with Close.

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// NodeJoined binds an enrolled node to its live intent, starts its idle clock and closes the breaker.
func (e *Engine) NodeJoined(ctx context.Context, token, nodeID string) error {
	err := e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error {
		in, ok, err := t.Intent(token)
		if err != nil || !ok || in.Coordinator != e.o.Coordinator || in.State != store.BurstLive {
			return refused("intent.unavailable", err, "intent %s cannot take a node", token)
		}
		now := t.NowMs()
		if bound, err := t.SetNode(token, nodeID, now); err != nil || !bound {
			return refused("node.bind", err, "intent %s already has a node", token)
		}
		if err = t.SetIdle(token, now+e.cfg.IdleTimeout.Milliseconds()); err != nil {
			return err
		}
		return t.BreakerReset(in.Provider)
	})
	if err != nil {
		return err
	}
	return e.at(PointAfterJoin)
}

// JoinFailed counts a join failure against the provider's breaker and tears the intent down.
func (e *Engine) JoinFailed(ctx context.Context, token string) error {
	err := e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error {
		in, ok, err := t.Intent(token)
		if err != nil || !ok || in.Coordinator != e.o.Coordinator {
			return refused("intent.unavailable", err, "intent %s is not this coordinator's", token)
		}
		_, err = t.BreakerFail(in.Provider, BreakerThreshold, BreakerOpenMs, t.NowMs())
		return err
	})
	if err != nil {
		return err
	}
	return e.Teardown(ctx, token, "join failed")
}

// Tick ends the intents whose time has come, retries failed teardowns and sweeps.
func (e *Engine) Tick(ctx context.Context) (SweepReport, error) {
	intents, err := e.o.Store.BurstIntents(ctx, e.o.Coordinator)
	if err != nil {
		return SweepReport{}, err
	}
	var errs []error
	for _, in := range intents {
		reason, err := e.due(ctx, in)
		if err != nil {
			if errors.Is(err, ErrKilled) {
				return SweepReport{}, err
			}
			errs = append(errs, err)
			continue
		}
		if reason == "" {
			continue
		}
		if err = e.teardown(ctx, in.Token, reason, true); err != nil {
			if errors.Is(err, ErrKilled) {
				return SweepReport{}, err
			}
			errs = append(errs, err)
		}
	}
	rep, err := e.Sweep(ctx, false)
	return rep, errors.Join(append(errs, err)...)
}

// due says why an intent must be torn down now ("" when it must not).
func (e *Engine) due(ctx context.Context, in store.BurstIntent) (string, error) {
	if e.isCreating(in.Token) {
		return "", nil // the create call has not returned: no timer applies yet
	}
	_, failed, err := e.o.Store.TeardownFailure(ctx, in.Token)
	if err != nil {
		return "", err
	}
	if failed || in.State == store.BurstTearingDown {
		return "retry", nil
	}
	lease, ok, err := e.o.Store.BurstLease(ctx, in.Token)
	if err != nil {
		return "", err
	}
	now := e.now().UnixMilli()
	switch {
	case ok && now >= lease.MaxUntil:
		return "lease expired", nil
	case in.NodeID == "":
		if ok && now >= lease.IdleUntil {
			return ReasonJoinTimeout, nil
		}
		return "", nil
	}
	used, err := e.o.Store.BurstNodeUsed(ctx, in.NodeID)
	if err != nil {
		return "", err
	}
	if !used {
		if ok && now >= lease.IdleUntil {
			return ReasonIdle, nil
		}
		return "", nil
	}
	busy, err := e.o.Store.BurstNodeBusy(ctx, in.NodeID)
	switch {
	case err != nil:
		return "", err
	case busy:
		return "", e.at(PointMidJob)
	}
	return "job done", nil
}

// Start is the recovery a dispatcher start runs: every open intent of this
// coordinator is torn down (its jobs died with the old process), then one sweep.
func (e *Engine) Start(ctx context.Context) error {
	intents, err := e.o.Store.BurstIntents(ctx, e.o.Coordinator)
	if err != nil {
		return err
	}
	var errs []error
	for _, in := range intents {
		if err = e.Teardown(ctx, in.Token, "dispatcher start"); err != nil {
			if errors.Is(err, ErrKilled) {
				return err
			}
			errs = append(errs, err)
		}
	}
	_, err = e.Sweep(ctx, false)
	return errors.Join(append(errs, err)...)
}

type loopState struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// StartLoop runs Tick every SweepEvery until Close. A second call is a no-op.
func (e *Engine) StartLoop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loop != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := &loopState{cancel: cancel, done: make(chan struct{})}
	e.loop = l
	go func() {
		defer close(l.done)
		t := time.NewTicker(e.cfg.SweepEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, _ = e.Tick(ctx) // failures are alerted and retried at the next tick
			}
		}
	}()
}

// Close stops the loop and waits for it.
func (e *Engine) Close() {
	e.mu.Lock()
	l := e.loop
	e.loop = nil
	e.mu.Unlock()
	if l != nil {
		l.cancel()
		<-l.done
	}
}
