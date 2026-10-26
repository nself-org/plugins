package reliability

// Purpose: coordinator outages (EPIC D6, Acceptance 5), kill -9 of the coordinator, a VPS reboot of a
//          node and drain hand-back. Down 20 s (< agent TTL) with 20 running agents: 0 requeues. Down
//          5 minutes: every agent self-fences, each attempt goes lost (agent.self_fenced) and is retried
//          exactly once, each side-effect file is written once, and the coordinator expires no lease for
//          its own downtime. Drain hand-back never overlaps two attempts of a job.
// Inputs:  the sim world; a coordinator restart is a new lease.Engine over the same store.
// Outputs: assertions only.
// Constraints: no sleeps.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/drain"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// fleet grants n jobs on n runners and lets one heartbeat land.
func fleet(t *testing.T, w *world, n int) []*agent {
	t.Helper()
	agents := make([]*agent, n)
	for i := range agents {
		id := fmt.Sprintf("j%02d", i)
		runner := fmt.Sprintf("node-%02d", i)
		lid := "lease-" + id
		agents[i] = w.startAgent(runner, lid, w.grant(w.job(id, "test", true, 1), lid, runner, 1))
	}
	w.run(heartbeat, agents...)
	return agents
}

// outage stops the coordinator for d, then starts a new incarnation with restart grace.
func outage(t *testing.T, w *world, d time.Duration, agents []*agent) {
	t.Helper()
	w.down = true
	w.run(d, agents...)
	w.engine = w.coordinator()
	w.down = false
	if _, err := w.engine.Start(w.ctx, "coordinator-2", 2); err != nil {
		t.Fatal(err)
	}
}

func TestFaultCoordinatorDownShort(t *testing.T) {
	w := newWorld(t)
	agents := fleet(t, w, 20)
	outage(t, w, 20*time.Second, agents)
	for i, a := range agents {
		if !a.fencedAt.IsZero() {
			t.Fatalf("agent %d fenced during a 20 s outage", i)
		}
		if err := a.reconnect(); err != nil {
			t.Fatalf("reconnect %d: %v", i, err)
		}
	}
	if n, err := w.engine.Sweep(w.ctx); err != nil || n != 0 {
		t.Fatalf("short outage requeued %d: %v", n, err)
	}
	for i, a := range agents {
		id := fmt.Sprintf("j%02d", i)
		if ran, err := a.finish(id); !ran || err != nil {
			t.Fatalf("job %s after short outage: ran=%t err=%v", id, ran, err)
		}
		w.wantAttempts(id, "finalizing/")
		w.wantEffect(id, a.runner)
	}
}

func TestFaultCoordinatorDownLong(t *testing.T) {
	w := newWorld(t)
	agents := fleet(t, w, 20)
	down := w.clock()
	outage(t, w, 5*time.Minute, agents)
	if n, err := w.engine.Sweep(w.ctx); err != nil || n != 0 {
		t.Fatalf("coordinator expired %d leases for its own downtime: %v", n, err)
	}
	for i, a := range agents {
		id := fmt.Sprintf("j%02d", i)
		if a.fencedAt.IsZero() || !a.fencedAt.After(down) || a.fencedAt.Sub(down) > coordTTL {
			t.Fatalf("agent %d fenced at %v (outage began %v)", i, a.fencedAt, down)
		}
		if ran, _ := a.finish(id); ran {
			t.Fatalf("fenced job %s wrote its side effect", id)
		}
		if err := a.reconnect(); err != nil {
			t.Fatalf("fence report %d: %v", i, err)
		}
		wantCode(t, "duplicate fence report", a.reconnect(), "E662")
		if got := w.retryReasons(id + ".a1"); len(got) != 1 || got[0] != "agent.self_fenced" {
			t.Fatalf("job %s retry reasons %v", id, got)
		}
	}
	for i, a := range agents {
		id := fmt.Sprintf("j%02d", i)
		rows := w.wantAttempts(id, "retryable/lost", "queued/")
		next := fmt.Sprintf("node-%02d", (i+1)%len(agents))
		lid := "lease-r-" + id
		b := w.startAgent(next, lid, w.grant(rows[1].ID, lid, next, 2))
		if ran, err := b.finish(id); !ran || err != nil {
			t.Fatalf("retry %s: ran=%t err=%v", id, ran, err)
		}
		wantCode(t, "late result of the fenced attempt", a.lateResult(), "E662")
		w.wantAttempts(id, "retryable/lost", "finalizing/")
		w.wantEffect(id, next)
	}
}

func TestFaultCoordinatorKilledMidRun(t *testing.T) {
	w := newWorld(t)
	agents := fleet(t, w, 3)
	outage(t, w, 0, agents) // kill -9 and immediate restart: the new incarnation re-adopts every lease
	w.run(3*heartbeat, agents...)
	if n, err := w.engine.Sweep(w.ctx); err != nil || n != 0 {
		t.Fatalf("restart requeued %d: %v", n, err)
	}
	for i, a := range agents {
		id := fmt.Sprintf("j%02d", i)
		if ran, err := a.finish(id); !ran || err != nil {
			t.Fatalf("job %s: ran=%t err=%v", id, ran, err)
		}
		w.wantEffect(id, a.runner)
	}
}

func TestFaultNodeReboot(t *testing.T) {
	w := newWorld(t)
	a1 := w.job("jboot", "test", true, 1)
	a := w.startAgent("node-a", "lease-a", w.grant(a1, "lease-a", "node-a", 1))
	w.run(heartbeat, a)
	// The container restarts: the job process is gone and the new agent process holds no live job, so it
	// reports the lease it finds in its spool as self-fenced.
	a.dead = true
	w.run(heartbeat, a)
	if err := w.engine.Reconnect(w.ctx, "lease-a", a.epoch, "self_fenced"); err != nil {
		t.Fatal(err)
	}
	rows := w.wantAttempts("jboot", "retryable/lost", "queued/")
	if prev := w.previousRunner(rows[1].ID); prev != "node-a" {
		t.Fatalf("previous runner %q", prev)
	}
	b := w.startAgent("node-b", "lease-b", w.grant(rows[1].ID, "lease-b", "node-b", 1))
	if ran, err := b.finish("jboot"); !ran || err != nil {
		t.Fatalf("retry: ran=%t err=%v", ran, err)
	}
	w.wantEffect("jboot", "node-b")
}

func TestFaultDrainHandBack(t *testing.T) {
	w := newWorld(t)
	a1 := w.job("jdrain", "test", true, 1)
	a := w.startAgent("node-a", "lease-a", w.grant(a1, "lease-a", "node-a", 1))
	w.run(heartbeat, a)
	leases := &drain.EngineLeases{Engine: w.engine, JobInfo: func(context.Context, string) (string, bool, error) { return "test", true, nil }}
	running, err := leases.Running(w.ctx)
	if err != nil || len(running) != 1 || !running[0].HandBackable() {
		t.Fatalf("running leases %+v %v", running, err)
	}
	a.partitioned = true // the draining node stops answering; the drain waits for expiry
	w.run(coordTTL+heartbeat, a)
	if n := w.active("jdrain"); n != 1 {
		t.Fatalf("active attempts before hand-back = %d", n)
	}
	if err := leases.HandBack(w.ctx, running[0], true, "drain"); err != nil {
		t.Fatal(err)
	}
	if n := w.active("jdrain"); n != 0 {
		t.Fatalf("active attempts after hand-back = %d, want 0 before the retry is leased", n)
	}
	wantCode(t, "old lease after hand-back", w.engine.Fence(w.ctx, "lease-a", a.epoch), "E662")
	rows := w.wantAttempts("jdrain", "retryable/lost", "queued/")
	draining := simRunner("node-a")
	draining.Capability.Availability.State.Value = ptr("draining")
	res := place("jdrain", 2, "node-a", draining, simRunner("node-b"))
	if len(res.Placements) != 1 || res.Placements[0].RunnerID != "node-b" || !contains(reasonsFor(res, "node-a"), string(model.NodeDraining)) {
		t.Fatalf("placement after drain %+v", res)
	}
	b := w.startAgent("node-b", "lease-b", w.grant(rows[1].ID, "lease-b", "node-b", 1))
	if n := w.active("jdrain"); n != 1 {
		t.Fatalf("active attempts after retry lease = %d", n)
	}
	a.partitioned = false
	if ran, _ := a.finish("jdrain"); ran {
		t.Fatal("handed-back job wrote its side effect")
	}
	if ran, err := b.finish("jdrain"); !ran || err != nil {
		t.Fatalf("retry: ran=%t err=%v", ran, err)
	}
	w.wantEffect("jdrain", "node-b")
}
