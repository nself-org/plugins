package agent

// Purpose: the acceptance scenarios of the pull carrier (P7-NODE-08), written once and run both as plain tests
//          and as subtests of the integration test TestAgentPull.
// Inputs:  the e2e rig (real listener, store, CA, gate, driver) and a Carrier as the agent.
// Constraints: bounded waits; counts are exact (no duplicate or missing line, one result).

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// session drives one lease through a carrier the way the agent session would.
type leaseRun struct {
	t     *testing.T
	e     *e2e
	c     *Carrier
	l     protocol.Lease
	done  <-chan error
	seq   uint64
	token string
}

// startLease seeds an attempt, starts Driver.Run for it and waits for the agent to receive the lease.
func (e *e2e) startLease(t *testing.T, c *Carrier, node string, n string) *leaseRun {
	t.Helper()
	leaseID, att, ep := e.seedLease(n, node)
	done := e.runLease(context.Background(), node, leaseID, att, ep)
	l := *recvType(t, c, "lease", 5*time.Second).Body.(*protocol.Lease)
	return &leaseRun{t: t, e: e, c: c, l: l, done: done, token: l.Token}
}

func (r *leaseRun) log(text string) {
	r.t.Helper()
	r.seq++
	if err := r.c.Send(logMsg(r.l, r.seq, text)); err != nil {
		r.t.Fatal(err)
	}
}

func (r *leaseRun) beat() {
	r.t.Helper()
	if err := r.c.Send(beatMsg(r.l)); err != nil {
		r.t.Fatal(err)
	}
}

func (r *leaseRun) finish() {
	r.t.Helper()
	if err := r.c.Send(resultMsg(r.l)); err != nil {
		r.t.Fatal(err)
	}
	select {
	case err := <-r.done:
		if err != nil {
			r.t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		r.t.Fatal("Run did not return after the result")
	}
}

// wantLines asserts the run saw exactly lines "line-1" .. "line-n", each once, in order.
func wantLines(t *testing.T, s *runSink, n int) {
	t.Helper()
	eventually(t, 10*time.Second, fmt.Sprintf("%d log lines", n), func() bool { return len(s.lines()) >= n })
	time.Sleep(100 * time.Millisecond) // let a duplicate show up
	got := s.lines()
	if len(got) != n {
		t.Fatalf("%d lines reached the run, want %d: %v", len(got), n, got)
	}
	for i, g := range got {
		if g != fmt.Sprintf("line-%d", i+1) {
			t.Fatalf("line %d is %q: %v", i+1, g, got)
		}
	}
}

// scenarioPartition: a partition while the job logs and finishes; everything arrives once after healing.
func scenarioPartition(t *testing.T, partitionFor time.Duration) {
	e := newE2E(t, "")
	e.addNode("n-1")
	px := newNetProxy(t, e.ln.Addr().String())
	e.base = "https://" + px.addr()
	c := e.carrier(e.nodeCert("n-1"), nil)
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, c, "welcome", 5*time.Second)
	run := e.startLease(t, c, "n-1", "1")
	for i := 1; i <= 5; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	run.beat()
	recvType(t, c, "heartbeat", 5*time.Second)
	wantLines(t, e.sink, 5)

	px.partition()
	for i := 6; i <= 15; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	run.beat()
	time.Sleep(partitionFor)
	px.heal()
	for i := 16; i <= 20; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	wantLines(t, e.sink, 20)
	run.finish()
	if n := e.sink.nResults(); n != 1 {
		t.Fatalf("%d results reached the run, want 1", n)
	}
}

// scenarioRestart: the coordinator is killed mid-stream and comes back with no memory of the session; the agent
// says hello again, resends what was not acknowledged, and no line is duplicated or lost.
func scenarioRestart(t *testing.T) {
	e := newE2E(t, "")
	e.addNode("n-1")
	addr := e.ln.Addr().String()
	c := e.carrier(e.nodeCert("n-1"), nil)
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, c, "welcome", 5*time.Second)
	run := e.startLease(t, c, "n-1", "1")
	for i := 1; i <= 8; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	run.beat()
	recvType(t, c, "heartbeat", 5*time.Second)
	cl, err := e.tokens.Verify(run.token)
	if err != nil {
		t.Fatal(err)
	}
	before, found, _ := e.st.TokenCheck(context.Background(), cl.JTI)
	if !found {
		t.Fatal("the token was never bound")
	}
	for i := 9; i <= 12; i++ { // sent after the last ack: the agent still holds them
		run.log(fmt.Sprintf("line-%d", i))
	}
	e.kill()
	for i := 13; i <= 16; i++ {
		run.log(fmt.Sprintf("line-%d", i))
	}
	e.start(addr) // a new listener: every session of the old one is gone
	wantLines(t, e.sink, 16)
	after, _, _ := e.st.TokenCheck(context.Background(), cl.JTI)
	if after.Session == before.Session || after.Serial != before.Serial {
		t.Fatalf("the token must move to the new session of the same certificate: before %+v after %+v", before, after)
	}
	run.finish()
	if n := e.sink.nResults(); n != 1 {
		t.Fatalf("%d results reached the run, want 1", n)
	}
	b := readLog(t, LogFile(e.drv.LogDir, run.l.LeaseID))
	if strings.Count(b, "\n") != 16 {
		t.Fatalf("the durable log has %d lines, want 16:\n%s", strings.Count(b, "\n"), b)
	}
}
