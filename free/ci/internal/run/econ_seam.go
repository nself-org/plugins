package run

// Purpose: the seam between the run dispatcher and internal/economics (D-1280).
// economics imports run (its finalizer), so run cannot import economics; economics
// registers these functions from its init and the dispatcher calls them here.
// Inputs:  SetEconomics at init; the per-run dispatcher (dispatch_hook.go).
// Outputs: Bind of the economics env before a dispatcher starts, the spend keys on
// the placement policy, and the pipeline estimate noted for an approval hold.
// Constraints: with no hooks registered every call is inert and nothing paid is
// placed; the spend policy is resolved once, in memory, and never read from the store
// inside an admission transaction; the dispatcher never changes a spend value.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// EconEnv mirrors economics.Env.
type EconEnv struct {
	Store       *store.Store
	Spend       func(context.Context) (schedpolicy.SpendPolicy, error)
	Now         func() time.Time
	Coordinator string
}

// EconHooks is what internal/economics provides to the dispatcher.
type EconHooks struct {
	Bind    func(EconEnv) (restore func())
	Apply   func(p *sched.Policy, sp schedpolicy.SpendPolicy, coordinator string)
	Pending func(ctx context.Context, run string, estimateMicro int64) error
	// Worst is the worst-case cost of a pipeline whose jobs run timeoutsMs each, priced at the dearest paid runner of costs.
	Worst func(costs map[string]sched.Cost, timeoutsMs []int64) int64
}

var econHooks *EconHooks

// SetEconomics registers the economics hooks (called once, from economics' init).
func SetEconomics(h EconHooks) { econHooks = &h }

// spendNow is the spend policy the dispatcher resolved (zero = the defaults: nothing paid).
func (rd *runDispatch) spendNow(context.Context) (schedpolicy.SpendPolicy, error) {
	rd.polM.Lock()
	defer rd.polM.Unlock()
	if rd.spend == nil {
		return schedpolicy.SpendPolicy{}, nil
	}
	return *rd.spend, nil
}

// bindEconomics resolves the policy (best effort: a failure is retried by the first job) and binds the
// economics env before the dispatcher exists, so an unbound process never refuses metered runners by accident.
func (rd *runDispatch) bindEconomics(ctx context.Context) {
	if econHooks == nil || econHooks.Bind == nil {
		return
	}
	_, _ = rd.policy(ctx, "")
	rd.econRestore = econHooks.Bind(EconEnv{Store: rd.sess.Store, Spend: rd.spendNow, Now: rd.sess.now, Coordinator: rd.sess.Coordinator.ID})
}

// applySpend puts the spend keys on the scheduler policy (economics.ApplyPolicy).
func (rd *runDispatch) applySpend(p *sched.Policy) {
	if econHooks != nil && econHooks.Apply != nil && rd.spend != nil {
		econHooks.Apply(p, *rd.spend, rd.sess.Coordinator.ID)
	}
}

// econSink notes the worst case of the whole pipeline for a job held with approval.pending: such a job
// never reaches the budget step, so ApproveSpend would find no figure.
type econSink struct {
	dispatch.DecisionSink
	rd *runDispatch
}

func (s econSink) Record(ctx context.Context, d dispatch.Decision) {
	if d.Outcome == dispatch.OutcomeAwaitingAppr && econHooks != nil && econHooks.Pending != nil && econHooks.Worst != nil && d.World != nil {
		timeouts := []int64{d.Job.TimeoutMs}
		if pl := s.rd.sess.Plan; pl != nil && len(pl.Jobs) > 0 {
			timeouts = timeouts[:0]
			for _, pj := range pl.Jobs {
				timeouts = append(timeouts, jobTimeoutMs(pj.Job.Timeout))
			}
		}
		if est := econHooks.Worst(d.World.Costs, timeouts); est > 0 {
			_ = econHooks.Pending(ctx, d.Job.Pipeline.ID, est)
		}
	}
	s.DecisionSink.Record(ctx, d)
}

// jobTimeoutMs is the timeout planview gives a job: its declared one, else the default.
func jobTimeoutMs(declared string) int64 {
	if d, err := time.ParseDuration(declared); err == nil && d > 0 {
		return d.Milliseconds()
	}
	return planview.DefaultTimeoutMs
}
