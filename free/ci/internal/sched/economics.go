package sched

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func economics(w World, j Job, r Runner) []model.PlacementReasonDetail {
	var out []model.PlacementReasonDetail
	add := func(code model.PlacementReason, detail string) { out = append(out, reason(code, detail)) }
	c := w.Costs[r.ID]
	// The class the cost contributor priced the runner at wins over the class the runner
	// claims for itself: a rated provider cannot be made free by its own capability.
	class := runnerClass(w, r)
	paid := class != "owned" && class != "local" && class != "included"
	cost, overflow := worstCost(j.TimeoutMs, c)
	if class == "owned" || class == "local" {
		cost, overflow = 0, false
	}
	if paid {
		if c.RateMicro <= 0 {
			add(model.CostUnknownRate, "missing or nonpositive rate")
		}
		max := w.Policy.MaxPaidTimeoutMs
		if max <= 0 {
			max = 7200000
		}
		if j.TimeoutMs <= 0 || j.TimeoutMs > max {
			add(model.CostUnbounded, "timeout")
		}
		if !w.Policy.PaidEnabled {
			add(model.PaidDisabled, "operator policy")
		}
		p := w.Budgets.Providers[r.Provider]
		if !w.Budgets.Day.Set && !w.Budgets.Month.Set && !p.Set {
			add(model.BudgetUnset, "day, month or provider budget required")
		}
		if c.OwnerCoordinator != "" && c.OwnerCoordinator != w.Policy.CoordinatorID {
			add(model.ProviderOwnedElsewhere, "provider owner")
		}
		if class == "ephemeral" && r.CoordinatorMode != trust.Team {
			add(model.BurstTeamOnly, "team coordinator required")
		}
		for _, b := range []Budget{w.Budgets.Job, w.Budgets.Pipeline, w.Budgets.Day, w.Budgets.Month, p} {
			if b.Set && (overflow || cost > b.Remaining) {
				add(model.BudgetExceeded, "budget")
			}
		}
		if overflow {
			add(model.BudgetExceeded, "saturated cost")
		}
		if w.Policy.ApprovalThresholdMicro > 0 && cost > w.Policy.ApprovalThresholdMicro && !w.Approvals[j.Pipeline.ID] {
			add(model.ApprovalPending, "spend approval")
		}
	}
	if class == "included" && !labelMatches(r.Capability.Identity.Labels, "nself.visibility=public") {
		a, ok := w.Allowances[r.Provider]
		if !ok || !a.Known || !a.CeilingKnown {
			add(model.AllowanceUnknown, "consumption or ceiling unknown")
		}
		remaining, marginOverflow := int64(0), a.MarginMicro < 0
		if !marginOverflow {
			remaining, marginOverflow = Add(a.RemainingEstimated, -a.MarginMicro)
		}
		if ok && a.Known && a.CeilingKnown && (overflow || marginOverflow || remaining <= cost) {
			add(model.AllowanceBelowMargin, "allowance margin")
		}
	}
	if w.Policy.Overrides.MaxCostMicro > 0 && cost > w.Policy.Overrides.MaxCostMicro {
		add(model.OverrideMaxCost, "maximum cost")
	}
	return out
}

func worstCost(timeout int64, c Cost) (int64, bool) {
	if c.RateMicro <= 0 {
		return int64(^uint64(0) >> 1), true
	}
	if timeout < 0 {
		timeout = 0
	}
	inc := c.BillingIncrementMs
	if inc <= 0 {
		inc = 60000
	}
	units := timeout / inc
	if timeout%inc != 0 {
		units++
	}
	billed, of := Mul(units, inc)
	charge, of2 := MulDivUp(billed, c.RateMicro, max(c.DivMs, 1))
	charge, of3 := Add(charge, c.MinimumChargeMicro)
	charge, of4 := Add(charge, c.ExtrasMicro)
	return charge, of || of2 || of3 || of4
}
