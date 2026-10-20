package lease

import (
	"fmt"

	"context"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"testing"
	"time"
)

// A sweeper started on an engine whose lease deadline passed fails the lease without anyone calling Sweep.
func TestSweeperFailsAnExpiredLease(t *testing.T) {
	e, now := fixture(t, 1, false)
	(*now)[0] = (*now)[0].Add(2 * e.Config.TTL) // the deadline passed
	stop := e.StartSweeper(5*time.Millisecond, t.Logf)
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, _, err := e.Store.AttemptStateEpoch(context.Background(), "a0")
		if err != nil {
			t.Fatal(err)
		}
		if state == "retryable" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the expired lease was not failed by the sweeper")
}

// Stop returns, and no sweep runs afterwards.
func TestSweeperStops(t *testing.T) {
	e, _ := fixture(t, 0, false)
	stop := e.StartSweeper(time.Millisecond, nil)
	stop()
	stop2 := e.StartSweeper(0, nil) // the default period
	stop2()
}

// expireFor grants a lease on node id to a fresh attempt n, lets its deadline pass and sweeps.
func expireFor(t *testing.T, e *Engine, now *[]time.Time, id string, n int) {
	t.Helper()
	ctx := context.Background()
	key := fmt.Sprintf("b%d", n)
	p := store.PipelineRow{ID: "p" + key, Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := e.Store.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := e.Store.CreateJob(ctx, store.JobRow{ID: "j" + key, PipelineID: p.ID, Name: key, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Store.CreateAttempt(ctx, store.AttemptRow{ID: "a" + key, JobID: "j" + key, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	d := store.Demand{AttemptID: "a" + key, LeaseID: "l" + key, RunnerID: id, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: e.Config.TTL}
	if _, err := e.Grant(ctx, "host", d, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.Store.Transition(ctx, "a"+key, "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
		t.Fatal(err)
	}
	(*now)[0] = (*now)[0].Add(2 * e.Config.TTL)
	if got, err := e.Sweep(ctx); err != nil || got != 1 {
		t.Fatalf("sweep %d: %d %v", n, got, err)
	}
}

// Three leases of one node that expire in a row put the node in maintenance; a result in between resets the streak
// (the store test of LeaseResult), and a fence the agent reported itself is not counted.
func TestSweepCountsSilentNodeFailuresInTheBreaker(t *testing.T) {
	e, now := fixture(t, 0, false)
	id := registeredNode(t, e)
	for n := 1; n <= 2; n++ {
		expireFor(t, e, now, id, n)
	}
	node, err := e.Nodes.Get(context.Background(), id)
	if err != nil || node.Record.State == "maintenance" {
		t.Fatalf("maintenance after two failures: %+v %v", node.Record, err)
	}
	expireFor(t, e, now, id, 3)
	node, err = e.Nodes.Get(context.Background(), id)
	if err != nil || node.Record.State != "maintenance" {
		t.Fatalf("a node whose leases expired three times in a row is %q, want maintenance (%v)", node.Record.State, err)
	}
}

func TestSelfFencedLeaseIsNotACountedFailure(t *testing.T) {
	e, _ := fixture(t, 1, false)
	id := registeredNode(t, e)
	ctx := context.Background()
	l, err := e.Store.LeaseGet(ctx, "l0")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Reconnect(ctx, "l0", l.Epoch, "self_fenced"); err != nil {
		t.Fatal(err)
	}
	if count, err := e.Breaker(ctx, id, false); err != nil || count != 0 {
		t.Fatalf("a self-fenced lease counted against the node: %d %v", count, err)
	}
}
