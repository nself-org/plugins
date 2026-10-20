package burst_test

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func waitingJob(attempt string) dispatch.Waiting {
	return dispatch.Waiting{
		Job: dispatch.Job{AttemptID: attempt, Sched: sched.Job{Key: "k-" + attempt, TimeoutMs: 600_000,
			Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 2, MemMB: 4096}}},
		Pipeline: "pipe-1", Project: "proj", ExpectedWaitMs: 600_000, Demand: 1,
	}
}

// TestWaitingJobProvisionsOneMachine: the dispatcher's waiting job becomes one intent-first provision, a
// second ask for the same job inside AskEvery makes neither a machine nor another decision row, and the
// next job gets its own machine (D-1329).
func TestWaitingJobProvisionsOneMachine(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.eng.Waiting(ctx, waitingJob("att-1"))
	if n := len(r.labelled(coord)); n != 1 {
		t.Fatalf("a waiting job should provision one machine, found %d", n)
	}
	r.eng.Waiting(ctx, waitingJob("att-1"))
	if n := len(r.labelled(coord)); n != 1 {
		t.Fatalf("a second ask for the same job inside AskEvery made another machine: %d", n)
	}
	r.clk.Advance(burst.AskEvery + 1)
	r.eng.Waiting(ctx, waitingJob("att-1"))
	if n := len(r.labelled(coord)); n != 1 {
		t.Fatalf("the per-job window must stop a second machine for the same job: %d", n)
	}
	second := waitingJob("att-2") // one machine is already on its way for a demand of 1, so a second job is demand 2
	second.Demand = 2
	r.eng.Waiting(ctx, second)
	if n := len(r.labelled(coord)); n != 2 {
		t.Fatalf("the next waiting job should get its own machine: %d", n)
	}
}

// TestWaitingJobRefusedLeavesNoMachine: a provider disabled in the operator policy is refused and a
// refused ask provisions nothing.
func TestWaitingJobRefusedLeavesNoMachine(t *testing.T) {
	r := newRig(t, func(r *rig) { r.pol.Burst.Enabled = map[string]bool{} })
	r.eng.Waiting(context.Background(), waitingJob("att-1"))
	if n := len(r.labelled(coord)); n != 0 {
		t.Fatalf("a disabled provider made %d machines", n)
	}
}

// TestJoinTimeoutOpensTheBreaker: a machine that never joins is torn down at its join deadline and counts
// as a join failure, so three in a row stop a provider that provisions paid machines nobody can use
// (the dispatcher now asks on every waiting job, D-1329).
func TestJoinTimeoutOpensTheBreaker(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for _, job := range []string{"j1", "j2", "j3"} {
		r.provision(job)
		r.clk.Advance(20 * time.Minute) // past the join deadline: the idle timeout plus the provider's latency
		if _, err := r.eng.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.cloud.All()) != 0 {
		t.Fatal("a machine that never joined was left running")
	}
	if _, err := r.eng.Provision(ctx, r.request("j4")); burst.ReasonOf(err) != burst.ReasonBreakerOpen {
		t.Fatalf("three join timeouts in a row must open the breaker: %v", err)
	}
}
