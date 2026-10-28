package ssh

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/model"
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

// TestSSHTransportBoundsTheBlobsOneLeaseMayOpen proves a node cannot fill the sink with many
// individually valid blobs.
func TestSSHTransportBoundsTheBlobsOneLeaseMayOpen(t *testing.T) {
	sink := &recSink{}
	err := runScripted(t, &Driver{}, func(a *scriptedAgent) {
		for i := range 5 { // four blobs of maxBlob fit the budget, the fifth does not
			d := protocol.Digest{Stream: "cache", SHA256: strings.Repeat(string(rune('a'+i)), 64), Size: maxBlob}
			a.send("digest", d, "lease-1", 4)
			if _, ok := a.next("offset"); !ok {
				return
			}
		}
		a.next("cancel")
	}, sink)
	reasonOf(t, err, ReasonBlob)
}

// TestSSHTransportNeverSendsSecretsToAnUnverifiedNode proves a PERSONAL coordinator refuses,
// before ssh starts, to hand secret values to a node that is not operator-owned and uid-separated.
func TestSSHTransportNeverSendsSecretsToAnUnverifiedNode(t *testing.T) {
	for name, mut := range map[string]func(*model.Capability){
		"ownership unknown": func(n *model.Capability) { n.Identity.Ownership.Value = nil },
		"hosted":            func(n *model.Capability) { h := "hosted"; n.Identity.Ownership.Value = &h },
		"not eligible":      func(n *model.Capability) { n.SecretsEligible = false },
	} {
		t.Run(name, func(t *testing.T) {
			node, home := testNode(t)
			mut(&node)
			started := false
			d := &Driver{Home: home, NoRenew: true, Personal: true, Start: func(context.Context, remote.Target, string) (*Conn, error) {
				started = true
				return nil, errors.New("must not start")
			}, Secrets: func(context.Context, protocol.Lease) ([]protocol.SecretValue, error) {
				t.Error("the secret source was consulted for an unverified node")
				return nil, nil
			}}
			l := testLease()
			l.Secrets = []protocol.SecretRef{{Ref: "project/T", Class: "project"}}
			reasonOf(t, d.Run(context.Background(), node, l, &recSink{}), ReasonSecretsRefused)
			if started {
				t.Fatal("ssh started for a node that may not get secrets")
			}
		})
	}
}

// TestSSHTransportLosesASilentAgent proves an agent that goes quiet after welcome cannot hold
// the session and the lease open: the attempt is lost and the agent is told to cancel.
func TestSSHTransportLosesASilentAgent(t *testing.T) {
	reason := make(chan string, 1)
	d := &Driver{silenceTTL: 120 * time.Millisecond}
	start := time.Now()
	err := runScripted(t, d, func(a *scriptedAgent) {
		if m, ok := a.next("cancel"); ok {
			reason <- m.Body.(*protocol.Cancel).Reason
		}
	}, &recSink{})
	if !IsLost(err) {
		t.Fatalf("err = %v, want session lost", err)
	}
	if took := time.Since(start); took < 100*time.Millisecond || took > 5*time.Second {
		t.Fatalf("lost after %v, want about the 120 ms coordinator TTL", took)
	}
	select {
	case r := <-reason:
		if r != "agent_silent" {
			t.Fatalf("cancel reason %q", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the silent agent was not told to cancel")
	}
}

// TestSSHTransportOnlyAHeartbeatNamingTheLeaseRenewsIt proves an empty heartbeat is answered
// (the agent fence needs the ack) but renews nothing.
func TestSSHTransportOnlyAHeartbeatNamingTheLeaseRenewsIt(t *testing.T) {
	var renewals int
	var acked bool
	d := &Driver{Renew: func(context.Context, string, uint64) error { renewals++; return nil }}
	err := runScripted(t, d, func(a *scriptedAgent) {
		a.send("heartbeat", protocol.Heartbeat{Leases: []protocol.LeaseState{}}, "", 0)
		m, ok := a.next("heartbeat")
		acked = ok && m.Ack != nil
		a.send("result", protocol.Result{LeaseID: "lease-1", Epoch: 4, Evidence: evidence()}, "lease-1", 4)
		a.next("ack")
	}, &recSink{})
	if err != nil {
		t.Fatal(err)
	}
	if !acked || renewals != 0 {
		t.Fatalf("acked=%v renewals=%d, want an ack and no renewal", acked, renewals)
	}
}

// TestSSHTransportRefusesNodeIDsThatDifferOnlyByCase proves the pin lookup cannot be fooled by
// ssh's case-insensitive HostKeyAlias matching.
func TestSSHTransportRefusesNodeIDsThatDifferOnlyByCase(t *testing.T) {
	node, home := testNode(t)
	node.Identity.ID = "Node1"
	d := &Driver{Home: home, NoRenew: true}
	reasonOf(t, d.Run(context.Background(), node, testLease(), &recSink{}), ReasonNoDestination)
}

// TestSSHTransportDestinationBracketsIPv6 proves an IPv6 node reaches ParseHostSpec in the form it accepts.
func TestSSHTransportDestinationBracketsIPv6(t *testing.T) {
	var n model.Capability
	n.Identity.SSH = &model.CapabilitySSH{Hostname: "2001:db8::5", Port: 2222, User: "ci"}
	dest, ok := destination(n)
	if !ok {
		t.Fatal("no destination")
	}
	if _, err := remote.ParseHostSpec(dest); err != nil {
		t.Fatalf("%q: %v", dest, err)
	}
}

func blobScript(payload []byte, withUpload bool, refSHA string) func(a *scriptedAgent) {
	return func(a *scriptedAgent) {
		sum := sha256.Sum256(payload)
		digest := hex.EncodeToString(sum[:])
		a.send("digest", protocol.Digest{Stream: "artifact", SHA256: digest, Size: int64(len(payload))}, "lease-1", 4)
		if _, ok := a.next("offset"); !ok { // the coordinator answers a digest either way
			return
		}
		if withUpload {
			a.send("chunk", protocol.Chunk{Stream: "artifact", Offset: 0, Data: base64.StdEncoding.EncodeToString(payload), Digest: digest, Final: true}, "lease-1", 4)
		}
		if refSHA == "" {
			refSHA = digest
		}
		a.send("artifact-ref", protocol.ArtifactRef{LeaseID: "lease-1", Path: "out.bin", SHA256: refSHA, Size: int64(len(payload))}, "lease-1", 4)
		a.send("result", protocol.Result{LeaseID: "lease-1", Epoch: 4, Evidence: evidence()}, "lease-1", 4)
		a.next("ack")
	}
}

// TestSSHTransportArtifactRefsNeedAReceivedBlob proves an agent cannot reference a blob it never sent.
func TestSSHTransportArtifactRefsNeedAReceivedBlob(t *testing.T) {
	payload := []byte("artifact bytes")
	sink := &recSink{}
	if err := runScripted(t, &Driver{}, blobScript(payload, true, ""), sink); err != nil {
		t.Fatalf("a ref to a received blob: %v", err)
	}
	if len(sink.artifacts) != 1 {
		t.Fatalf("artifact refs = %d", len(sink.artifacts))
	}
	sink = &recSink{}
	other := strings.Repeat("ab", 32)
	reasonOf(t, runScripted(t, &Driver{}, blobScript(payload, true, other), sink), ReasonBlob)
	if len(sink.artifacts) != 0 {
		t.Fatal("a ref to a blob that was never received reached the sink")
	}
}

// TestSSHTransportABlobTheSinkAlreadyHoldsIsVerifiedNotResent proves a dedup hit counts as received.
func TestSSHTransportABlobTheSinkAlreadyHoldsIsVerifiedNotResent(t *testing.T) {
	payload := []byte("already stored")
	sink := &recSink{startAt: int64(len(payload)), prefix: payload}
	if err := runScripted(t, &Driver{}, blobScript(payload, false, ""), sink); err != nil {
		t.Fatal(err)
	}
	if b := sink.blobs["artifact"]; b == nil || !b.closed || len(sink.artifacts) != 1 {
		t.Fatalf("dedup hit not committed: %+v, refs %d", b, len(sink.artifacts))
	}
}

// blockingSink hangs in Log until the context it was handed is cancelled.
type blockingSink struct {
	*recSink
	entered, released chan struct{}
}

func (b blockingSink) Log(ctx context.Context, _ protocol.Log) error {
	close(b.entered)
	<-ctx.Done()
	close(b.released)
	return ctx.Err()
}

// TestSSHTransportSilenceBoundHoldsWhileASinkCallbackBlocks proves the watchdog does not share
// a goroutine with the Sink: a hung Log callback is cancelled and the attempt is still lost.
func TestSSHTransportSilenceBoundHoldsWhileASinkCallbackBlocks(t *testing.T) {
	sink := blockingSink{recSink: &recSink{}, entered: make(chan struct{}), released: make(chan struct{})}
	d := &Driver{silenceTTL: 150 * time.Millisecond}
	err := runScripted(t, d, func(a *scriptedAgent) {
		a.send("log", protocol.Log{LeaseID: "lease-1", LineSeq: 1, Stream: "stdout", Text: "hello"}, "lease-1", 4)
		a.next("cancel")
	}, sink)
	if !IsLost(err) {
		t.Fatalf("err = %v, want session lost", err)
	}
	select {
	case <-sink.released:
	case <-time.After(5 * time.Second):
		t.Fatal("the blocked Sink callback was never cancelled")
	}
}
