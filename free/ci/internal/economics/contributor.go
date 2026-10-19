package economics

// Purpose: the `economics` world contributors (Epic D11, P7-SCHED-18's
// world.Register). Each contributor owns exactly one sched.World field, so the
// economics set registers four: Costs (`economics`), Allowances
// (`economics-allowances`), Budgets (`economics-budgets`) and Approvals
// (`economics-approvals`). Together they give the scheduler the cost class and
// declared rate of every runner (with the provider's owner coordinator), the
// allowance of each included provider, the budgets still open and the runs whose
// spend is approved.
// Inputs:  the world with its runners built, the Env, and w.NowMs as the clock.
// Outputs: Costs[runner id], Allowances[provider], Budgets, Approvals[run].
// Constraints: read only; no Env means no change (local work is unaffected); a
// policy or store failure leaves the field empty, which the scheduler reads as
// no rate, no budget and an unknown allowance, so paid and included capacity
// become ineligible and the failure never opens money (the admission step
// still re-checks everything); Budgets.Job and Budgets.Pipeline carry the
// declared limits (the step counts the pipeline exactly); Day, Month and the
// provider budgets carry the limit minus what is reserved in the current window.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func init() {
	for _, c := range []world.Contributor{
		{Name: "economics", Field: "Costs", Fn: contributeCosts},
		{Name: "economics-allowances", Field: "Allowances", Fn: contributeAllowances},
		{Name: "economics-budgets", Field: "Budgets", Fn: contributeBudgets},
		{Name: "economics-approvals", Field: "Approvals", Fn: contributeApprovals},
	} {
		if err := world.Register(c); err != nil {
			panic(err)
		}
	}
}

func allRunners(w *sched.World) []sched.Runner {
	return append(append([]sched.Runner(nil), w.Runners...), w.Hosted...)
}

// load returns the env and policy, or ok false when economics is unbound or unreadable.
func load(ctx context.Context) (Env, Policy, bool) {
	env, ok := envOf(ctx)
	if !ok {
		return Env{}, Policy{}, false
	}
	p, err := env.policy(ctx)
	if err != nil {
		return Env{}, Policy{}, false
	}
	return env, p, true
}

func contributeCosts(ctx context.Context, w *sched.World) error {
	env, p, ok := load(ctx)
	if !ok {
		return nil
	}
	owners, err := env.Store.ProviderOwners(ctx)
	if err != nil {
		return nil
	}
	costs := map[string]sched.Cost{}
	for _, r := range allRunners(w) {
		c := CostFor(p, r.Class, r.Provider)
		if isPaid(c.Class) && c.OwnerCoordinator == "" {
			c.OwnerCoordinator = owners[r.Provider]
		}
		costs[r.ID] = c
	}
	if len(costs) > 0 {
		w.Costs = costs
	}
	return nil
}

func contributeAllowances(ctx context.Context, w *sched.World) error {
	env, p, ok := load(ctx)
	if !ok {
		return nil
	}
	high, err := env.Store.WindowHigh(ctx, dayOf(time.UnixMilli(w.NowMs)))
	if err != nil {
		return nil
	}
	_, month := peekWindows(high, time.UnixMilli(w.NowMs))
	out := map[string]sched.Allowance{}
	for _, r := range allRunners(w) {
		if _, done := out[r.Provider]; done || classOf(p, r.Class, r.Provider) != ClassIncluded {
			continue
		}
		in, err := env.Store.Allowance(ctx, r.Provider, month)
		if err != nil {
			return nil
		}
		out[r.Provider] = AllowanceOf(p, r.Provider, in).ToSched(p.MarginMicro)
	}
	if len(out) > 0 {
		w.Allowances = out
	}
	return nil
}

func contributeBudgets(ctx context.Context, w *sched.World) error {
	env, p, ok := load(ctx)
	if !ok {
		return nil
	}
	high, err := env.Store.WindowHigh(ctx, dayOf(time.UnixMilli(w.NowMs)))
	if err != nil {
		return nil
	}
	day, month := peekWindows(high, time.UnixMilli(w.NowMs))
	open := func(b schedpolicy.Budget, scope, key, window string) (sched.Budget, bool) {
		if !b.Set {
			return sched.Budget{}, true
		}
		held, err := env.Store.Reserved(ctx, scope, key, window)
		if err != nil {
			return sched.Budget{}, false
		}
		rem, _ := sched.Add(b.Micro, -held)
		return sched.Budget{Set: true, Remaining: max(rem, 0)}, true
	}
	var b sched.Budgets
	b.Job = sched.Budget{Set: p.Budgets.Job.Set, Remaining: p.Budgets.Job.Micro}
	b.Pipeline = sched.Budget{Set: p.Budgets.Pipeline.Set, Remaining: p.Budgets.Pipeline.Micro}
	if b.Day, ok = open(p.Budgets.Day, "day", "", day); !ok {
		return nil
	}
	if b.Month, ok = open(p.Budgets.Month, "month", "", month); !ok {
		return nil
	}
	for id, lim := range p.Budgets.Provider {
		pb, ok := open(schedpolicy.Budget{Micro: lim, Set: true}, "provider", id, month)
		if !ok {
			return nil
		}
		if b.Providers == nil {
			b.Providers = map[string]sched.Budget{}
		}
		b.Providers[id] = pb
	}
	w.Budgets = b
	return nil
}

func contributeApprovals(ctx context.Context, w *sched.World) error {
	env, ok := envOf(ctx)
	if !ok {
		return nil
	}
	valid, err := env.Store.ValidApprovals(ctx)
	if err != nil || len(valid) == 0 {
		return nil
	}
	out := make(map[string]bool, len(valid))
	for run := range valid {
		out[run] = true
	}
	w.Approvals = out
	return nil
}
