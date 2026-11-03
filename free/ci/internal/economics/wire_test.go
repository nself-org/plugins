package economics

// Tests of the dispatcher wiring (D-1280): the pipeline worst case, a bound env with no spend policy
// placing nothing paid, and a noted worst case that ApproveSpend binds to and releases the whole run.
// Fake rates and an in-memory store only; no provider is called.

import (
	"math"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

var hourly = sched.Cost{Class: ClassMetered, RateMicro: 3_600_000, DivMs: msPerHour, BillingIncrementMs: 60_000}

func TestWirePipelineWorstPricesTheDearestPaidRunner(t *testing.T) {
	cheap := sched.Cost{Class: ClassMetered, RateMicro: 1_800_000, DivMs: msPerHour, BillingIncrementMs: 60_000}
	free := sched.Cost{Class: ClassOwned, RateMicro: 99_000_000, DivMs: msPerHour}
	costs := map[string]sched.Cost{"a": cheap, "b": hourly, "c": free}
	if got := PipelineWorst(costs, []int64{300_000, 300_000}); got != 600_000 { // 2 x 5 min at 3.6e6/h
		t.Fatalf("worst %d, want 600000", got)
	}
	if got := PipelineWorst(map[string]sched.Cost{"c": free}, []int64{300_000}); got != 0 {
		t.Fatalf("a world with no paid runner is worth nothing, got %d", got)
	}
	big := sched.Cost{Class: ClassEphemeral, RateMicro: math.MaxInt64, DivMs: 1, BillingIncrementMs: 1}
	if got := PipelineWorst(map[string]sched.Cost{"x": big}, []int64{1 << 40, 1 << 40}); got != math.MaxInt64 {
		t.Fatalf("a saturated pipeline must not wrap, got %d", got)
	}
}

func TestWireBoundWithNoSpendPolicyPlacesNothingPaid(t *testing.T) {
	r := newApprovalRig(t)
	env := WithEnv(r.ctx, Env{Store: r.st, Now: r.clk.now, Coordinator: "me"}) // bound, Spend nil: the defaults
	a := queueAttempt(t, r.st, "pipe", "sha-1", "project")
	if ref := refusalOf(t, place(env, r.st, request(a, ClassMetered, "cloudco", 60_000, 0))); ref.Reason != model.PaidDisabled {
		t.Fatalf("no spend policy must place nothing paid: %+v", ref)
	}
	var p sched.Policy
	ApplyPolicy(&p, Policy{}, "me")
	if p.PaidEnabled || p.ApprovalThresholdMicro != 0 || p.CoordinatorID != "me" {
		t.Fatalf("the default policy enables nothing: %+v", p)
	}
}

func TestWireNotedWorstCaseLetsApproveSpendReleaseTheWholeRun(t *testing.T) {
	r := newApprovalRig(t)
	a := queueAttempt(t, r.st, "pipe", "sha-1", "project")
	b := queueAttempt(t, r.st, "pipe", "sha-1", "project")
	for _, id := range []string{a, b} { // threshold 100 000: each job is held before it reaches the step in a real run
		if ref := refusalOf(t, place(r.env, r.st, request(id, "", "cloudco", 300_000, 0))); ref.Reason != model.ApprovalPending {
			t.Fatalf("job %s: %+v", id, ref)
		}
	}
	worst := PipelineWorst(map[string]sched.Cost{"r": hourly}, []int64{300_000, 300_000})
	if err := NotePending(r.env, "pipe", worst); err != nil {
		t.Fatal(err)
	}
	got, err := ApproveSpend(r.env, "pipe", r.admin)
	if err != nil {
		t.Fatal(err)
	}
	if got.EstimateMicro != worst || worst != 600_000 {
		t.Fatalf("approved %d, worst case %d: the figure ApproveSpend binds to is the pipeline's", got.EstimateMicro, worst)
	}
	for _, id := range []string{a, b} {
		if err = place(r.env, r.st, request(id, "", "cloudco", 300_000, 0)); err != nil {
			t.Fatalf("after approval of the whole run, job %s: %v", id, err)
		}
	}
}
