//go:build integration

package reliability

// Purpose: suspend and clock jumps (D6). The agent fence reads the wall clock and a monotonic clock and acts on
//          the larger delta, so neither a ten-minute suspend (wall jumps, monotonic stands still) nor a stepped
//          wall clock can extend a lease; a late acknowledgement cannot revive a fenced agent; a clock stepped
//          backwards neither fences nor extends. The coordinator side: leases of a coordinator that was away
//          for ten minutes get their restart grace and nothing is expired before the agents could report.
// Inputs:  injected clocks (the agent's), an injected engine clock (the coordinator's); no sleeping.
// Constraints: no Docker: the clocks are the fault.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type fakeClocks struct {
	wall time.Time
	mono time.Duration
}

func (c *fakeClocks) fence(fenced *int) *lease.AgentFence {
	f := lease.NewAgentFence(func() time.Time { return c.wall }, func() time.Duration { return c.mono }, 45*time.Second, 10*time.Second)
	f.OnFence = func() { *fenced++ }
	return f
}

func TestFaultSuspendAndClockJump(t *testing.T) {
	t.Run("a ten-minute suspend: wall jumps, monotonic stands still", func(t *testing.T) {
		c := &fakeClocks{wall: time.Unix(1000, 0)}
		var fenced int
		f := c.fence(&fenced)
		c.wall = c.wall.Add(10 * time.Minute)
		if !f.Check() || fenced != 1 {
			t.Fatalf("the wake did not fence (fenced %d)", fenced)
		}
		f.Ack() // the coordinator answers late: too late
		if !f.Check() || fenced != 1 {
			t.Fatalf("a late ack revived the fence or fired it twice (fenced %d)", fenced)
		}
	})
	t.Run("monotonic time passes while the wall clock was stepped back", func(t *testing.T) {
		c := &fakeClocks{wall: time.Unix(1000, 0)}
		var fenced int
		f := c.fence(&fenced)
		c.mono += 10 * time.Minute
		c.wall = c.wall.Add(-time.Hour)
		if !f.Check() || fenced != 1 {
			t.Fatalf("a stepped-back wall clock hid 10 minutes of silence (fenced %d)", fenced)
		}
	})
	t.Run("a wall clock stepped back inside the TTL neither fences nor extends", func(t *testing.T) {
		c := &fakeClocks{wall: time.Unix(1000, 0)}
		var fenced int
		f := c.fence(&fenced)
		c.mono += 10 * time.Second
		c.wall = c.wall.Add(-time.Hour)
		if f.Check() || fenced != 0 {
			t.Fatalf("fenced after 10 s of silence (fenced %d)", fenced)
		}
		c.mono += 30 * time.Second // 40 s of silence in all: past agent_ttl (25 s)
		if !f.Check() {
			t.Fatal("the stepped clock extended the lease")
		}
	})
	t.Run("an ack inside agent_ttl keeps the agent running through a long wall jump on the monotonic clock", func(t *testing.T) {
		c := &fakeClocks{wall: time.Unix(1000, 0)}
		var fenced int
		f := c.fence(&fenced)
		c.mono += 10 * time.Second
		c.wall = c.wall.Add(10 * time.Second)
		f.Ack()
		if f.Check() || fenced != 0 {
			t.Fatalf("a healthy agent fenced (fenced %d)", fenced)
		}
	})
}

// A coordinator that was suspended for ten minutes expires nothing on wake when it first gives the leases of
// the earlier incarnation their restart grace, and expires them exactly when the grace ends.
func TestFaultCoordinatorSuspendKeepsRestartGrace(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	st, err := store.Open(t.TempDir()+"/state.db", store.Options{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	eng := lease.New(st, nil, lease.Config{Clock: clock, TTL: 45 * time.Second, Heartbeat: 10 * time.Second})
	ctx := context.Background()
	p := store.PipelineRow{ID: "p", Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err = st.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateJob(ctx, store.JobRow{ID: "j", PipelineID: "p", Name: "j", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	old, err := st.Register(ctx, 4242, "boot-old", "old-coordinator")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: "a", JobID: "j", N: 1, CoordinatorID: old.ID}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = eng.Grant(ctx, "host", store.Demand{AttemptID: "a", LeaseID: "l", RunnerID: "n1", CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: 45 * time.Second}, 1); err != nil {
		t.Fatal(err)
	}

	now = now.Add(10 * time.Minute) // the suspend: the wall clock jumps, the deadline is far behind
	if _, err = eng.Start(ctx, "new-coordinator", 2); err != nil {
		t.Fatal(err)
	}
	if got, err := eng.Sweep(ctx); err != nil || got != 0 {
		t.Fatalf("the sweep right after the wake expired %d leases (%v): the restart grace was not applied", got, err)
	}
	now = now.Add(44 * time.Second)
	if got, _ := eng.Sweep(ctx); got != 0 {
		t.Fatalf("the sweep inside the grace expired %d leases", got)
	}
	now = now.Add(2 * time.Second)
	if got, err := eng.Sweep(ctx); err != nil || got != 1 {
		t.Fatalf("the sweep after the grace expired %d leases (%v), want the silent one", got, err)
	}
}
