package burst

// Purpose: the registrations that connect the engine to the rest of the module
// (Epic D12): the `ephemeral` admission step, the `burst` world contributor, the
// dispatcher start hook and the revoke hook that marks an intent in the revoke
// transaction.
// Inputs:  Bind(engine) at coordinator start.
// Outputs: a restore function.
// Constraints: nothing here runs without a bound engine (an unbound process is
// unchanged: the step admits, the contributor adds nothing, the start hook
// returns); the step needs no engine (it reads the admission view); registration
// happens in init and panics only on a programming error.

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

var bound atomic.Pointer[Engine]

// Bind installs the process-wide engine and returns the function that restores
// the previous one and stops this engine's loop.
func Bind(e *Engine) (restore func()) {
	old := bound.Swap(e)
	return func() {
		if cur := bound.Swap(old); cur != nil && cur != old {
			cur.Close()
		}
	}
}

func current() *Engine { return bound.Load() }

func init() {
	if err := dispatch.RegisterStep(StepName, Step); err != nil {
		panic(err)
	}
	if err := world.Register(world.Contributor{Name: "burst", Field: "Pending", Fn: contributePending}); err != nil {
		panic(err)
	}
	dispatch.OnStart(onStart)
	dispatch.OnWaiting(onWaiting)
	lifecycle.RegisterRevoker(store.BurstRevokeHook())
}

// startTimeout bounds the recovery a dispatcher start runs.
const startTimeout = 2 * time.Minute

func onStart(*dispatch.Dispatcher) error {
	e := current()
	if e == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	_ = e.Start(ctx) // a failed teardown is alerted and retried by the loop; the dispatcher still starts
	e.StartLoop()
	return nil
}

// contributePending adds the capacity on its way: one slot per live intent whose node has not joined.
func contributePending(ctx context.Context, w *sched.World) error {
	e := current()
	if e == nil {
		return nil
	}
	rows, err := e.o.Store.BurstPending(ctx)
	if err != nil {
		return nil // an unreadable table adds no capacity: the scheduler waits or uses what it has
	}
	if len(rows) == 0 {
		return nil
	}
	pend := make([]sched.Pending, 0, len(rows))
	for _, r := range rows {
		pend = append(pend, sched.Pending{Provider: r[0], Platform: r[1], Slots: 1})
	}
	w.Pending = pend
	return nil
}

// Bound returns the bound engine, or nil when none is.
func Bound() *Engine { return current() }

// ProviderIDs lists the engine's provider ids in order.
func (e *Engine) ProviderIDs() []string { return e.providerIDs() }

// Coordinator is the coordinator id this engine labels its machines with.
func (e *Engine) Coordinator() string { return e.o.Coordinator }
