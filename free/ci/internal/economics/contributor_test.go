package economics

// Tests of the world contributors (Epic D11): costs with the owner
// coordinator, allowances, open budgets and approvals as the scheduler reads them.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func TestEconomicsContributorsFillTheWorldOnlyWhenBound(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: at("2026-10-19T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(map[string]string{
		schedpolicy.KeyBudgetJob: `700000`, schedpolicy.KeyBudgetPipeline: `800000`, "economics.budgets.provider.cloudco": `900000`,
		"economics.rates.github-hosted": `{"class":"included","rate_per_minute_micro":100}`, "economics.ceilings.github-hosted": `1000000`, schedpolicy.KeyMargin: `7`,
		"economics.providers.cloudco": `{"owner_coordinator":"coord-1"}`,
	}))
	c := WithEnv(ctx, envFor(st, p, clk, "coord-1"))
	runners := []sched.Runner{{ID: "r-cloud", Class: "", Provider: "cloudco"}, {ID: "r-gh", Provider: "github-hosted"}, {ID: "r-local", Class: ClassLocal, Provider: "operator"}}
	w := sched.World{NowMs: clk.now().UnixMilli(), Runners: runners}

	bare := w
	for _, f := range []func(context.Context, *sched.World) error{contributeCosts, contributeAllowances, contributeBudgets, contributeApprovals} {
		if err := f(ctx, &bare); err != nil {
			t.Fatal(err)
		}
	}
	if bare.Costs != nil || bare.Allowances != nil || bare.Approvals != nil || bare.Budgets.Day.Set {
		t.Fatalf("an unbound process changed the world: %+v", bare)
	}

	// 120 000 micro is held in the day, month and provider scopes.
	a := queueAttempt(t, st, "pipe", "rev", "project")
	if err := place(c, st, request(a, "", "cloudco", 120_000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := RecordUsage(c, Usage{Key: "u", Provider: "github-hosted", Date: "2026-10-18", Minutes: 3, CostMicro: 400_000}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []func(context.Context, *sched.World) error{contributeCosts, contributeAllowances, contributeBudgets, contributeApprovals} {
		if err := f(c, &w); err != nil {
			t.Fatal(err)
		}
	}
	cloud := w.Costs["r-cloud"]
	if cloud.Class != ClassMetered || cloud.RateMicro != 3_600_000 || cloud.DivMs != msPerHour || cloud.OwnerCoordinator != "coord-1" {
		t.Fatalf("cloud cost: %+v", cloud)
	}
	if gh := w.Costs["r-gh"]; gh.Class != ClassIncluded || gh.RateMicro != 100 || gh.DivMs != msPerMinute {
		t.Fatalf("github cost: %+v", gh)
	}
	if lc := w.Costs["r-local"]; lc.Class != ClassLocal || lc.RateMicro != 0 {
		t.Fatalf("local cost: %+v", lc)
	}
	al, ok := w.Allowances["github-hosted"]
	if !ok || !al.Known || !al.CeilingKnown || al.RemainingEstimated != 600_000 || al.MarginMicro != 7 {
		t.Fatalf("allowance: %+v", w.Allowances)
	}
	if _, ok = w.Allowances["cloudco"]; ok {
		t.Fatal("a paid provider got an allowance")
	}
	b := w.Budgets
	if !b.Job.Set || b.Job.Remaining != 700_000 || !b.Pipeline.Set || b.Pipeline.Remaining != 800_000 ||
		!b.Day.Set || b.Day.Remaining != 1_000_000-120_000 || !b.Month.Set || b.Month.Remaining != 5_000_000-120_000 || b.Providers["cloudco"].Remaining != 900_000-120_000 {
		t.Fatalf("budgets: %+v", b)
	}
}

func TestEconomicsContributorsAreRegisteredOncePerField(t *testing.T) {
	for _, c := range []world.Contributor{{Name: "x", Field: "Costs", Fn: contributeCosts}, {Name: "y", Field: "Allowances", Fn: contributeAllowances},
		{Name: "z", Field: "Budgets", Fn: contributeBudgets}, {Name: "q", Field: "Approvals", Fn: contributeApprovals}} {
		if err := world.Register(c); err == nil {
			t.Errorf("field %s has no owner after init: a second registration was accepted", c.Field)
		}
	}
}

func TestEconomicsApplyPolicyCopiesTheSpendKeysTheSchedulerReads(t *testing.T) {
	p := spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyApproval: `5000`}))
	var sp sched.Policy
	ApplyPolicy(&sp, p, "coord-1")
	if !sp.PaidEnabled || sp.MaxPaidTimeoutMs != schedpolicy.DefaultMaxPaidTimeoutMs || sp.ApprovalThresholdMicro != 5000 || sp.CoordinatorID != "coord-1" {
		t.Fatalf("%+v", sp)
	}
	ApplyPolicy(&sp, spendPolicy(t, map[string]string{}), "coord-1")
	if sp.PaidEnabled || sp.ApprovalThresholdMicro != 0 {
		t.Fatalf("defaults: %+v", sp)
	}
}
