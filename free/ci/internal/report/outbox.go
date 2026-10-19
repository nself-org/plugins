package report

// Purpose: the status outbox (CI D13). A run reserves a seq when it starts,
// stores its status when it ends, and delivers it only if no newer seq was
// posted for the same (revision, context). Unposted entries survive a
// coordinator restart and are drained by the next invocation.
// Inputs:  a store and a Reporter. Outputs: delivered statuses.
// Constraints: the payload is the Status JSON (no secrets); delivery failures
// leave the entry pending, they never fail the run.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Outbox delivers statuses in seq order per (revision, context).
type Outbox struct {
	Store       *store.Store
	Reporter    Reporter
	Owner, Repo string        // the repository Reserve works for; Complete and Drain read it from the status
	Pause       time.Duration // poll interval while an older post is in flight (tests)
}

// key is the store key of one repository's statuses: two clones that share a
// commit (a fork and its upstream) must not order or supersede each other.
func key(owner, repo string) string { return Context + "@" + owner + "/" + repo }

// Reserve takes the seq a run will use for its final status.
func (o *Outbox) Reserve(ctx context.Context, revision string) (int64, error) {
	return o.Store.OutboxReserve(ctx, revision, key(o.Owner, o.Repo))
}

// Complete stores the status for seq and tries to deliver it. delivered is
// false when the entry stayed pending or was superseded.
func (o *Outbox) Complete(ctx context.Context, seq int64, s Status) (delivered bool, err error) {
	if err = s.Validate(); err != nil {
		return false, err
	}
	body, err := json.Marshal(s)
	if err != nil {
		return false, err
	}
	if err = o.Store.OutboxReady(ctx, s.Revision, key(s.Owner, s.Repo), seq, body); err != nil {
		return false, err
	}
	return o.deliver(ctx, seq, s, true)
}

func (o *Outbox) deliver(ctx context.Context, seq int64, s Status, heal bool) (bool, error) {
	if o.Reporter == nil {
		return false, nil
	}
	k := key(s.Owner, s.Repo)
	for waited := 0; waited < 240; waited++ { // an older post in flight: wait up to ~2 minutes
		res, token, err := o.Store.OutboxClaim(ctx, s.Revision, k, seq)
		if err != nil {
			return false, err
		}
		switch res {
		case store.ClaimOK:
			bg := context.WithoutCancel(ctx)
			if err = o.Reporter.Post(ctx, s); err != nil {
				// the request may have landed (a lost response): a release that finds a newer status requeues it
				if rerr := o.Store.OutboxRelease(bg, s.Revision, k, seq, token); heal && store.IsClaimLost(rerr) {
					_, _ = o.Drain(ctx)
				}
				return false, err
			}
			err = o.Store.OutboxPosted(bg, s.Revision, k, seq, token)
			if heal && store.IsClaimLost(err) { // suspended past the claim TTL: a newer status may have been overwritten
				_, _ = o.Drain(ctx)
				return true, nil
			}
			return true, err
		case store.ClaimWait:
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(o.pause()):
			}
		default:
			return false, nil
		}
	}
	return false, nil // still waiting: the entry stays pending for the next invocation
}

func (o *Outbox) pause() time.Duration {
	if o.Pause > 0 {
		return o.Pause
	}
	return 500 * time.Millisecond
}

// healRounds bounds how often Drain re-reads the outbox after a delivery lost
// its claim (each round needs another poster suspended past the claim TTL).
const healRounds = 5

// Drain delivers every pending entry, oldest first, and returns how many
// were delivered. A delivery error does not stop the drain: the failed entry
// stays pending, entries of the same repository behind it are skipped (they
// must not overtake it) and entries of other repositories are still
// delivered. The first error is returned. A cancelled context stops the drain.
// An entry whose claim was lost while it was delivered makes the drain read the outbox again,
// because the store queued the newest status for another delivery.
func (o *Outbox) Drain(ctx context.Context) (int, error) {
	if o.Reporter == nil {
		return 0, nil
	}
	n := 0
	var first error
	for round := 0; round < healRounds; round++ {
		list, err := o.Store.OutboxUnposted(ctx)
		if err != nil {
			return n, err
		}
		again := false
		failed := map[string]bool{} // repositories whose oldest entry failed this round
		for _, e := range list {
			var s Status
			if err = json.Unmarshal(e.Payload, &s); err != nil || s.Validate() != nil {
				continue // a corrupt entry is left for the operator; it must not block the rest
			}
			k := key(s.Owner, s.Repo)
			if failed[k] {
				continue
			}
			done, err := o.deliver(ctx, e.Seq, s, false)
			if done {
				n++
			}
			if store.IsClaimLost(err) {
				again = true
				continue
			}
			if err != nil {
				if first == nil {
					first = err
				}
				if ctx.Err() != nil {
					return n, first
				}
				failed[k] = true
			}
		}
		if !again {
			break
		}
	}
	return n, first
}
