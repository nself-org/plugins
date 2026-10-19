package ssh

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func runScripted(t *testing.T, d *Driver, script func(a *scriptedAgent), sink transport.Sink) error {
	t.Helper()
	node, home := testNode(t)
	d.Home = home
	d.Start = scriptedStarter(t, script)
	d.NoRenew = d.Renew == nil
	d.SSHVersion = newDriver(t, home, newAgentRig(t, okRunner()), nil).SSHVersion
	return d.Run(context.Background(), node, testLease(), sink)
}

func heartbeatOf(state string) protocol.Heartbeat {
	return protocol.Heartbeat{Leases: []protocol.LeaseState{{LeaseID: "lease-1", Epoch: 4, State: state}}}
}

func reasonOf(t *testing.T, err error, want string) {
	t.Helper()
	if Reason(err) != want {
		t.Fatalf("err = %v, want reason %s", err, want)
	}
}

// TestSSHTransportRefusesToRunWithoutLeaseRenewal proves a driver nobody wired to the lease
// engine does not start a job whose lease would silently expire.
func TestSSHTransportRefusesToRunWithoutLeaseRenewal(t *testing.T) {
	node, home := testNode(t)
	d := &Driver{Home: home}
	reasonOf(t, d.Run(context.Background(), node, testLease(), &recSink{}), ReasonNotConfigured)
}

// TestSSHTransportRenewFailureFencesAndCancels proves a lease that cannot be renewed ends
// the attempt as fenced (E662) and tells the agent to cancel.
func TestSSHTransportRenewFailureFencesAndCancels(t *testing.T) {
	cancelled := make(chan struct{}, 1)
	d := &Driver{Renew: func(context.Context, string, uint64) error { return errors.New("lease gone") }}
	err := runScripted(t, d, func(a *scriptedAgent) {
		a.send("heartbeat", heartbeatOf("running"), "", 0)
		if _, ok := a.next("cancel"); ok {
			cancelled <- struct{}{}
		}
	}, &recSink{})
	reasonOf(t, err, ReasonFenced)
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent was never told to cancel")
	}
}

// TestSSHTransportAnswersEveryHeartbeatWithItsAck proves the reply names the agent's seq,
// which is what the agent's fence waits for, and renews the lease.
func TestSSHTransportAnswersEveryHeartbeatWithItsAck(t *testing.T) {
	renewed := make(chan uint64, 4)
	d := &Driver{Renew: func(_ context.Context, id string, epoch uint64) error {
		if id == "lease-1" {
			renewed <- epoch
		}
		return nil
	}}
	var ackOf, sent uint64
	err := runScripted(t, d, func(a *scriptedAgent) {
		a.send("heartbeat", heartbeatOf("running"), "", 0)
		sent = a.seq
		m, ok := a.next("heartbeat")
		if ok && m.Ack != nil {
			ackOf = *m.Ack
		}
		a.send("result", protocol.Result{LeaseID: "lease-1", Epoch: 4, Evidence: evidence()}, "lease-1", 4)
		a.next("ack")
	}, &recSink{})
	if err != nil {
		t.Fatal(err)
	}
	if ackOf == 0 || ackOf != sent {
		t.Fatalf("heartbeat reply acked %d, the agent sent seq %d", ackOf, sent)
	}
	if len(renewed) != 1 || <-renewed != 4 {
		t.Fatal("the lease was not renewed once at epoch 4")
	}
}

// TestSSHTransportMapsRefusedAndSelfFencedLeases proves the agent's own verdicts end the attempt.
func TestSSHTransportMapsRefusedAndSelfFencedLeases(t *testing.T) {
	for state, reason := range map[string]string{"refused": ReasonRefused, "self_fenced": ReasonFenced} {
		t.Run(state, func(t *testing.T) {
			err := runScripted(t, &Driver{}, func(a *scriptedAgent) {
				a.send("heartbeat", heartbeatOf(state), "", 0)
				a.next("cancel")
			}, &recSink{})
			reasonOf(t, err, reason)
		})
	}
}

// TestSSHTransportRejectsAResultOfAnotherEpoch proves a stale result never reaches the sink.
func TestSSHTransportRejectsAResultOfAnotherEpoch(t *testing.T) {
	sink := &recSink{}
	err := runScripted(t, &Driver{}, func(a *scriptedAgent) {
		a.send("result", protocol.Result{LeaseID: "lease-1", Epoch: 3, Evidence: evidence()}, "lease-1", 3)
		a.next("cancel")
	}, sink)
	reasonOf(t, err, ReasonFenced)
	if len(sink.results) != 0 {
		t.Fatal("a stale-epoch result reached the sink")
	}
}

// TestSSHTransportDropsBlobFramesOfAnotherAttempt proves digest and chunk frames outside this
// lease and epoch never open a blob.
func TestSSHTransportDropsBlobFramesOfAnotherAttempt(t *testing.T) {
	sink := &recSink{}
	err := runScripted(t, &Driver{}, func(a *scriptedAgent) {
		d := protocol.Digest{Stream: "cache", SHA256: "0000000000000000000000000000000000000000000000000000000000000000", Size: 4}
		a.send("digest", d, "lease-1", 3)
		a.send("digest", d, "other-lease", 4)
		a.send("result", protocol.Result{LeaseID: "lease-1", Epoch: 4, Evidence: evidence()}, "lease-1", 4)
		a.next("ack")
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.blobs) != 0 {
		t.Fatalf("a stale frame opened a blob: %v", sink.blobs)
	}
}

// TestSSHTransportRefusesBadBlobAnnouncements proves the agent cannot name a path-like digest
// or lift the size bound with size 0.
func TestSSHTransportRefusesBadBlobAnnouncements(t *testing.T) {
	good := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for name, d := range map[string]protocol.Digest{
		"path as digest": {Stream: "cache", SHA256: "../../etc/passwd", Size: 4},
		"size zero":      {Stream: "cache", SHA256: good, Size: 0},
		"huge":           {Stream: "cache", SHA256: good, Size: maxBlob + 1},
	} {
		t.Run(name, func(t *testing.T) {
			sink := &recSink{}
			err := runScripted(t, &Driver{}, func(a *scriptedAgent) {
				a.send("digest", d, "lease-1", 4)
				a.next("cancel")
			}, sink)
			reasonOf(t, err, ReasonBlob)
			if len(sink.blobs) != 0 {
				t.Fatal("a refused announcement opened a blob")
			}
		})
	}
}

// noAbortSink hands out writers that cannot discard what they hold.
type noAbortSink struct{ *recSink }

type plainWriter struct{ *memBlob }

func (noAbortSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return plainWriter{&memBlob{}}, nil
}

func (plainWriter) Abort() {} // wrong signature on purpose: not an Aborter

// TestSSHTransportRefusesASinkThatCannotDiscard proves a failed blob can never be committed by Close.
func TestSSHTransportRefusesASinkThatCannotDiscard(t *testing.T) {
	err := runScripted(t, &Driver{}, func(a *scriptedAgent) { sendBlob(a, "cache", []byte("payload"), false) }, noAbortSink{&recSink{}})
	reasonOf(t, err, ReasonBlob)
}

// TestSSHTransportFlippedByteInAResumedBlobIsRejected proves a resumed blob is verified whole.
func TestSSHTransportFlippedByteInAResumedBlobIsRejected(t *testing.T) {
	payload := []byte("0123456789abcdef")
	sink := &recSink{startAt: 10, prefix: payload[:10]}
	err := runScripted(t, &Driver{}, func(a *scriptedAgent) { sendBlob(a, "artifact", payload, true) }, sink)
	reasonOf(t, err, ReasonBlob)
	if b := sink.blobs["artifact"]; b == nil || !b.abort || b.closed {
		t.Fatalf("the corrupt resumed blob was not discarded: %+v", b)
	}
}

// TestSSHTransportDiscardsABlobLeftHalfWritten proves a session that ends mid-blob aborts the writer.
func TestSSHTransportDiscardsABlobLeftHalfWritten(t *testing.T) {
	sink := &recSink{}
	good := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	err := runScripted(t, &Driver{}, func(a *scriptedAgent) {
		a.send("digest", protocol.Digest{Stream: "cache", SHA256: good, Size: 8}, "lease-1", 4)
		a.next("offset")
		a.send("chunk", protocol.Chunk{Stream: "cache", Offset: 0, Data: "AAAA", Digest: good}, "lease-1", 4)
		// the agent goes away here
	}, sink)
	if !IsLost(err) {
		t.Fatalf("err = %v, want session lost", err)
	}
	if b := sink.blobs["cache"]; b == nil || !b.abort {
		t.Fatalf("the half-written blob was not aborted: %+v", b)
	}
}

// TestSSHTransportSecretsOnlyForAPersonalCoordinator proves the secrets frame and the
// secret-accepting agent command are used only when the coordinator is PERSONAL.
func TestSSHTransportSecretsOnlyForAPersonalCoordinator(t *testing.T) {
	secrets := func(context.Context, protocol.Lease) ([]protocol.SecretValue, error) {
		return []protocol.SecretValue{{Ref: "project/T", Value: "v"}}, nil
	}
	l := testLease()
	l.Secrets = []protocol.SecretRef{{Ref: "project/T", Class: "project"}}
	for _, personal := range []bool{false, true} {
		node, home := testNode(t)
		a := newAgentRig(t, okRunner())
		var seen []string
		a.tap = func(c session.Carrier) session.Carrier { return &typeTap{Carrier: c, seen: &seen} }
		var got startArgs
		d := newDriver(t, home, a, &got)
		d.Personal, d.Secrets = personal, secrets
		if err := d.Run(context.Background(), node, l, &recSink{}); err != nil {
			t.Fatalf("personal=%v: %v", personal, err)
		}
		hasSecrets := false
		for _, k := range seen {
			hasSecrets = hasSecrets || k == "secrets"
		}
		wantCmd := agentCommand
		if personal {
			wantCmd = agentCommandSecret
		}
		if hasSecrets != personal || got.command != wantCmd {
			t.Errorf("personal=%v: secrets frame %v, command %q", personal, hasSecrets, got.command)
		}
	}
}

// typeTap records the type of every frame the agent receives.
type typeTap struct {
	session.Carrier
	seen *[]string
}

func (c *typeTap) Recv() (protocol.Message, error) {
	m, err := c.Carrier.Recv()
	if err == nil {
		*c.seen = append(*c.seen, m.Type)
	}
	return m, err
}
