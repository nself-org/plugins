package ssh

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func (b *memBlob) Abort() error { b.abort = true; return nil }

// scriptedAgent is a node whose agent end is a test script on the real stream carrier.
type scriptedAgent struct {
	t   *testing.T
	car session.Carrier
	seq uint64
}

func (a *scriptedAgent) send(kind string, body any, lease string, epoch uint64) {
	a.t.Helper()
	a.seq++
	m := protocol.Message{V: 1, Type: kind, Seq: a.seq, Body: body}
	if lease != "" {
		m.LeaseID, m.Epoch = lease, &epoch
	}
	if err := a.car.Send(m); err != nil {
		a.t.Errorf("agent send %s: %v", kind, err)
	}
}

// next returns the coordinator's next frame of the wanted type, or false at EOF.
func (a *scriptedAgent) next(want string) (protocol.Message, bool) {
	for {
		m, err := a.car.Recv()
		if err != nil {
			return m, false
		}
		if m.Type == want {
			return m, true
		}
	}
}

// scriptedStarter returns a Starter whose "ssh" runs script as the agent.
func scriptedStarter(t *testing.T, script func(a *scriptedAgent)) Starter {
	return func(ctx context.Context, _ remote.Target, _ string) (*Conn, error) {
		cr, cw := io.Pipe()
		ar, aw := io.Pipe()
		er, ew := io.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { _ = ew.Close(); _ = aw.Close() }()
			car, err := session.NewStreamCarrier(cr, aw, protocol.CarrierPolicy{Carrier: "A"}, true)
			if err != nil {
				t.Errorf("agent carrier: %v", err)
				return
			}
			defer func() { _ = car.Close() }()
			a := &scriptedAgent{t: t, car: car}
			a.send("hello", protocol.Hello{Versions: []int{1}, AgentVersion: "1.5.0",
				NodeKey:             protocol.NodeKey{Alg: "ed25519", KeyID: "k1", Public: "pub"},
				Identity:            protocol.Identity{MachineID: "m1", HostKeyFingerprints: []string{}},
				CompatModeSupported: []protocol.CompatMode{protocol.Compat15}, Slots: protocol.Slots{Total: 1}}, "", 0)
			if _, ok := a.next("welcome"); !ok {
				return
			}
			script(a)
		}()
		return &Conn{Stdin: cw, Stdout: ar, Stderr: er, Wait: func() error { <-done; return nil },
			Kill: func() error { _ = cr.Close(); _ = aw.Close(); return nil }}, nil
	}
}

// sendBlob announces payload on stream, sends it from the offset the coordinator
// reports (flip changes one byte on the wire), and then delivers the result.
func sendBlob(a *scriptedAgent, stream string, payload []byte, flip bool) {
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	a.send("digest", protocol.Digest{Stream: stream, SHA256: digest, Size: int64(len(payload))}, "lease-1", 4)
	m, ok := a.next("offset")
	if !ok {
		return
	}
	off := m.Body.(*protocol.Offset).Offset
	wire := append([]byte(nil), payload[off:]...)
	if flip {
		wire[0] ^= 0xff
	}
	a.send("chunk", protocol.Chunk{Stream: stream, Offset: off, Data: base64.StdEncoding.EncodeToString(wire), Digest: digest, Final: true}, "lease-1", 4)
	a.send("result", protocol.Result{LeaseID: "lease-1", Epoch: 4, Evidence: evidence()}, "lease-1", 4)
	a.next("ack")
}

// TestSSHTransportBlobWithAFlippedByteIsRejectedByDigest proves CACHE delta 5 over this carrier.
func TestSSHTransportBlobWithAFlippedByteIsRejectedByDigest(t *testing.T) {
	node, home := testNode(t)
	d := newDriver(t, home, newAgentRig(t, okRunner()), nil)
	d.Start = scriptedStarter(t, func(a *scriptedAgent) { sendBlob(a, "cache", []byte("cache payload bytes"), true) })
	sink := &recSink{}
	err := d.Run(context.Background(), node, testLease(), sink)
	if Reason(err) != ReasonBlob {
		t.Fatalf("err = %v, want a blob rejection", err)
	}
	b := sink.blobs["cache"]
	if b == nil || !b.abort || b.closed {
		t.Fatalf("the corrupt blob was not discarded: %+v", b)
	}
	if len(sink.results) != 0 {
		t.Fatal("a result was accepted after a corrupt blob")
	}
}

// TestSSHTransportInterruptedBlobResumesFromTheReportedOffset proves the sink's
// offset steers the agent: only the missing tail crosses the carrier.
func TestSSHTransportInterruptedBlobResumesFromTheReportedOffset(t *testing.T) {
	node, home := testNode(t)
	d := newDriver(t, home, newAgentRig(t, okRunner()), nil)
	payload := []byte("0123456789abcdef")
	d.Start = scriptedStarter(t, func(a *scriptedAgent) { sendBlob(a, "artifact", payload, false) })
	sink := &recSink{startAt: 10}
	if err := d.Run(context.Background(), node, testLease(), sink); err != nil {
		t.Fatal(err)
	}
	b := sink.blobs["artifact"]
	if b == nil || string(b.data) != "abcdef" || !b.closed || b.Offset() != int64(len(payload)) {
		t.Fatalf("resumed blob = %+v", b)
	}
}
