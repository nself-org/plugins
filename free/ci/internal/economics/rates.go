package economics

// Purpose: cost classes (Epic D11): which class a runner is priced as, and the
// sched.Cost the scheduler sees for it.
// Inputs:  the typed spend policy and a runner's class and provider.
// Outputs: classOf, CostFor.
// Constraints: GitHub-hosted capacity is `included` and nothing else: its
// overage is never a placement option and no GitHub class is ever metered
// (owner decision, APC P8); a runner with no declared rate and no class of its
// own is `owned` (free to run); rates are the owner's declaration, shown with
// confidence `estimated`.

import (
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

// The cost classes.
const (
	ClassLocal     = "local"
	ClassOwned     = "owned"
	ClassIncluded  = "included"
	ClassMetered   = "metered"
	ClassEphemeral = "ephemeral"
)

// anyPaid lets schedpolicy.PaidAllowed answer "is this a GitHub id": it is false for GitHub whatever the policy says.
var anyPaid = schedpolicy.SpendPolicy{PaidEnabled: true, Budgets: schedpolicy.Budgets{Day: schedpolicy.Budget{Micro: 1, Set: true}}}

// isGitHub reports whether provider names GitHub-hosted capacity (schedpolicy owns the rule).
func isGitHub(provider string) bool { return !schedpolicy.PaidAllowed(anyPaid, provider) }

func isPaid(class string) bool { return class == ClassMetered || class == ClassEphemeral }

// classOf is the class a runner is priced as. The owner's rate table beats the
// runner's own claim: a provider with a declared paid rate is paid whatever class
// its capability says, so a runner cannot call itself free.
func classOf(p Policy, runnerClass, provider string) string {
	if isGitHub(provider) {
		return ClassIncluded
	}
	declared := ""
	if r, ok := p.Rates[provider]; ok {
		declared = r.Class
		if declared == "" {
			declared = ClassMetered
		}
		if isPaid(declared) {
			return declared
		}
	}
	switch runnerClass {
	case ClassLocal, ClassOwned, ClassIncluded, ClassMetered, ClassEphemeral:
		return runnerClass
	}
	if declared != "" {
		return declared
	}
	return ClassOwned
}

// CostFor is the sched.Cost of a runner. The scheduler prices with the rate per
// DivMs milliseconds, so an hourly or per-minute rate keeps its precision.
func CostFor(p Policy, runnerClass, provider string) sched.Cost {
	class := classOf(p, runnerClass, provider)
	c := sched.Cost{Class: class, Confidence: "computed", OwnerCoordinator: p.OwnerCoordinator[provider]}
	if class == ClassLocal || class == ClassOwned {
		return c
	}
	c.Confidence = "estimated"
	if rs, ok := SnapOf(p.Rates[provider]); ok {
		c.RateMicro, c.DivMs, c.BillingIncrementMs, c.MinimumChargeMicro, c.ExtrasMicro = rs.RateMicro, rs.RateUnitMs, rs.IncMs, rs.MinMicro, rs.ExtrasMicro
	}
	return c
}
