package economics

// Purpose: what the economics hooks need and the process cannot pass them: the
// store, the effective spend policy, a clock and the coordinator id. The
// dispatcher's step, the run finalizer and the world contributor are called
// with a context and nothing else, so they find an Env here: the one in the
// context, else the one bound for the process.
// Inputs:  Bind(Env) or WithEnv(ctx, Env).
// Outputs: envOf(ctx).
// Constraints: Spend is called from inside the admission transaction, which
// holds the store's one writer, so it must answer from memory (a policy
// resolved before the dispatcher started) and never read the store. An
// unbound process has no env: every hook is then inert and local work is unchanged.

import (
	"context"
	"sync"
	"time"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
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
	boundMu sync.Mutex
)

// Bind installs the process-wide env and returns the function that restores the previous one.
func Bind(e Env) (restore func()) {
	boundMu.Lock()
	defer boundMu.Unlock()
	old, oldOK := bound, boundOK
	bound, boundOK = e, true
	return func() {
		boundMu.Lock()
		bound, boundOK = old, oldOK
		boundMu.Unlock()
	}
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

func init() {
	cirun.RegisterSpend(cirun.SpendWiring{
		Bind: func(st *store.Store, coordinator string, spend func(context.Context) (Policy, error)) func() {
			return Bind(Env{Store: st, Spend: spend, Coordinator: coordinator})
		},
		ApplyPolicy: ApplyPolicy,
		NotePending: NotePending,
	})
}
