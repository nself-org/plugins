package dispatch

// D-1280 regression: a job held with approval.pending never reaches the budget step, so the
// dispatcher reports its estimate (Config.PendingSpend) for ApproveSpend to bind to.

import (
	"context"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func TestAwaitingApprovalReportsThePipelineEstimate(t *testing.T) {
	st := openStore(t)
	cloud := testRunner("cloud", "lan", 4)
	cloud.Class = "metered"
	live := liveWorld(st, func(p *sched.Policy) { p.PaidEnabled, p.ApprovalThresholdMicro = true, 1 }, cloud)
	world := func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
		w, d, err := live(ctx, p)
		w.Costs[cloud.ID] = sched.Cost{RateMicro: 5}
		w.Budgets.Day = sched.Budget{Remaining: 1 << 40, Set: true}
		return w, d, err
	}
	type call struct {
		run string
		est int64
	}
	var mu sync.Mutex
	var calls []call
	d := newDispatcher(t, Config{Store: st, World: world, Sink: &recSink{}, PendingSpend: func(_ context.Context, run string, est int64) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, call{run, est})
		return nil
	}})
	a, b := queueAttempt(t, st, "pipe", "proj", "a"), queueAttempt(t, st, "pipe", "proj", "b")
	for _, id := range []string{a, b} {
		if _, err := d.enqueue(testQueued(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	const one = 60000 * 5 // a 60 s job at 5 micro per millisecond, one billing increment
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 || calls[0].run != "pipe" || calls[0].est != 2*one {
		t.Fatalf("pending spend calls = %+v, want one call for pipe with both jobs' worst case (%d)", calls, 2*one)
	}
}
