package usage

// Purpose: the daily import on the TEAM coordinator's Reconciler tick (P7-HOST-10, Epic D12/D19). The
// coordinator calls Reconcile every Every(); the pass imports at most once per Interval, remembered
// as a host-provider mark (name "github-usage", key = the account), so a restart never re-imports
// early. A failed pass retries after Backoff (an attempt mark, name "github-usage-attempt"), not on
// every tick.
// Inputs:  the policy source (UsePolicy), the coordinator's store (Use), an injected clock.
// Outputs: ledger rows as Importer writes them; the success mark moves to the pass time only after a
//          success.
// Constraints: dormant unless compat.V15(); with billing_import false a pass makes no API call and
//          writes no mark. The Reconciler never enqueues a trigger. It needs the coordinator start to
//          call UsePolicy and Use once (the same wiring HOST-16 documents for its own Use functions).

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	// Name is the Reconciler name and the provider_mark name.
	Name = "github-usage"
	// Every is how often the coordinator calls the pass; most calls find the mark fresh and return.
	Every = time.Hour
	// Interval is the minimum time between two imports of one account.
	Interval = 24 * time.Hour
	// AttemptName is the provider_mark name of the last attempt, success or failure.
	AttemptName = "github-usage-attempt"
	// Backoff is the minimum time between two attempts after a failed one, so a token without the
	// billing scope (403) or a retired endpoint (410) costs a few calls a day, not one a tick.
	Backoff = 6 * time.Hour
)

// Marks is the host-provider mark store; *store.Store satisfies it.
type Marks interface {
	ProviderMarkGet(ctx context.Context, name, key string) (time.Time, bool, error)
	ProviderMarkPut(ctx context.Context, name, key string, at time.Time) error
}

var _ Marks = (*store.Store)(nil)

// Reconciler is the daily import.
type Reconciler struct {
	Policy func(context.Context) (*policy.Effective, error)
	Store  *store.Store
	Clock  func() time.Time // default time.Now

	// Marks overrides Store for the marks (tests).
	Marks Marks
	// Build overrides FromPolicy (tests).
	Build func(context.Context, *policy.Effective) (*Importer, error)

	mu sync.RWMutex // guards Policy and Store against Use after the coordinator started
}

var (
	_ coordinator.Reconciler = (*Reconciler)(nil)
	_ coordinator.Everyer    = (*Reconciler)(nil)
)

// Name implements coordinator.Reconciler.
func (*Reconciler) Name() string { return Name }

// Every implements coordinator.Everyer.
func (*Reconciler) Every() time.Duration { return Every }

// Reconcile implements coordinator.Reconciler.
func (r *Reconciler) Reconcile(ctx context.Context, _ coordinator.Enqueuer) error {
	if !compat.V15() {
		return nil
	}
	r.mu.RLock()
	pol, st := r.Policy, r.Store
	r.mu.RUnlock()
	if pol == nil {
		return nil // nothing installed yet: the same as no policy, which means import off
	}
	eff, err := pol(ctx)
	if err != nil {
		return err
	}
	set, err := SettingsFrom(eff)
	if err != nil || !set.Import {
		return err
	}
	marks := r.Marks
	if marks == nil && st != nil {
		marks = st
	}
	if marks == nil {
		return errors.New("usage: no marks store installed")
	}
	now := time.Now
	if r.Clock != nil {
		now = r.Clock
	}
	if at, ok, err := marks.ProviderMarkGet(ctx, Name, set.Account); err != nil {
		return err
	} else if ok && now().Sub(at) < Interval {
		return nil
	}
	if at, ok, err := marks.ProviderMarkGet(ctx, AttemptName, set.Account); err != nil {
		return err
	} else if ok && now().Sub(at) < Backoff {
		return nil // the last attempt failed recently
	}
	// The attempt mark is written before the call, so a crash or a hang also counts as an attempt.
	if err := marks.ProviderMarkPut(ctx, AttemptName, set.Account, now()); err != nil {
		return err
	}
	build := r.Build
	if build == nil {
		build = FromPolicy
	}
	imp, err := build(ctx, eff)
	if err != nil {
		return err
	}
	imp.Now = now
	if st != nil {
		ctx = economics.WithEnv(ctx, economics.Env{Store: st, Now: now, Coordinator: Name})
	}
	if _, err := imp.Run(ctx); err != nil {
		return err
	}
	return marks.ProviderMarkPut(ctx, Name, set.Account, now())
}

var def struct {
	sync.Mutex
	r *Reconciler
}

// Default returns the process-wide Reconciler the init() registered.
func Default() *Reconciler {
	def.Lock()
	defer def.Unlock()
	if def.r == nil {
		def.r = &Reconciler{}
	}
	return def.r
}

// UsePolicy installs the source of the operator's effective policy (the coordinator start does this).
func UsePolicy(fn func(context.Context) (*policy.Effective, error)) {
	r := Default()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Policy = fn
}

// Use sets the coordinator's store for the marks and the ledger writes.
func Use(st *store.Store) {
	r := Default()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Store = st
}

func init() {
	if err := coordinator.RegisterReconciler(Default()); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
}
