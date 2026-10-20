package run

// Purpose: binds the economics hooks (budget step, finalizer, contributors, ApproveSpend) for the
// time a run dispatcher lives (D-1280). Without a bound env they are inert and no paid placement happens.
// Inputs:  the store, the coordinator id and the spend policy a dispatcher resolved.
// Outputs: the economics binding, held for as long as any dispatcher of this process is live.
// Constraints: internal/economics imports this package (its finalizer), so it registers its entry
// points here (RegisterSpend) instead of being imported. Bind is process-wide while dispatchers are
// per run, so the binding is counted: the first dispatcher binds, the last one to close restores.
// The spend answer is the policy of the dispatcher that bound last, read from memory (the
// admission transaction holds the store's writer, so it must never read the store).

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// SpendWiring is what the run dispatcher needs from internal/economics.
type SpendWiring struct {
	// Bind installs the process-wide env and returns the function that restores the previous one.
	Bind func(st *store.Store, coordinator string, spend func(context.Context) (schedpolicy.SpendPolicy, error)) (restore func())
	// ApplyPolicy copies the spend keys into the scheduler's policy.
	ApplyPolicy func(sp *sched.Policy, p schedpolicy.SpendPolicy, coordinator string)
	// NotePending records the estimate of a run held for spend approval.
	NotePending func(ctx context.Context, run string, estimateMicro int64) error
}

var (
	econMu      sync.Mutex
	econWiring  SpendWiring
	econLive    int
	econRestore func()
	econSpend   func() schedpolicy.SpendPolicy
)

// RegisterSpend installs the economics entry points; internal/economics calls it from init.
func RegisterSpend(w SpendWiring) {
	econMu.Lock()
	econWiring = w
	econMu.Unlock()
}

func wiring() SpendWiring {
	econMu.Lock()
	defer econMu.Unlock()
	return econWiring
}

// spendPolicy is the spend policy resolved with the run's scheduling policy.
func (rd *runDispatch) spendPolicy() schedpolicy.SpendPolicy {
	rd.polM.Lock()
	defer rd.polM.Unlock()
	return rd.spend
}

func applySpend(sp *sched.Policy, p schedpolicy.SpendPolicy, coordinator string) {
	if f := wiring().ApplyPolicy; f != nil {
		f(sp, p, coordinator)
	}
}

func notePending(ctx context.Context, run string, estimateMicro int64) error {
	if f := wiring().NotePending; f != nil {
		return f(ctx, run, estimateMicro)
	}
	return nil
}

// econAcquire counts one more live dispatcher and binds economics on the first.
func econAcquire(st *store.Store, coordinator string, spend func() schedpolicy.SpendPolicy) {
	econMu.Lock()
	defer econMu.Unlock()
	econLive++
	econSpend = spend
	if econLive > 1 || econWiring.Bind == nil {
		return
	}
	econRestore = econWiring.Bind(st, coordinator, func(context.Context) (schedpolicy.SpendPolicy, error) {
		econMu.Lock()
		f := econSpend
		econMu.Unlock()
		return f(), nil
	})
}

// econRelease counts one dispatcher out and restores the previous binding on the last.
func econRelease() {
	econMu.Lock()
	defer econMu.Unlock()
	if econLive == 0 {
		return
	}
	if econLive--; econLive == 0 && econRestore != nil {
		econRestore()
		econRestore = nil
	}
}

// SpendRegistered reports whether internal/economics has installed its entry points.
func SpendRegistered() bool {
	w := wiring()
	return w.Bind != nil && w.ApplyPolicy != nil && w.NotePending != nil
}
