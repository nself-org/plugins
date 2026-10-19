package session

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// TestResultSignatureVerifiesOverTheWireBytes proves the signature covers the
// evidence exactly as the coordinator receives it: a real node key signs, the
// frame goes through the protocol encoder and decoder, and ed25519 verifies the
// decoded evidence bytes. The evidence has unsorted keys, HTML characters and a
// large number, which the encoder rewrites.
func TestResultSignatureVerifiesOverTheWireBytes(t *testing.T) {
	key, err := nodekey.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ev := `{"z":1,"excerpt":"a<b && c>d","big":12345678901234567890,"a":{"y":2,"x":[3,1]}}`
	r := startRig(t, okRunner(ev), func(c *Config) { c.Signer = key })
	r.handshake()
	r.lease(testLease("l1", 1))
	res := r.car.next(t, "result")
	var wire bytes.Buffer
	enc := protocol.NewEncoder(&wire)
	res.Seq = 1
	if err := enc.Encode(res); err != nil {
		t.Fatal(err)
	}
	dec, _ := protocol.NewResumedDecoder(&wire, testPolicy, 0, 0)
	got, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}
	body := got.Body.(*protocol.Result)
	sig, err := base64.RawStdEncoding.DecodeString(body.Signature.Value)
	if err != nil || body.Signature.KeyID != key.KeyID() {
		t.Fatalf("signature %+v: %v", body.Signature, err)
	}
	if !ed25519.Verify(key.Public(), body.Evidence, sig) {
		t.Fatalf("signature does not verify over the received evidence %s", body.Evidence)
	}
	if !strings.Contains(string(body.Evidence), "12345678901234567890") {
		t.Fatalf("large number altered: %s", body.Evidence)
	}
}

// TestCancelForAnotherEpochIsIgnored proves only the running epoch can be cancelled.
func TestCancelForAnotherEpochIsIgnored(t *testing.T) {
	started := make(chan struct{})
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		close(started)
		<-ctx.Done()
		return JobResult{Outcome: Outcome{State: "cancelled"}, Evidence: json.RawMessage(`{}`)}, nil
	})
	r := startRig(t, runner)
	r.handshake()
	r.lease(testLease("l1", 5))
	<-started
	r.car.pushFenced("cancel", protocol.Cancel{LeaseID: "l1", Reason: "future"}, 6, "l1")
	r.car.pushFenced("cancel", protocol.Cancel{LeaseID: "l1", Reason: "now"}, 5, "l1")
	ack := r.car.next(t, "ack").Body.(protocol.Ack)
	if ack.AckSeq != 4 { // welcome 1, lease 2, future cancel 3 (ignored), real cancel 4
		t.Fatalf("ack %+v", ack)
	}
}

// TestSecretsFramesAreFenced proves a secrets frame must carry this lease's
// exact epoch, only declared refs are taken, each once, and error text that
// echoes a secret is masked before it leaves the node.
func TestSecretsFramesAreFenced(t *testing.T) {
	got := make(chan map[string]string, 1)
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		got <- j.Setup.Secrets
		return JobResult{}, errors.New("runner blew up with DECLARED-VALUE and JOBTOKEN-xyz")
	})
	r := startRig(t, runner)
	r.handshake()
	l := testLease("l1", 3)
	l.Token = "JOBTOKEN-xyz"
	l.Job.Secrets = []string{"TOKEN"}
	l.Secrets = []protocol.SecretRef{{Ref: "project/TOKEN", Class: "project"}}
	r.lease(l)
	r.car.pushFenced("secrets", protocol.Secrets{LeaseID: "l1", Values: []protocol.SecretValue{{Ref: "project/TOKEN", Value: "WRONG-EPOCH"}}}, 2, "l1")
	r.car.pushFenced("secrets", protocol.Secrets{LeaseID: "l1", Values: []protocol.SecretValue{{Ref: "project/OTHER", Value: "UNDECLARED"}, {Ref: "project/TOKEN", Value: "DECLARED-VALUE"}}}, 3, "l1")
	r.car.pushFenced("secrets", protocol.Secrets{LeaseID: "l1", Values: []protocol.SecretValue{{Ref: "project/TOKEN", Value: "SECOND-FRAME"}}}, 3, "l1")
	secrets := <-got
	if secrets["TOKEN"] != "DECLARED-VALUE" || len(secrets) != 2 { // the ref and its short name
		t.Fatalf("secrets %v", secrets)
	}
	var errLog string
	for {
		m := <-r.car.out
		if lg, ok := m.Body.(protocol.Log); ok && strings.HasPrefix(lg.Text, "agent:") {
			errLog = lg.Text
		}
		if m.Type == "result" {
			break
		}
	}
	if strings.Contains(errLog, "DECLARED-VALUE") || strings.Contains(errLog, "JOBTOKEN") || !strings.Contains(errLog, "[redacted]") {
		t.Fatalf("error log %q", errLog)
	}
}

// TestExpiredLeaseIsRefused proves a lease past its expiry, or with an
// unparseable one, never starts and costs no slot.
func TestExpiredLeaseIsRefused(t *testing.T) {
	var started bool
	r := startRig(t, RunnerFunc(func(context.Context, JobRun) (JobResult, error) { started = true; return JobResult{}, nil }))
	r.handshake()
	old := testLease("l1", 1)
	old.ExpiresAt = "2026-10-08T18:00:00Z" // before the manual clock's 19:00
	bad := testLease("l2", 1)
	bad.ExpiresAt = "soon"
	r.lease(old)
	r.lease(bad)
	var reasons []string
	for len(reasons) < 2 {
		m := <-r.car.out
		if hb, ok := m.Body.(protocol.Heartbeat); ok {
			for _, l := range hb.Leases {
				if l.State == "refused" {
					reasons = append(reasons, l.LeaseID)
				}
			}
		}
	}
	if started {
		t.Fatal("an expired lease ran")
	}
	if n, _ := r.table.Used(); n != 0 {
		t.Fatalf("expired lease holds %d slots", n)
	}
}

// TestJobRootsAreUnique proves two runs with one lease id (two coordinators)
// get separate directories.
func TestJobRootsAreUnique(t *testing.T) {
	state := t.TempDir()
	l := protocol.Lease{LeaseID: "l/1", Epoch: 1}
	a, err := newJobRoot(state, l)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newJobRoot(state, l)
	if err != nil || a.Dir == b.Dir {
		t.Fatalf("roots %q %q %v", a.Dir, b.Dir, err)
	}
}

type stuckCarrier struct {
	*fakeCarrier
	release chan struct{}
	once    sync.Once
}

func (c *stuckCarrier) fake() *fakeCarrier { return c.fakeCarrier }

func (c *stuckCarrier) Send(m protocol.Message) error {
	if m.Type == "result" {
		<-c.release // the peer has stopped reading
		return io.ErrClosedPipe
	}
	return c.fakeCarrier.Send(m)
}
func (c *stuckCarrier) Close() error {
	c.once.Do(func() { close(c.release) })
	return c.fakeCarrier.Close()
}

// TestShutdownDoesNotHangOnAStalledPeer proves Run returns even when a result
// write is blocked because the coordinator stopped reading.
func TestShutdownDoesNotHangOnAStalledPeer(t *testing.T) {
	saved := shutdownGrace
	shutdownGrace = 200 * time.Millisecond
	t.Cleanup(func() { shutdownGrace = saved })
	st := &stuckCarrier{fakeCarrier: newFakeCarrier(), release: make(chan struct{})}
	started := make(chan struct{})
	state := t.TempDir()
	r := startRigWith(t, state, st, RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		close(started)
		<-ctx.Done()
		return JobResult{Outcome: Outcome{State: "cancelled"}, Evidence: json.RawMessage(`{}`)}, nil
	}))
	r.handshake()
	r.lease(testLease("l1", 1))
	<-started
	r.cancel()
	if err := r.wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
