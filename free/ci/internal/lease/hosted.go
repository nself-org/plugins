package lease

import (
	"context"
	"fmt"
	"time"
)

// LiveState is the provider's current view of a hosted attempt.
type LiveState string

const (
	LiveQueued   LiveState = "queued"
	LiveRunning  LiveState = "running"
	LiveFinished LiveState = "finished"
	LiveVanished LiveState = "vanished"
)

// LeaseProfile supplies provider liveness in place of agent heartbeats.
type LeaseProfile struct {
	Liveness func(context.Context, string) (LiveState, error)
	TTL      time.Duration
	Poll     time.Duration
}

// RegisterProfile rejects duplicate or incomplete hosted provider registrations.
func (e *Engine) RegisterProfile(provider string, p LeaseProfile) error {
	if provider == "" || p.Liveness == nil {
		return fmt.Errorf("invalid lease profile")
	}
	if p.TTL <= 0 {
		p.TTL = e.Config.TTL
	}
	if p.Poll <= 0 {
		p.Poll = e.Config.HostedPoll
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.profiles[provider]; exists {
		return fmt.Errorf("lease profile %q already registered", provider)
	}
	e.profiles[provider] = p
	return nil
}

// PollHosted renews positive liveness; vanished attempts expire one provider TTL later.
func (e *Engine) PollHosted(ctx context.Context, provider, leaseID, attemptID string, epoch int64) (LiveState, error) {
	e.mu.Lock()
	p, ok := e.profiles[provider]
	e.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("lease profile %q not registered", provider)
	}
	state, err := p.Liveness(ctx, attemptID)
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	e.polled[leaseID] = e.Config.Clock()
	e.mu.Unlock()
	switch state {
	case LiveQueued, LiveRunning:
		e.mu.Lock()
		delete(e.gone, leaseID)
		e.mu.Unlock()
		return state, e.Renew(ctx, leaseID, epoch, p.TTL)
	case LiveFinished:
		return state, e.Result(ctx, leaseID, epoch)
	case LiveVanished:
		e.mu.Lock()
		_, seen := e.gone[leaseID]
		if !seen {
			e.gone[leaseID] = e.Config.Clock()
		}
		e.mu.Unlock()
		if !seen {
			return state, e.Store.LeaseRenew(ctx, leaseID, epoch, e.Config.Clock(), p.TTL, true)
		}
		return state, nil
	default:
		return "", fmt.Errorf("invalid hosted liveness %q", state)
	}
}

// PollDue runs each hosted liveness check no more often than the profile interval.
func (e *Engine) PollDue(ctx context.Context, provider, leaseID, attemptID string, epoch int64) (bool, LiveState, error) {
	e.mu.Lock()
	p, ok := e.profiles[provider]
	last := e.polled[leaseID]
	due := ok && (last.IsZero() || e.Config.Clock().Sub(last) >= p.Poll)
	e.mu.Unlock()
	if !ok {
		return false, "", fmt.Errorf("lease profile %q not registered", provider)
	}
	if !due {
		return false, "", nil
	}
	state, err := e.PollHosted(ctx, provider, leaseID, attemptID, epoch)
	return true, state, err
}
