package reliability

// Purpose: the node side of the sim: an agent with its own wall and monotonic clocks and the real
//          lease.AgentFence, plus the placement view (sched.Place) a retry goes through.
// Inputs:  the world, a runner name and the lease the agent holds.
// Outputs: heartbeats, job completion (which writes the side-effect file) and late carrier frames.
// Constraints: a frozen agent's clocks keep moving (time passes) but it runs no code; a suspended agent's
//          monotonic clock stops while its wall clock jumps; no sleeps.

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// agent is one node agent holding one lease.
type agent struct {
	w           *world
	runner      string
	leaseID     string
	epoch       int64
	wall        time.Time
	mono        time.Duration
	fence       *lease.AgentFence
	fencedAt    time.Time // world time at which the fence latched (zero: never)
	frozen      bool      // SIGSTOP / docker pause: no code runs
	partitioned bool      // the carrier cannot reach the coordinator
	dead        bool      // kill -9: never runs again
	acks        int
	lastAck     time.Time // world time of the last acknowledged heartbeat
}

func (w *world) startAgent(runner, leaseID string, epoch int64) *agent {
	a := &agent{w: w, runner: runner, leaseID: leaseID, epoch: epoch, wall: w.clock()}
	a.fence = lease.NewAgentFence(func() time.Time { return a.wall }, func() time.Duration { return a.mono }, coordTTL, heartbeat)
	a.fence.OnFence = func() { a.fencedAt = w.clock() }
	a.lastAck = w.clock()
	return a
}

// run moves time forward in heartbeat steps; every live, unfrozen agent heartbeats at each step.
func (w *world) run(d time.Duration, agents ...*agent) {
	for left := d; left > 0; left -= heartbeat {
		step := min(left, heartbeat)
		w.advance(step)
		for _, a := range agents {
			a.wall = a.wall.Add(step)
			a.mono += step
			if !a.frozen && !a.dead {
				a.beat()
			}
		}
	}
}

// beat is one heartbeat: the fence is checked first, then the coordinator is asked to renew.
func (a *agent) beat() {
	if a.fence.Check() || a.partitioned || a.w.down {
		return
	}
	if a.w.engine.Renew(a.w.ctx, a.leaseID, a.epoch, 0) == nil {
		a.fence.Ack()
		a.acks++
		a.lastAck = a.w.clock()
	}
}

// finish completes the fixture job: on a fenced agent the job is killed before its side effect;
// otherwise the side effect is written and the result is sent.
func (a *agent) finish(jobID string) (ran bool, err error) {
	if a.dead || a.fence.Check() {
		return false, nil
	}
	a.w.effect(jobID, a.runner)
	return true, a.w.engine.Result(a.w.ctx, a.leaseID, a.epoch)
}

// lateResult is a result frame buffered before the fault and delivered after it.
func (a *agent) lateResult() error { return a.w.engine.Result(a.w.ctx, a.leaseID, a.epoch) }

// reconnect is the agent's report after a carrier or coordinator outage.
func (a *agent) reconnect() error {
	status := "running"
	if a.fence.Check() {
		status = "self_fenced"
	}
	return a.w.engine.Reconnect(a.w.ctx, a.leaseID, a.epoch, status)
}

func ptr[T any](v T) *T { return &v }

// simRunner is an eligible linux/amd64 node speaking protocol 1.
func simRunner(id string) sched.Runner {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions = []int{1}
	c.Protocol.Min = 1
	c.Platform.OS.Value = ptr("linux")
	c.Platform.Arch.Value = ptr("amd64")
	c.Resources.CPU.Value = ptr(float64(8))
	c.Resources.MemMB.Value = ptr(int64(8192))
	c.Location.Kind.Value = ptr("lan")
	return sched.Runner{ID: id, Location: "lan", Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: 1, CPU: 8, MemMB: 8192}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", NoLANRoute: true}}
}

// placeInputs is the scheduler's world and the one job a placement of jobKey asks about; previous is the runner
// retry anti-affinity excludes.
func placeInputs(jobKey string, attempt int, previous string, runners ...sched.Runner) (sched.World, sched.Job) {
	w := sched.World{Runners: runners, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	if previous != "" {
		w.Previous[jobKey] = previous
	}
	j := sched.Job{Key: jobKey, ID: jobKey, Project: "project", Attempt: attempt, Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000}
	return w, j
}

// place runs the scheduler for one job attempt.
func place(jobKey string, attempt int, previous string, runners ...sched.Runner) sched.Result {
	w, j := placeInputs(jobKey, attempt, previous, runners...)
	return sched.Place(w, []sched.Job{j})
}

// reasonsFor returns the placement reason codes the scheduler gave one runner.
func reasonsFor(r sched.Result, runner string) []string {
	var out []string
	for _, x := range r.Explanations {
		for _, alt := range x.Alternatives {
			if alt.RunnerID == runner {
				for _, q := range alt.Reasons {
					out = append(out, q.Code)
				}
			}
		}
	}
	return out
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
