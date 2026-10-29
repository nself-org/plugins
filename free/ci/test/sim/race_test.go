package sim

// Purpose: P7-AUTO-14 Sim A7: two eligible nodes race one job; the faster
// one finishes, its result stands, the slower one is cancelled (its remote
// process group is gone), and exactly one result is recorded and posted.
// Inputs:  a placement world of two owned LAN nodes and a paid one; each
// node runs the job in a simulated process group that ends after a fixed
// virtual duration.
// Outputs: pass or fail.
// Constraints: pure Go, no processes, no network, no sleeps: a virtual clock
// ends the shortest live process group once every sibling is admitted.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/race"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// raceNode is a remote node; pgroup is the process group of the sibling it runs.
type raceNode struct {
	takes  time.Duration
	pgroup bool
	end    chan race.Result
}

type raceCluster struct {
	mu       sync.Mutex
	nodes    map[string]*raceNode
	want     int
	admitted []race.Sibling
	recorded []race.Evidence // the run store's result rows for the job
	posted   []race.Evidence // statuses posted for the commit
	now      time.Duration
}

func (c *raceCluster) Create(_ context.Context, n int, runner string) (race.Sibling, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	node := c.nodes[runner]
	node.pgroup = true
	s := race.Sibling{N: n, AttemptID: fmt.Sprintf("job.a%d", n), LeaseID: "lease-" + runner, RunnerID: runner, Epoch: 1}
	c.admitted = append(c.admitted, s)
	if len(c.admitted) == c.want {
		c.advance()
	}
	return s, nil
}

// advance moves the virtual clock to the end of the shortest live process group.
func (c *raceCluster) advance() {
	var next *raceNode
	for _, n := range c.nodes {
		if n.pgroup && (next == nil || n.takes < next.takes) {
			next = n
		}
	}
	if next != nil {
		c.now = next.takes
		next.pgroup = false
		next.end <- race.Result{State: race.StatePassed}
	}
}

func (c *raceCluster) Wait(_ context.Context, s race.Sibling) race.Result {
	return <-c.nodes[s.RunnerID].end
}

// Cancel is the lease cancel: the node kills the sibling's process group.
func (c *raceCluster) Cancel(_ context.Context, s race.Sibling, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := c.nodes[s.RunnerID]
	if !n.pgroup || s.Epoch != 1 {
		return nil // already ended, or a stale epoch: idempotent
	}
	n.pgroup = false
	n.end <- race.Result{State: race.StateCancelled, FailureClass: race.StateCancelled}
	return nil
}

func (c *raceCluster) Post(_ context.Context, e race.Evidence) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recorded = append(c.recorded, e)
	c.posted = append(c.posted, e)
	return nil
}

func raceRunner(id, class string) sched.Runner {
	loc, os, arch := "lan", "linux", "amd64"
	cpu, mem := float64(8), int64(8192)
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects, c.Lifecycle.Eligible = []string{"p"}, true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = &os, &arch
	c.Resources.CPU.Value, c.Resources.MemMB.Value = &cpu, &mem
	c.Location.Kind.Value = &loc
	return sched.Runner{ID: id, Location: loc, Class: class, Provider: "operator", Avail: sched.Avail{Slots: 2, CPU: 16, MemMB: 16384}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", NoLANRoute: true}}
}

func TestRaceCancelsLoserProcessGroup(t *testing.T) {
	w := sched.World{Runners: []sched.Runner{raceRunner("fast", "owned"), raceRunner("slow", "owned"), raceRunner("rented", "paid")},
		Policy: sched.Policy{Preset: "balanced", PaidEnabled: true}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	j := sched.Job{Key: "job", ID: "job", Project: "p", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000}
	choice := race.Select(w, j, race.DefaultWidth, false)
	if !choice.Raceable() || len(choice.Runners) != 2 || choice.Excluded["rented"] != race.ReasonSpend {
		t.Fatalf("choice = %+v", choice)
	}
	c := &raceCluster{want: 2, nodes: map[string]*raceNode{
		"fast": {takes: 40 * time.Second, end: make(chan race.Result, 1)},
		"slow": {takes: 90 * time.Second, end: make(chan race.Result, 1)},
	}}
	ev, err := race.Run(context.Background(), race.Spec{Job: "job", Choice: choice, First: 1}, c, c)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Winner == nil || ev.Winner.RunnerID != "fast" || ev.Result.State != race.StatePassed || c.now != 40*time.Second {
		t.Fatalf("winner = %+v result = %+v at %v", ev.Winner, ev.Result, c.now)
	}
	if c.nodes["slow"].pgroup || c.nodes["fast"].pgroup {
		t.Fatal("a process group outlived the race")
	}
	cancelled := ev.CancelledSiblings()
	if len(cancelled) != 1 || ev.Losers[0].RunnerID != "slow" {
		t.Fatalf("cancelled = %v losers = %+v", cancelled, ev.Losers)
	}
	if len(c.recorded) != 1 || len(c.posted) != 1 {
		t.Fatalf("recorded %d, posted %d: want exactly one of each", len(c.recorded), len(c.posted))
	}
	// a repeat cancel after the race is a no-op (idempotent, epoch fenced)
	if err = c.Cancel(context.Background(), ev.Losers[0].Sibling, "again"); err != nil || len(c.nodes["slow"].end) != 0 {
		t.Fatal("a repeated cancel had an effect")
	}
}
