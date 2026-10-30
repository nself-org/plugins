package reliability

// Purpose: faults of the node as a whole. A black-hole node never answers: its leases expire one after another,
//          the expiry sweep counts each as an infrastructure failure, and the third in a row puts the node in
//          maintenance. An agent that offers an incompatible protocol is refused at the hello (E652), is never
//          placed on, and the placement explanation names node.protocol for it.
// Inputs:  the sim world (real store, lease engine with a node registry, scheduler, planview) and its clocks.
// Outputs: assertions only.
// Constraints: no sleeps; this is the sim variant of the black-hole (netem loss 100 %) and incompatible-agent
//          scenarios of P7-NODE-11, which the Docker layer cannot run on every machine.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// registerNode records a node the way the operator does: discovered, then set online.
func (w *world) registerNode(id string) {
	w.t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "internal", "model", "testdata", "capability", "valid", "laptop.valid.json"))
	if err != nil {
		w.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(body, &c); err != nil {
		w.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = id, id
	if _, err = w.nodes.Register(w.ctx, c, "operator"); err != nil {
		w.t.Fatal(err)
	}
	if _, err = w.nodes.SetState(w.ctx, id, "online", "operator", "operator"); err != nil {
		w.t.Fatal(err)
	}
}

// nodeState is the registry state of a node and the reason recorded with it.
func (w *world) nodeState(id string) (state, reason string) {
	w.t.Helper()
	n, err := w.nodes.Get(w.ctx, id)
	if err != nil {
		w.t.Fatal(err)
	}
	return n.Record.State, n.Record.Reason
}

func TestFaultBlackHoleNodeGoesToMaintenance(t *testing.T) {
	w := newWorld(t)
	w.registerNode("node-a")
	jobs := []string{"jbh1", "jbh2", "jbh3"}
	for i, id := range jobs {
		a1 := w.job(id, "test", true, 1)
		w.grant(a1, fmt.Sprintf("lease-bh-%d", i), "node-a", 1) // node-a takes the job and never answers again
		w.advance(coordTTL + heartbeat)                         // no heartbeat: the grant's own deadline passes
		if n, err := w.engine.Sweep(w.ctx); err != nil || n != 1 {
			t.Fatalf("sweep %d: %d leases failed, err %v", i+1, n, err)
		}
		w.wantAttempts(id, "retryable/lost", "queued/")
		if got := w.retryReasons(a1); len(got) != 1 || got[0] != "lease.expired" {
			t.Fatalf("retry reasons of %s: %v", id, got)
		}
		state, reason := w.nodeState("node-a")
		if want := i == len(jobs)-1; (state == "maintenance") != want {
			t.Fatalf("after %d lost jobs node-a is %q (%s); maintenance only after the third", i+1, state, reason)
		}
	}
	if state, reason := w.nodeState("node-a"); state != "maintenance" || !strings.Contains(reason, "three consecutive infra failures") {
		t.Fatalf("black hole is %q (%s), want maintenance for three consecutive infra failures", state, reason)
	}
	for _, id := range jobs {
		if got := w.effectLines(id); len(got) != 0 {
			t.Fatalf("%s wrote its side effect on the black hole: %v", id, got)
		}
	}

	// The retries go to the healthy node. The black hole is also ineligible by its own state, not only by anti-affinity.
	state, _ := w.nodeState("node-a")
	hole := simRunner("node-a")
	hole.Capability.Availability.State.Value = ptr(state)
	fresh := place("jbh-new", 1, "", hole, simRunner("node-b"))
	if len(fresh.Placements) != 1 || fresh.Placements[0].RunnerID != "node-b" || !contains(reasonsFor(fresh, "node-a"), string(model.NodeMaintenance)) {
		t.Fatalf("a new job: placements %+v, node-a reasons %v, want node-b and %s for node-a", fresh.Placements, reasonsFor(fresh, "node-a"), model.NodeMaintenance)
	}
	for i, id := range jobs {
		rows := w.attempts(id)
		prev := w.previousRunner(rows[1].ID)
		res := place(id, rows[1].N, prev, hole, simRunner("node-b"))
		if prev != "node-a" || len(res.Placements) != 1 || res.Placements[0].RunnerID != "node-b" {
			t.Fatalf("retry of %s: previous %q placements %+v, want node-b", id, prev, res.Placements)
		}
		b := w.startAgent("node-b", fmt.Sprintf("lease-b-%d", i), w.grant(rows[1].ID, fmt.Sprintf("lease-b-%d", i), "node-b", 1))
		w.run(heartbeat, b)
		if ran, err := b.finish(id); !ran || err != nil {
			t.Fatalf("retry of %s on node-b: ran=%t err=%v", id, ran, err)
		}
		w.wantAttempts(id, "retryable/lost", "finalizing/")
		w.wantEffect(id, "node-b")
	}
	if state, _ := w.nodeState("node-a"); state != "maintenance" {
		t.Fatalf("black hole left maintenance by itself: %q", state)
	}
}

func TestFaultIncompatibleAgent(t *testing.T) {
	w := newWorld(t)

	// The handshake: an agent that speaks only protocol 2 is refused with E652 and never gets a session.
	if v, rej := protocol.Negotiate([]int{2}, []int{1}, "2.0.0", "1.0.0"); v != 0 || rej == nil || rej.Code != protocol.CodeVersion || rej.Reason != "no_common_version" {
		t.Fatalf("incompatible hello: version %d reject %+v, want E652 no_common_version", v, rej)
	}
	if v, rej := protocol.Negotiate([]int{1}, []int{1}, "1.4.0", "1.0.0"); v != 1 || rej != nil {
		t.Fatalf("compatible hello: version %d reject %+v, want 1", v, rej)
	}

	// Placement: the incompatible node is ruled out with node.protocol and the job runs on the compatible one.
	v2 := simRunner("node-v2")
	v2.Capability.Protocol.Versions, v2.Capability.Protocol.Min = []int{2}, 2
	good := simRunner("node-b")
	a1 := w.job("jproto", "test", true, 1)
	sw, sj := placeInputs("jproto", 1, "", v2, good)
	res := sched.Place(sw, []sched.Job{sj})
	if len(res.Placements) != 1 || res.Placements[0].RunnerID != "node-b" {
		t.Fatalf("placement %+v, want node-b", res.Placements)
	}
	if got := reasonsFor(res, "node-v2"); !contains(got, string(model.NodeProtocol)) {
		t.Fatalf("node-v2 reasons %v, want %s", got, model.NodeProtocol)
	}
	b := w.startAgent("node-b", "lease-b", w.grant(a1, "lease-b", "node-b", 1))
	w.run(heartbeat, b)
	if ran, err := b.finish("jproto"); !ran || err != nil {
		t.Fatalf("job on node-b: ran=%t err=%v", ran, err)
	}
	w.wantAttempts("jproto", "finalizing/")
	w.wantEffect("jproto", "node-b")

	// `ci explain` renders this plan document: it names the protocol reason for the incompatible node.
	lines := strings.Join(planview.ExplainLines(planview.Build(planview.Input{Name: "pipe", World: sw, Jobs: []sched.Job{sj}, Result: res}), planview.SourcePlan), "\n")
	if !strings.Contains(lines, "node-v2  ineligible  "+string(model.NodeProtocol)) {
		t.Fatalf("the explanation does not name %s for node-v2:\n%s", model.NodeProtocol, lines)
	}

	// With only the incompatible node the job is unplaceable and the explanation says why.
	aw, aj := placeInputs("jalone", 1, "", v2)
	alone := sched.Place(aw, []sched.Job{aj})
	if len(alone.Placements) != 0 || !contains(reasonsFor(alone, "node-v2"), string(model.NodeProtocol)) {
		t.Fatalf("a job with only node-v2: placements %+v reasons %v", alone.Placements, reasonsFor(alone, "node-v2"))
	}
}
