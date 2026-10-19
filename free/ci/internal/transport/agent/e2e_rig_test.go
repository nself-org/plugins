package agent

// Purpose: the loopback rig of the pull carrier tests: the real agentapi listener over TLS 1.3 mTLS, the real
//          token gate, store, CA and lease engine, the Driver on the coordinator side and a Carrier as the agent.
// Inputs:  none. Outputs: helpers only.
// Constraints: fake synthetic values only; every listener, carrier and goroutine is closed by t.Cleanup; waits are
//          bounded and short.

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// runSink is the sink of the run that owns the lease.
type runSink struct {
	mu      sync.Mutex
	logs    []protocol.Log
	results []protocol.Result
	refs    []protocol.ArtifactRef
	failLog error
	eng     *lease.Engine // commits the result like the run's remote sink does
}

func (s *runSink) Log(_ context.Context, l protocol.Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failLog != nil {
		return s.failLog
	}
	s.logs = append(s.logs, l)
	return nil
}
func (s *runSink) Result(ctx context.Context, r protocol.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eng != nil {
		if err := s.eng.Result(ctx, r.LeaseID, int64(r.Epoch)); err != nil {
			return err
		}
	}
	s.results = append(s.results, r)
	return nil
}
func (s *runSink) ArtifactRef(_ context.Context, a protocol.ArtifactRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refs = append(s.refs, a)
	return nil
}
func (s *runSink) CacheRef(context.Context, protocol.CacheRef) error { return nil }
func (s *runSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, os.ErrInvalid
}

func (s *runSink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.logs))
	for i, l := range s.logs {
		out[i] = l.Text
	}
	return out
}

func (s *runSink) nResults() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.results)
}

type e2e struct {
	*rig
	drv  *Driver
	srv  *agentapi.Server
	ln   net.Listener
	base string
	sink *runSink
	pub  ed25519.PublicKey
}

// newE2E starts the listener on addr ("" picks a free loopback port).
func newE2E(t *testing.T, addr string) *e2e {
	t.Helper()
	r := newRig(t)
	e := &e2e{rig: r, sink: &runSink{eng: r.eng}}
	e.drv = &Driver{Gate: r.gate(), Minter: r.tokens, Leases: r.st, LogDir: t.TempDir(), Poll: 30 * time.Millisecond, CancelGrace: 2 * time.Second}
	e.pub, _, _ = ed25519.GenerateKey(rand.Reader)
	e.start(addr)
	return e
}

func (e *e2e) start(addr string) {
	e.t.Helper()
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	leaf := &agentapi.LeafSource{CA: e.ca, ID: "coord-1", IPs: []net.IP{net.ParseIP("127.0.0.1")}}
	cfg := agentapi.Config{CA: e.ca, Guard: pki.NewGuard(e.st), Cert: leaf.Get, Leases: e.eng, Sink: e.drv.Sink(), Source: e.drv, Store: e.st,
		Admit: e.drv.Gate.Check, PollTimeout: 2 * time.Second, SourcePoll: 20 * time.Millisecond, GuardEvery: 50 * time.Millisecond}
	srv, err := agentapi.New(cfg)
	if err != nil {
		e.t.Fatal(err)
	}
	var ln net.Listener
	for i := 0; i < 50; i++ { // the port of a killed listener may take a moment to free
		if ln, err = net.Listen("tcp", addr); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		e.t.Fatal(err)
	}
	e.srv, e.ln, e.base = srv, ln, "https://"+ln.Addr().String()
	e.drv.SetCancel(srv.Cancel)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	e.t.Cleanup(func() { e.stop(done) })
}

// stop shuts the listener down (idempotent per listener).
func (e *e2e) stop(done chan error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = e.srv.Shutdown(ctx)
	<-done
}

// kill closes the listener and every connection without a graceful drain and forgets all sessions: kill -9.
func (e *e2e) kill() {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = e.srv.Shutdown(ctx)
}

func (e *e2e) addNode(id string) {
	e.t.Helper()
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		e.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		e.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = id, id, Kind
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(c)
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = e.st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		e.t.Fatal(err)
	}
}

func (e *e2e) nodeCert(id string) tls.Certificate {
	e.t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		e.t.Fatal(err)
	}
	is, err := e.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id})
	if err != nil {
		e.t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	chain := append(append([][]byte{}, is.Chain...), is.RootDER)
	return tls.Certificate{Certificate: chain, PrivateKey: key, Leaf: leaf}
}

// carrier builds the agent-side carrier of node id with cert.
func (e *e2e) carrier(cert tls.Certificate, tune func(*CarrierConfig)) *Carrier {
	e.t.Helper()
	tlsc, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: e.ca.RootDER(), Cert: &cert, ServerName: "127.0.0.1"})
	if err != nil {
		e.t.Fatal(err)
	}
	cfg := CarrierConfig{Client: Options{BaseURL: e.base, TLS: tlsc, Wait: time.Second}, Backoff: Backoff{Min: 10 * time.Millisecond, Max: 50 * time.Millisecond}}
	if tune != nil {
		tune(&cfg)
	}
	c := NewCarrier(cfg)
	e.t.Cleanup(func() { _ = c.Close() })
	return c
}

func (e *e2e) hello() protocol.Message {
	return protocol.Message{V: 1, Type: "hello", Seq: 1, Body: protocol.Hello{Versions: []int{1}, AgentVersion: "1.5.0",
		NodeKey:          protocol.NodeKey{Alg: "ed25519", KeyID: pki.NodeKeyID(e.pub), Public: base64.StdEncoding.EncodeToString(e.pub)},
		CapabilityDigest: "sha256:00", Identity: protocol.Identity{MachineID: "m-1", HostKeyFingerprints: []string{}}, CompatModeSupported: []protocol.CompatMode{protocol.Compat15}, Slots: protocol.Slots{Total: 2}}}
}

// capability returns the registry document of the node as the dispatcher would hand it to Deliver.
func (e *e2e) capability(id string) model.Capability {
	e.t.Helper()
	n, err := e.st.NodeGet(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(n.Capability, &c); err != nil {
		e.t.Fatal(err)
	}
	return c
}

// recv reads the next message with a deadline.
func recv(t *testing.T, c *Carrier, within time.Duration) protocol.Message {
	t.Helper()
	type res struct {
		m   protocol.Message
		err error
	}
	ch := make(chan res, 1)
	go func() { m, err := c.Recv(); ch <- res{m, err} }()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("recv: %v", r.err)
		}
		return r.m
	case <-time.After(within):
		t.Fatal("recv: nothing arrived")
		return protocol.Message{}
	}
}

// recvType reads messages until one of type typ arrives.
func recvType(t *testing.T, c *Carrier, typ string, within time.Duration) protocol.Message {
	t.Helper()
	end := time.Now().Add(within)
	for time.Now().Before(end) {
		m := recv(t, c, time.Until(end))
		if m.Type == typ {
			return m
		}
	}
	t.Fatalf("no %s message within %v", typ, within)
	return protocol.Message{}
}

func eventually(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(within)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// protoLease is the lease the dispatcher hands to Deliver for the seeded attempt.
func protoLease(leaseID, attemptID string, epoch uint64) protocol.Lease {
	return protocol.Lease{LeaseID: leaseID, AttemptID: attemptID, Epoch: epoch, CompatMode: protocol.Compat15, Job: model.Job{Kind: "test", Run: []string{"true"}, Timeout: "1m"},
		Checkout: protocol.Checkout{Revision: "rev1"}, Secrets: []protocol.SecretRef{}, ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}
}
