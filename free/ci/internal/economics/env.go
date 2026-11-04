package economics

// Purpose: what the economics hooks need and the process cannot pass them: the
// store, the effective spend policy, a clock and the coordinator id. The
// dispatcher's step, the run finalizer and the world contributor are called
// with a context and nothing else, so they find an Env here: the one in the
// context, else the one bound for the process.
// Inputs:  Bind(Env) (a reference-counted process scope) or WithEnv(ctx, Env).
// Outputs: envOf(ctx).
// Constraints: Spend is called from inside the admission transaction, which
// holds the store's one writer, so it must answer from memory (a policy
// resolved before the dispatcher started) and never read the store. An
// unbound process has no env: every hook is then inert and local work is unchanged.

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Policy is the typed spend policy (schedpolicy.SpendPolicy; it cannot live
// here because this package imports schedpolicy).
type Policy = schedpolicy.SpendPolicy

// Env is the economics environment of one coordinator.
type Env struct {
	Store       *store.Store
	Spend       func(context.Context) (Policy, error)
	Now         func() time.Time
	Coordinator string
}

// StaticSpend answers every call with p.
func StaticSpend(p Policy) func(context.Context) (Policy, error) {
	return func(context.Context) (Policy, error) { return p, nil }
}

var (
	bound   Env
	boundOK bool
	boundN  int // runs holding the binding; the last release unbinds
	boundMu sync.Mutex
)

// Bind takes a hold on the process-wide env and returns the function that releases it. The
// binding is a reference-counted scope: the first hold binds, later holds (overlapping runs of
// one coordinator) share it, and only the last release unbinds, in any order, so one run
// finishing never unbinds economics under another. A later hold must carry the same store,
// coordinator and spend policy as the bound one; otherwise it is refused (nothing is held) so
// two runs never price spend under different rules. A release runs once; further calls are no-ops.
func Bind(e Env) (release func(), err error) {
	boundMu.Lock()
	defer boundMu.Unlock()
	if boundOK {
		if err = sameEnv(bound, e); err != nil {
			return func() {}, err
		}
	} else {
		bound, boundOK = e, true
	}
	boundN++
	var once sync.Once
	return func() {
		once.Do(func() {
			boundMu.Lock()
			defer boundMu.Unlock()
			if boundN--; boundN <= 0 {
				bound, boundOK, boundN = Env{}, false, 0
			}
		})
	}, nil
}

// sameEnv is nil when b would price spend exactly as a does (same store, coordinator and
// resolved spend policy). It reads the policies from memory and never carries their values into the error.
func sameEnv(a, b Env) error {
	if a.Store != b.Store || a.Coordinator != b.Coordinator {
		return fmt.Errorf("%s: economics is already bound for another store or coordinator; overlapping runs must share one", CodeSpendRefused)
	}
	pa, ea := a.policy(context.Background())
	pb, eb := b.policy(context.Background())
	if ea != nil || eb != nil || !reflect.DeepEqual(pa, pb) {
		return fmt.Errorf("%s: economics is already bound with a different spend policy; overlapping runs must share one", CodeSpendRefused)
	}
	return nil
}

type envKey struct{}

// WithEnv returns a context carrying e, which wins over the bound env.
func WithEnv(ctx context.Context, e Env) context.Context { return context.WithValue(ctx, envKey{}, e) }

func envOf(ctx context.Context) (Env, bool) {
	var e Env
	if v, ok := ctx.Value(envKey{}).(Env); ok {
		e = v
	} else {
		boundMu.Lock()
		e = bound
		ok = boundOK
		boundMu.Unlock()
		if !ok {
			return Env{}, false
		}
	}
	if e.Now == nil {
		e.Now = time.Now
	}
	return e, e.Store != nil
}

func (e Env) policy(ctx context.Context) (Policy, error) {
	if e.Spend == nil {
		return Policy{}, nil // no spend policy bound: the defaults, which place nothing paid
	}
	return e.Spend(ctx)
}

// ApplyPolicy copies the spend keys the scheduler reads from the policy into
// the scheduler's own policy (sched.Policy is built by schedpolicy.Placement,
// which does not know the spend keys; the caller that builds the dispatcher's
// world policy applies them here, with this coordinator's id).
func ApplyPolicy(sp *sched.Policy, p Policy, coordinator string) {
	sp.PaidEnabled, sp.MaxPaidTimeoutMs, sp.CoordinatorID = p.PaidEnabled, p.MaxPaidTimeoutMs, coordinator
	sp.ApprovalThresholdMicro = 0
	if p.ApprovalThresholdMicro.Set {
		sp.ApprovalThresholdMicro = p.ApprovalThresholdMicro.Micro
	}
}
