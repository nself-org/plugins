package agent

// Purpose: loopback tests of the pull carrier against the real agentapi listener: a lease delivered by the
//          driver reaches the agent, its logs, heartbeats and result reach the run, and the token is carried and
//          bound.
// Inputs:  the e2e rig. Constraints: bounded waits; every result is checked on the run's sink.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// runLease starts Driver.Run for the seeded lease and returns its result channel.
func (e *e2e) runLease(ctx context.Context, node, leaseID, attemptID string, epoch uint64) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- e.drv.Run(ctx, e.capability(node), protoLease(leaseID, attemptID, epoch), e.sink) }()
	return ch
}

func (e *e2e) connect(node string) *Carrier {
	e.t.Helper()
	e.addNode(node)
	c := e.carrier(e.nodeCert(node), nil)
	if err := c.Send(e.hello()); err != nil {
		e.t.Fatal(err)
	}
	if m := recv(e.t, c, 5*time.Second); m.Type != "welcome" {
		e.t.Fatalf("first message %s, want welcome", m.Type)
	}
	return c
}

func logMsg(l protocol.Lease, seq uint64, text string) protocol.Message {
	ep := l.Epoch
	return protocol.Message{Type: "log", LeaseID: l.LeaseID, Epoch: &ep, Body: protocol.Log{LeaseID: l.LeaseID, LineSeq: seq, Stream: "stdout", Text: text}}
}

func resultMsg(l protocol.Lease) protocol.Message {
	ep := l.Epoch
	return protocol.Message{Type: "result", LeaseID: l.LeaseID, Epoch: &ep, Body: protocol.Result{LeaseID: l.LeaseID, Epoch: l.Epoch, Evidence: []byte(`{}`), Signature: protocol.Signature{KeyID: "k", Value: "v"}}}
}

func beatMsg(l protocol.Lease) protocol.Message {
	return protocol.Message{Type: "heartbeat", Body: protocol.Heartbeat{Leases: []protocol.LeaseState{{LeaseID: l.LeaseID, Epoch: l.Epoch, State: "running"}}, Slots: protocol.Slots{Total: 2, Used: 1}}}
}

func TestPullDeliversALeaseAndItsOutput(t *testing.T) {
	e := newE2E(t, "")
	leaseID, att, ep := e.seedLease("1", "n-1")
	c := e.connect("n-1")
	done := e.runLease(context.Background(), "n-1", leaseID, att, ep)

	lm := recvType(t, c, "lease", 5*time.Second)
	l := *lm.Body.(*protocol.Lease)
	if l.LeaseID != leaseID || l.Epoch != ep || l.Token == "" {
		t.Fatalf("lease = %+v", l)
	}
	cl, err := e.tokens.Verify(l.Token)
	if err != nil || cl.RunnerID != "n-1" || cl.AttemptID != att || cl.Epoch != int64(ep) {
		t.Fatalf("token claims %+v err %v", cl, err)
	}
	for i, txt := range []string{"one", "two", "three"} {
		if err = c.Send(logMsg(l, uint64(i+1), txt)); err != nil {
			t.Fatal(err)
		}
	}
	if err = c.Send(beatMsg(l)); err != nil {
		t.Fatal(err)
	}
	hb := recvType(t, c, "heartbeat", 5*time.Second)
	if hb.Ack == nil {
		t.Fatal("the heartbeat answer carries no ack")
	}
	eventually(t, 5*time.Second, "three log lines", func() bool { return len(e.sink.lines()) == 3 })
	if got := e.sink.lines(); got[0] != "one" || got[1] != "two" || got[2] != "three" {
		t.Fatalf("lines = %v", got)
	}
	if err = c.Send(resultMsg(l)); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the result")
	}
	if e.sink.nResults() != 1 {
		t.Fatalf("%d results reached the run", e.sink.nResults())
	}
	b, found, _ := e.st.TokenCheck(context.Background(), cl.JTI)
	if !found || !b.Spent || b.NodeID != "n-1" {
		t.Fatalf("binding after the result: %+v found=%v", b, found)
	}
	state, _, _ := e.st.AttemptStateEpoch(context.Background(), att)
	if state == "running" {
		t.Fatal("the lease was never committed to the engine")
	}
}
