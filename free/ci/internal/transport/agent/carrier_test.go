package agent

// Purpose: tests of the pull carrier's own behaviour against the real listener: frame validation, Close and
//          flush, back-pressure, a new transport session after the coordinator forgot the old one (with the
//          kept leases reconciled and the welcome delivered once), and a hello the coordinator rejects.
// Inputs:  the e2e rig. Constraints: bounded waits; timings shortened through the package variables.

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestSendRefusesAFrameTheCodecRefuses(t *testing.T) {
	e := newE2E(t, "")
	c := e.connect("n-1")
	big := protocol.Log{LeaseID: "l1", LineSeq: 1, Stream: "stdout", Text: strings.Repeat("x", protocol.MaxFrame)}
	ep := uint64(1)
	err := c.Send(protocol.Message{Type: "log", LeaseID: "l1", Epoch: &ep, Body: big})
	var pe *protocol.Error
	if !errors.As(err, &pe) || pe.Reason != "frame_too_large" {
		t.Fatalf("err = %v", err)
	}
	c.mu.Lock()
	n := len(c.out)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("a refused frame was queued (%d)", n)
	}
}

func TestSendAfterCloseAndDoubleClose(t *testing.T) {
	e := newE2E(t, "")
	c := e.connect("n-1")
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := c.Send(protocol.Message{Type: "heartbeat", Body: protocol.Heartbeat{Leases: []protocol.LeaseState{}}}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("send after close = %v", err)
	}
	if _, err := c.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("recv after close = %v", err)
	}
}

func TestSendStallsWhenTheCoordinatorTakesNothing(t *testing.T) {
	old := sendTimeout
	sendTimeout = 150 * time.Millisecond
	defer func() { sendTimeout = old }()
	e := newE2E(t, "")
	e.addNode("n-1")
	e.base = "https://127.0.0.1:1" // nothing listens: the carrier never connects
	c := e.carrier(e.nodeCert("n-1"), nil)
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	ep := uint64(1)
	text := strings.Repeat("y", protocol.MaxFrame-300)
	var stalled error
	for i := 1; i <= 20 && stalled == nil; i++ {
		stalled = c.Send(protocol.Message{Type: "log", LeaseID: "l1", Epoch: &ep, Body: protocol.Log{LeaseID: "l1", LineSeq: uint64(i), Stream: "stdout", Text: text}})
	}
	if !errors.Is(stalled, errStalled) {
		t.Fatalf("err = %v, want the stall after about %d MiB", stalled, maxOutBytes>>20)
	}
}

func TestSendUnblocksWhenFramesAreConfirmed(t *testing.T) {
	e := newE2E(t, "")
	c := e.connect("n-1")
	c.mu.Lock()
	c.out = append(c.out, &outFrame{tseq: 1, size: maxOutBytes}) // a full buffer
	c.outBytes = maxOutBytes
	c.nextT = 1
	c.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		done <- c.Send(protocol.Message{Type: "heartbeat", Body: protocol.Heartbeat{Leases: []protocol.LeaseState{}}})
	}()
	select {
	case err := <-done:
		t.Fatalf("Send did not wait: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	c.mu.Lock()
	c.trim(1)
	c.mu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Send stayed blocked after the buffer drained")
	}
}

func TestCloseDeliversTheResultFirst(t *testing.T) {
	e := newE2E(t, "")
	c := e.connect("n-1")
	run := e.startLease(t, c, "n-1", "1")
	run.log("line-1")
	if err := c.Send(resultMsg(run.l)); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-run.done:
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the result written just before Close never arrived")
	}
}

func TestANewTransportSessionKeepsTheLeaseAndDeliversTheWelcomeOnce(t *testing.T) {
	for _, withBeat := range []bool{false, true} {
		t.Run(fmt.Sprintf("heartbeat sent=%v", withBeat), func(t *testing.T) {
			e := newE2E(t, "")
			c := e.connect("n-1")
			run := e.startLease(t, c, "n-1", "1")
			run.log("line-1")
			if withBeat {
				run.beat()
				recvType(t, c, "heartbeat", 5*time.Second)
			}
			wantLines(t, e.sink, 1)
			before, _, _ := e.st.TokenCheck(ctxBG(), mustJTI(t, e, run.token))
			e.srv.CloseNode("n-1") // the coordinator forgets the session and closes every connection
			run.log("line-2")
			run.log("line-3")
			eventually(t, 10*time.Second, "lines 2 and 3", func() bool { return len(e.sink.lines()) == 3 })
			after, _, _ := e.st.TokenCheck(ctxBG(), mustJTI(t, e, run.token))
			if after.Session == before.Session {
				t.Fatal("the agent did not start a new transport session")
			}
			// no second welcome and no cancel reached the session
			deadline := time.After(300 * time.Millisecond)
			for {
				select {
				case m := <-c.inbox:
					if m.Type == "welcome" || m.Type == "cancel" {
						t.Fatalf("unexpected %s after a new transport session", m.Type)
					}
					continue
				case <-deadline:
				}
				break
			}
			run.finish()
		})
	}
}

func TestAHelloTheCoordinatorRejectsIsDeliveredAndEndsTheCarrier(t *testing.T) {
	e := newE2E(t, "")
	e.addNode("n-1")
	cert := e.nodeCert("n-1")
	first := e.carrier(cert, nil)
	if err := first.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, first, "welcome", 5*time.Second) // pins the node key
	e.pub, _, _ = ed25519.GenerateKey(rand.Reader)
	second := e.carrier(cert, nil)
	if err := second.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	m := recv(t, second, 5*time.Second)
	rj, ok := m.Body.(*protocol.Reject)
	if !ok || rj.Code != pki.CodeInvalid || rj.Reason != pki.ReasonNodeKeyChanged {
		t.Fatalf("first message = %s %+v", m.Type, m.Body)
	}
	if _, err := second.Recv(); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("after the reject Recv = %v, want the carrier's own error", err)
	}
	// the loop has ended: a frame sent now would sit in the buffer for ever, so Send says so
	if err := second.Send(logFrame("l1", 1)); err == nil {
		t.Fatal("Send queued a frame on a carrier whose loop ended")
	}
}

func TestOnlyTheFirstHelloIsAnswered(t *testing.T) {
	e := newE2E(t, "")
	e.addNode("n-1")
	c := e.carrier(e.nodeCert("n-1"), nil)
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, c, "welcome", 5*time.Second)
	c.mu.Lock()
	h := c.hello
	sess := c.sess
	c.mu.Unlock()
	if len(h) == 0 || sess == "" {
		t.Fatal("hello or session not kept")
	}
}
