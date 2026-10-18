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

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Outbox delivers statuses in seq order per (revision, context).
type Outbox struct {
	Store    *store.Store
	Reporter Reporter
}

// Reserve takes the seq a run will use for its final status.
func (o *Outbox) Reserve(ctx context.Context, revision string) (int64, error) {
	return o.Store.OutboxReserve(ctx, revision, Context)
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
	if err = o.Store.OutboxReady(ctx, s.Revision, Context, seq, body); err != nil {
		return false, err
	}
	return o.deliver(ctx, seq, s)
}

func (o *Outbox) deliver(ctx context.Context, seq int64, s Status) (bool, error) {
	if o.Reporter == nil {
		return false, nil
	}
	ok, err := o.Store.OutboxClaim(ctx, s.Revision, Context, seq)
	if err != nil || !ok {
		return false, err
	}
	if err = o.Reporter.Post(ctx, s); err != nil {
		_ = o.Store.OutboxRelease(context.WithoutCancel(ctx), s.Revision, Context, seq)
		return false, err
	}
	return true, o.Store.OutboxPosted(ctx, s.Revision, Context, seq)
}

// Drain delivers every pending entry, oldest first, and returns how many
// were delivered. The first delivery error stops the drain.
func (o *Outbox) Drain(ctx context.Context) (int, error) {
	if o.Reporter == nil {
		return 0, nil
	}
	list, err := o.Store.OutboxUnposted(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range list {
		var s Status
		if err = json.Unmarshal(e.Payload, &s); err != nil || s.Validate() != nil {
			continue // a corrupt entry is left for the operator; it must not block the rest
		}
		done, err := o.deliver(ctx, e.Seq, s)
		if err != nil {
			return n, err
		}
		if done {
			n++
		}
	}
	return n, nil
}
