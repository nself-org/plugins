package burst

// Purpose: Teardown, the guarantee that a machine does not outlive its intent
// (Epic D12): revoke the node, mark the intent tearing-down, delete the machine,
// verify by List that it is gone, and only then end the intent and release its
// reservation. A failure at any step alerts once, blocks new provisioning on
// that provider, and is retried at every sweep until the machine is gone.
// Inputs:  an intent token and the reason for the teardown.
// Outputs: nil when the intent is terminal, else an E697 (the retry continues).
// Constraints: revoke and the tearing-down mark commit in one transaction (the
// store's revoke hook marks the intent); a revoke that fails never keeps the
// machine alive (the machine is deleted, the revoke is retried, the intent stays
// open); only machines labelled role=ci-ephemeral with this coordinator's id and
// this intent are deleted, and never one in the exclusion set; money is released
// only after List no longer returns the machine; texts carry classes, not provider errors.

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Teardown ends one intent; see the file comment.
func (e *Engine) Teardown(ctx context.Context, token, reason string) error {
	return e.teardown(ctx, token, reason, false)
}

// Idle and join-timeout teardowns are decided from a read that admission can overtake: they give way to a lease.
func onlyIfUnused(reason string) bool { return reason == ReasonIdle || reason == ReasonJoinTimeout }

// lockIntent serializes the teardowns of one intent (Tick, Sweep, JoinFailed and a failed Provision may meet).
func (e *Engine) lockIntent(token string) func() {
	e.mu.Lock()
	m := e.locks[token]
	if m == nil {
		m = &sync.Mutex{}
		e.locks[token] = m
	}
	e.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func (e *Engine) teardown(ctx context.Context, token, reason string, ifUnused bool) error {
	unlock := e.lockIntent(token)
	defer unlock()
	if e.isCreating(token) {
		// the create call is still in flight in this process: ending the intent now would release money for a machine
		// that is about to appear. The caller's next sweep retries.
		return teardownFailed(nil, "intent %s is still being created; retried at the next sweep", token)
	}
	in, ok, err := e.o.Store.BurstIntent(ctx, token)
	if err != nil {
		return teardownFailed(err, "intent %s unreadable", token)
	}
	if !ok || in.Coordinator != e.o.Coordinator {
		return teardownFailed(nil, "intent %s is not this coordinator's", token)
	}
	if in.State == store.BurstTerminal {
		return nil
	}
	p, ok := e.providers[in.Provider]
	if !ok {
		return e.failed(ctx, in, errors.New("provider not configured"), "provider "+in.Provider+" is not configured")
	}
	revokeErr := e.markAndRevoke(ctx, in, reason, ifUnused && onlyIfUnused(reason))
	if errors.Is(revokeErr, errNowInUse) {
		return nil // a lease won the race: the node is working, the intent stays live
	}
	if errors.Is(revokeErr, ErrKilled) {
		return revokeErr
	}
	if err = e.at(PointMidTeardown); err != nil {
		return err
	}
	gone, err := e.deleteResources(ctx, p, in)
	switch {
	case err != nil:
		return e.failed(ctx, in, err, errClass(err))
	case !gone:
		return e.failed(ctx, in, errStillListed, errClass(errStillListed))
	case revokeErr != nil:
		return e.failed(ctx, in, revokeErr, "node revoke failed")
	}
	if err = e.finalize(ctx, in, reason); err != nil {
		return err
	}
	e.mu.Lock()
	delete(e.locks, token)
	e.mu.Unlock()
	return nil
}

// markAndRevoke ends the node's life. A bound node is revoked first (its hook marks the intent in the
// same transaction) and the intent is marked regardless. An unbound intent is marked first, which stops
// a node from binding to it, and is read again: a node that bound between the read and the mark is revoked too.
func (e *Engine) markAndRevoke(ctx context.Context, in store.BurstIntent, reason string, ifUnused bool) error {
	mark := func() error {
		return e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error { return t.MarkTearingDown(in.Token, reason, t.NowMs()) })
	}
	revoke := func(node string) error {
		if e.o.Revoker == nil {
			return errors.New("no node revoker configured")
		}
		return e.o.Revoker.Revoke(ctx, node)
	}
	if err := e.at(PointBeforeMark); err != nil {
		return err
	}
	if ifUnused {
		var marked bool
		err := e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error {
			var err error
			marked, err = t.MarkTearingDownIfUnused(in.Token, reason, t.NowMs())
			return err
		})
		if err != nil {
			return err
		}
		if !marked {
			if now, ok, rerr := e.o.Store.BurstIntent(ctx, in.Token); rerr == nil && ok && now.State == store.BurstLive {
				return errNowInUse
			}
		}
		if in.NodeID != "" {
			return revoke(in.NodeID)
		}
		return e.revokeLateJoiner(ctx, in.Token, revoke)
	}
	if in.NodeID != "" {
		return errors.Join(revoke(in.NodeID), mark())
	}
	if err := mark(); err != nil {
		return err
	}
	return e.revokeLateJoiner(ctx, in.Token, revoke)
}

// revokeLateJoiner revokes a node that bound to the intent between the read and the mark.
func (e *Engine) revokeLateJoiner(ctx context.Context, token string, revoke func(string) error) error {
	again, ok, err := e.o.Store.BurstIntent(ctx, token)
	if err != nil || !ok || again.NodeID == "" {
		return err
	}
	return revoke(again.NodeID)
}

var errNowInUse = errors.New("burst: the node took its job first")

// own keeps the machines that carry this coordinator's labels and this intent.
func (e *Engine) own(rs []Resource, token string) []Resource {
	var out []Resource
	for _, r := range rs {
		if Mine(r.Labels, e.o.Coordinator) && r.Labels[LabelIntent] == token {
			out = append(out, r)
		}
	}
	return out
}

// deleteResources deletes the intent's machines and reports whether List no longer returns any.
func (e *Engine) deleteResources(ctx context.Context, p Provider, in store.BurstIntent) (bool, error) {
	ex, err := e.o.Exclusion(ctx)
	if err != nil {
		return false, fmt.Errorf("exclusion set: %w", err) // fail closed: nothing is deleted without it
	}
	found, err := p.List(ctx, map[string]string{LabelIntent: in.Token})
	if err != nil {
		return false, err
	}
	mine := e.own(found, in.Token)
	ids := map[string]bool{}
	for _, r := range mine {
		if excluded(ex, r) {
			return false, errExcluded
		}
		ids[r.ID] = true
	}
	for _, id := range sortedKeys(ids) {
		if err = p.Teardown(ctx, id); err != nil && !errors.Is(err, ErrNotFound) {
			return false, err
		}
	}
	after, err := p.List(ctx, map[string]string{LabelIntent: in.Token})
	if err != nil {
		return false, err
	}
	return len(e.own(after, in.Token)) == 0, nil
}

// failed records a failed teardown, alerts once (event + audit) and returns the E697.
func (e *Engine) failed(ctx context.Context, in store.BurstIntent, cause error, class string) error {
	var first bool
	werr := e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error {
		var err error
		if first, err = t.TeardownFailed(in.Token, in.Provider, t.NowMs()); err != nil || !first {
			return err
		}
		return t.AppendAudit("burst", AlertTeardownFailed, in.Token, fmt.Sprintf("provider=%s resource=%s reason=%s", in.Provider, in.ResourceID, class))
	})
	if first && werr == nil && e.o.Alert != nil {
		e.o.Alert(Alert{Kind: AlertTeardownFailed, Provider: in.Provider, Intent: in.Token, Resource: in.ResourceID, Detail: class})
	}
	return teardownFailed(errors.Join(cause, werr), "intent %s on %s: %s; retried at every sweep", in.Token, in.Provider, class)
}

// finalize ends the intent and releases its reservation in one transaction.
func (e *Engine) finalize(ctx context.Context, in store.BurstIntent, reason string) error {
	ectx := e.econCtx(ctx)
	err := e.o.Store.LedgerWrite(ectx, func(t store.LedgerTx) error {
		b := t.Burst()
		if err := b.MarkTerminal(in.Token, b.NowMs()); err != nil {
			return err
		}
		if row, held, err := t.Resource(in.Token); err != nil {
			return err
		} else if held && row.ConfirmedTeardownAt == nil {
			if err = economics.ReleaseResource(ectx, t, in.Token, e.now()); err != nil {
				return err
			}
		}
		return b.AppendAudit("burst", "burst.teardown", in.Token, fmt.Sprintf("provider=%s resource=%s reason=%s", in.Provider, in.ResourceID, reason))
	})
	if err != nil {
		return teardownFailed(err, "intent %s is gone but could not be ended", in.Token)
	}
	return nil
}
