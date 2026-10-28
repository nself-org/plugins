package agent

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// The delivery queue is deliberately volatile; the store owns lease lifetime.
// Exercise queued, running, and disconnected-cancel leases in one restart.
func TestRestartWithQueuedRunningAndPendingCancel(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	old := &Driver{CancelGrace: time.Millisecond}
	leases := make([]protocol.Lease, 3)
	for i, n := range []string{"2", "3", "1"} {
		id, att, ep := r.seedLease(n, "n-1")
		leases[i] = protoLease(id, att, ep)
		leases[i].Token = r.mint("restart-"+n, "n-1", att, ep)
		rt := &route{node: "n-1", leaseID: id, done: make(chan struct{}), sink: &runSink{}}
		if err := old.enqueue(rt, leases[i]); err != nil {
			t.Fatal(err)
		}
	}
	// Deliver l2 and l3, leaving l1 queued.
	for i := 0; i < 2; i++ {
		if l, err := old.Next(ctx, "n-1"); err != nil || l == nil {
			t.Fatalf("delivery: %v %v", l, err)
		}
	}
	cancelled := old.routes["l3"]
	if err := old.cancelRun(cancelled); err == nil {
		t.Fatal("cancel without a control stream succeeded")
	}
	if len(old.pending["n-1"]) != 1 {
		t.Fatal("cancel was not pending")
	}
	r.reopen()
	fresh := &Driver{Gate: r.gate(), Minter: r.tokens, Leases: r.st}
	if l, err := fresh.Next(ctx, "n-1"); err != nil || l != nil {
		t.Fatalf("volatile queue survived: %v %v", l, err)
	}
	// Running and cancelled agents report their held leases on reconnect. With
	// no adopted route both are refused as lost (the API sends a lease cancel).
	for _, l := range leases[:2] {
		c := call(l.LeaseID, l.Token, l.Epoch)
		c.Type, c.Reconcile = "heartbeat", true
		if err := fresh.Admit(ctx, c); ReasonOf(err) != ReasonLeaseLost {
			t.Fatalf("orphan %s: %v", l.LeaseID, err)
		}
	}
	// The dispatcher can explicitly re-adopt the undelivered lease: enqueue the
	// same durable fence, never grant a second lease for the attempt.
	rt := &route{node: "n-1", leaseID: leases[2].LeaseID, done: make(chan struct{}), sink: &runSink{}}
	if err := fresh.enqueue(rt, leases[2]); err != nil {
		t.Fatal(err)
	}
	if l, err := fresh.Next(ctx, "n-1"); err != nil || l == nil || l.Epoch != leases[2].Epoch {
		t.Fatalf("re-adopted lease: %v %v", l, err)
	}
	// Anything nobody re-adopts is bounded by its durable TTL, not the lost queue.
	r.eng.Config.Clock = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if n, err := r.eng.Sweep(ctx); err != nil || n != 3 {
		t.Fatalf("TTL sweep: %d %v", n, err)
	}
}
