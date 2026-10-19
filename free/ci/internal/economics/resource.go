package economics

// Purpose: money held for an ephemeral resource, a burst VM, for its whole life
// (Epic D12, review F7). P7-SCHED-08 calls ReserveResource at intent creation
// and ReleaseResource only on a confirmed teardown, each inside its own
// transaction, so the reservation commits or rolls back with the intent.
// Inputs:  a store.LedgerTx of the caller's transaction and the resource.
// Outputs: reservation rows in every set budget scope and a resource_ledger row.
// Constraints: the VM is reserved at its full lifetime bound (it may outlive its
// attempt) and stays in the budgets until teardown is confirmed; a failed
// teardown never releases money; a second ReleaseResource is a no-op; the
// actual is the elapsed time priced with the admitted rate and never exceeds
// the reservation; no background goroutine moves money.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Resource is an ephemeral resource to reserve for: Intent is its unique id,
// LifetimeMs the longest it may live (max_lifetime plus grace).
type Resource struct {
	Intent, Provider, Pipeline string
	LifetimeMs                 int64
}

// ReserveResource holds the lifetime bound of r against every budget in t's transaction.
func ReserveResource(ctx context.Context, t store.LedgerTx, r Resource) error {
	if r.Intent == "" || r.Provider == "" {
		return refuse(model.CostUnbounded, "a resource needs an intent and a provider")
	}
	env, ok := envOf(ctx)
	if !ok {
		return refuse(model.PaidDisabled, "economics is not bound, so nothing can be reserved")
	}
	if _, held, err := t.Resource(r.Intent); err != nil || held {
		return err // a retried provision reserves once
	}
	p, err := env.policy(ctx)
	if err != nil {
		return err
	}
	if err = paidGate(t, env, p, r.Provider); err != nil {
		return err
	}
	q := QuoteJob(p.Rates[r.Provider], r.LifetimeMs, schedpolicy.MaxPaidTimeoutCap)
	if q.Reason != "" {
		return refuse(q.Reason, "%s: lifetime %d ms", r.Provider, r.LifetimeMs)
	}
	day, month, err := windows(t, env.Now())
	if err != nil {
		return err
	}
	if _, err = reserve(t, p, spend{Owner: r.Intent, Pipeline: r.Pipeline, Provider: r.Provider, Micro: q.Micro}, day, month); err != nil {
		return err
	}
	return t.PutResource(store.ResourceRow{Intent: r.Intent, Provider: r.Provider, Pipeline: r.Pipeline, WindowDay: day, ReservedMicro: q.Micro, Rate: q.Snap, ProvisionedAt: t.Now()})
}

// ReleaseResource settles the intent's reservations after a confirmed teardown at `at`.
func ReleaseResource(ctx context.Context, t store.LedgerTx, intent string, at time.Time) error {
	row, ok, err := t.Resource(intent)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s: no reservation for resource %q", CodeSpendRefused, intent)
	}
	if row.ConfirmedTeardownAt != nil {
		return nil
	}
	actual := row.ReservedMicro
	if elapsed := (at.UnixNano() - row.ProvisionedAt) / int64(time.Millisecond); elapsed > 0 {
		if micro, _, overflow := Charge(row.Rate, elapsed); !overflow && micro < actual {
			actual = micro
		}
	}
	return t.ReleaseResource(intent, at.UnixNano(), actual)
}
