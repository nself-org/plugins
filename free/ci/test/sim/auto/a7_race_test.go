package auto

// Purpose: A7 two eligible nodes race one job: the faster finishes, the slower is cancelled, and
// exactly one result is recorded and posted; a repeated cancel does nothing.
// Inputs: race.Select and race.Run over a virtual two-node cluster. Outputs: pass or fail.
// Constraints: no processes, no sleeps; the virtual clock ends the shortest process group.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/race"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

type rnode struct {
	takes  time.Duration
	pgroup bool
	end    chan race.Result
}

type cluster struct {
	mu       sync.Mutex
	nodes    map[string]*rnode
	admitted int
	posted   []race.Evidence
	now      time.Duration
}

func (c *cluster) Create(_ context.Context, n int, runner string) (race.Sibling, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodes[runner].pgroup = true
	c.admitted++
	if c.admitted == len(c.nodes) {
		var next *rnode
		for _, nd := range c.nodes {
			if nd.pgroup && (next == nil || nd.takes < next.takes) {
				next = nd
			}
		}
		c.now, next.pgroup = next.takes, false
		next.end <- race.Result{State: race.StatePassed}
	}
	return race.Sibling{N: n, AttemptID: fmt.Sprintf("job.a%d", n), LeaseID: "lease-" + runner, RunnerID: runner, Epoch: 1}, nil
}

func (c *cluster) Wait(_ context.Context, s race.Sibling) race.Result {
	return <-c.nodes[s.RunnerID].end
}

func (c *cluster) Cancel(_ context.Context, s race.Sibling, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n := c.nodes[s.RunnerID]; n.pgroup {
		n.pgroup = false
		n.end <- race.Result{State: race.StateCancelled, FailureClass: race.StateCancelled}
	}
	return nil
}

func (c *cluster) Post(_ context.Context, e race.Evidence) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.posted = append(c.posted, e)
	return nil
}

func TestConformanceA7Race(t *testing.T) {
	w := sched.World{Runners: []sched.Runner{runnerOf("fast", 16384, "idle"), runnerOf("slow", 16384, "idle")},
		Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	j := sched.Job{Key: "job", ID: "job", Project: "p", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000}
	choice := race.Select(w, j, race.DefaultWidth, false)
	if !choice.Raceable() || len(choice.Runners) != 2 {
		t.Fatalf("choice = %+v", choice)
	}
	c := &cluster{nodes: map[string]*rnode{
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
	if c.nodes["fast"].pgroup || c.nodes["slow"].pgroup {
		t.Fatal("a process group outlived the race")
	}
	if len(ev.Losers) != 1 || ev.Losers[0].RunnerID != "slow" || len(c.posted) != 1 {
		t.Fatalf("losers = %+v, posted %d", ev.Losers, len(c.posted))
	}
	if err = c.Cancel(context.Background(), ev.Losers[0].Sibling, "again"); err != nil || len(c.nodes["slow"].end) != 0 {
		t.Fatal("a repeated cancel had an effect")
	}
}
