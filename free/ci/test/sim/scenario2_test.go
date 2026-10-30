//go:build integration

package sim

// Purpose: TestScenario2, the personal LAN leg (P7-NODE-17, NODE acceptance 1 and the LAN part of 8). A bare
//          foreground run (the run.Coordinator `nself ci` builds, V15 dispatch, the real ssh driver and the node
//          session over pipes) places a job on one of two paired LAN nodes while a paired node of another
//          architecture, a discovered-unpaired candidate and a deploy-inventory host are registered and online.
// Inputs:  INTEGRATION=1. No Docker. The NAT'd pull-agent leg is TestAgentPull (internal/transport/agent), whose
//          subtests run a real agent in a Docker node that publishes no port.
// Outputs: metrics node.dispatch_ms (run start to the first job process on a node, 20 runs),
//          node.heartbeat_cpu_pct and node.discovery_cpu_pct (see each metric's context).
// Constraints: the candidate, the deploy host and the other-architecture node run nothing and receive no session;
//          `ci explain` shows every alternative with its reason. A foreground run asks for the invoking host's own
//          platform (internal/run dispatch_world.go), so the two paired nodes declare that platform.

import (
	"context"
	"fmt"
	"net/netip"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/discovery"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/schedtel"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type lanNet struct{ entries []discovery.Entry }

func (l lanNet) Browse(context.Context, string, time.Duration) ([]discovery.Entry, error) {
	return l.entries, nil
}
func (lanNet) Advertise(discovery.Service) (func(), error) { return func() {}, nil }
func (lanNet) LocalAddrs() []netip.Addr                    { return nil }

// drain empties start signals left by an earlier run.
func drain(chs ...chan string) {
	for _, c := range chs {
		for len(c) > 0 {
			<-c
		}
	}
}

// explainLines is `ci explain` for job unit of a run: the recorded placement decision, every alternative with its reason.
func explainLines(t *testing.T, f *fixture, runID string) string {
	t.Helper()
	src := schedtel.Source{Open: func(context.Context) (*store.Store, func(), error) {
		st, err := store.Open(f.home+"/state.db", store.Options{})
		if err != nil {
			return nil, nil, err
		}
		return st, func() { _ = st.Close() }, nil
	}}
	plan, err := src.Decision(context.Background(), runID, "unit")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	return strings.Join(planview.ExplainLines(plan, "decision-record"), "\n")
}

func TestScenario2(t *testing.T) {
	pass := passRunner("lan job")
	ids := []string{"lan-a", "lan-b", "lan-other-arch", "cand-unpaired", "deploy-host"}
	runners := map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){}
	for _, id := range ids {
		runners[id] = pass
	}
	host := func(shape func(string, string) func(*model.Capability)) func(*model.Capability) {
		return shape(runtime.GOOS, runtime.GOARCH)
	}
	f := newFixtureWith(t, []nodeSpec{
		{id: "lan-a", host: "10.0.0.5", shape: host(shapePlatform)},
		{id: "lan-b", host: "10.0.0.6", shape: host(shapePlatform)},
		{id: "lan-other-arch", host: "10.0.0.7", shape: shapePlatform("linux", "riscv64")},
		{id: "cand-unpaired", host: "10.0.0.8", shape: host(shapeDiscovered)},
		{id: "deploy-host", host: "10.0.0.9", shape: host(shapeDeployHost)},
	}, runners)
	byID := map[string]*node{}
	for _, n := range f.nodes {
		byID[n.id] = n
	}
	ctx := context.Background()
	never := []string{"lan-other-arch", "cand-unpaired", "deploy-host"}
	requireNever := func(t *testing.T) {
		t.Helper()
		for _, id := range never {
			if n := byID[id]; len(n.ran()) != 0 || n.sessions.Load() != 0 {
				t.Errorf("%s ran %v and opened %d sessions", id, n.ran(), n.sessions.Load())
			}
		}
	}

	t.Run("a job lands on a paired LAN node and explain lists every alternative with its reason", func(t *testing.T) {
		ch, out := f.start(ctx, remoteJob(1))
		res := wait(t, ch, "the run")
		lines := explainLines(t, f, res.RunID)
		t.Logf("ci explain:\n%s", lines)
		if res.Exit != 0 || res.Summary.Result != "pass" {
			t.Fatalf("exit %d result %s\n%s", res.Exit, res.Summary.Result, out)
		}
		if ran := len(byID["lan-a"].ran()) + len(byID["lan-b"].ran()); ran != 1 {
			t.Fatalf("the job ran %d times on the paired nodes, want 1", ran)
		}
		requireNever(t)
		for _, want := range []string{"lan-a", "lan-b", "lan-other-arch", "cand-unpaired", "deploy-host",
			string(model.PlatformMismatch), string(model.NodeDiscovered), string(model.NodeDeployHost)} {
			if !strings.Contains(lines, want) {
				t.Errorf("explain does not mention %q", want)
			}
		}
	})

	t.Run("a discovered candidate stays discovered and unauthorized", func(t *testing.T) {
		g := &discovery.Guard{Home: t.TempDir(), Net: lanNet{[]discovery.Entry{{Name: "laptop._nself-ci._tcp.local.", Host: "laptop.local.",
			Addrs: []netip.Addr{netip.MustParseAddr("192.168.1.50")}, Port: 22, TXT: []string{"w=0a1b2c3d"}}}}}
		r, err := g.Browse(ctx, time.Second)
		if err != nil || len(r.Candidates) != 1 {
			t.Fatalf("browse: %v %+v", err, r)
		}
		if c := r.Candidates[0]; c.State != discovery.StateDiscovered || c.Authorized {
			t.Fatalf("candidate %+v", c)
		}
	})

	t.Run("dispatch overhead on the LAN is measured and no excluded node gets a job", func(t *testing.T) {
		var samples []float64
		for i := 0; i < 20; i++ {
			drain(byID["lan-a"].started, byID["lan-b"].started)
			begin := time.Now()
			ch, out := f.start(ctx, remoteJob(1))
			var first time.Time
			deadline := time.After(60 * time.Second)
		wait:
			for {
				select {
				case <-byID["lan-a"].started:
					first = time.Now()
					break wait
				case <-byID["lan-b"].started:
					first = time.Now()
					break wait
				case <-deadline:
					t.Fatalf("run %d: no job process started\n%s", i, out)
				}
			}
			samples = append(samples, ms(first.Sub(begin)))
			if res := wait(t, ch, "the run"); res.Exit != 0 {
				t.Fatalf("run %d exit %d\n%s", i, res.Exit, out)
			}
		}
		recordPerf("scenario2", "node.dispatch_ms", "ms", "run start to the first job process on a paired LAN node (plan, place, lease, ssh session, hello, start), in-process session, 20 sequential runs", samples)
		requireNever(t)
	})

	t.Run("heartbeat and discovery overhead are measured", func(t *testing.T) {
		const window = 10 * time.Second
		idle0 := cpuTime()
		time.Sleep(window)
		idle := cpuPct(idle0, cpuTime(), window)

		gate := make(chan struct{})
		slow := func(ctx context.Context, n *node, j session.JobRun) (session.JobResult, error) {
			select {
			case <-gate:
			case <-ctx.Done():
			}
			return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: evidence(j.Lease, "pass", "", nil)}, nil
		}
		byID["lan-a"].runner, byID["lan-b"].runner = slow, slow
		drain(byID["lan-a"].started, byID["lan-b"].started)
		ch, out := f.start(ctx, remoteJob(1))
		select {
		case <-byID["lan-a"].started:
		case <-byID["lan-b"].started:
		case <-time.After(60 * time.Second):
			t.Fatalf("the job never started\n%s", out)
		}
		busy0 := cpuTime()
		time.Sleep(window)
		busy := cpuPct(busy0, cpuTime(), window)
		close(gate)
		if res := wait(t, ch, "the run"); res.Exit != 0 {
			t.Fatalf("exit %d\n%s", res.Exit, out)
		}
		over := busy - idle
		if over < 0 {
			over = 0
		}
		recordPerf("scenario2", "node.heartbeat_cpu_pct", "%", fmt.Sprintf("process CPU while one remote job holds a session at a 1 s heartbeat (10x the 10 s production rate) minus the idle process, one core = 100, %v window; busy %.3f idle %.3f", window, busy, idle), []float64{over})

		g := &discovery.Guard{Home: t.TempDir()}
		c0, w0 := cpuTime(), time.Now()
		if _, err := g.Browse(ctx, 2*time.Second); err != nil {
			t.Fatalf("browse: %v", err)
		}
		recordPerf("scenario2", "node.discovery_cpu_pct", "%", "process CPU during one real mDNS browse of 2 s on this host, one core = 100", []float64{cpuPct(c0, cpuTime(), time.Since(w0))})
	})
}
