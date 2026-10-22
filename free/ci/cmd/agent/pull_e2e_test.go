package main

// Purpose: `nself-ci-agent pull` end to end on loopback: the real agent listener, token gate and pull driver
//          on one side, the real command (session, fence, executor) on the other, a node enrolled with a real
//          certificate. A shell job runs, its output and result arrive, and the job token never reaches the job.
// Inputs:  a CA, store and listener built in the test; the agent state dir under t.TempDir().
// Constraints: unix only (shell job); bounded waits; no real secrets.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

type pullRig struct {
	t      *testing.T
	st     *store.Store
	ca     *pki.CA
	tokens *pki.Tokens
	eng    *lease.Engine
	drv    *agent.Driver
	minter *recMinter
	srv    *agentapi.Server
	base   string
	sink   *pullSink
	px     *netProxy
}

// pullSink is the run's sink: it records output and commits results like the run's remote sink.
type pullSink struct {
	mu      sync.Mutex
	lines   []string
	results []protocol.Result
	eng     *lease.Engine
}

func (s *pullSink) Log(_ context.Context, l protocol.Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, l.Text)
	return nil
}
func (s *pullSink) Result(ctx context.Context, r protocol.Result) error {
	if err := s.eng.Result(ctx, r.LeaseID, int64(r.Epoch)); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, r)
	return nil
}
func (s *pullSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (s *pullSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (s *pullSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, os.ErrInvalid
}

func (s *pullSink) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

type rigOpts struct {
	heartbeat, coordTTL time.Duration // listener timings (0: defaults)
	leaseTTL            time.Duration // the engine's lease TTL (0: default)
	proxy               bool          // put a TCP proxy in front of the listener
}

func newPullRig(t *testing.T) *pullRig { return newPullRigWith(t, rigOpts{}) }

func newPullRigWith(t *testing.T, o rigOpts) *pullRig {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	r := &pullRig{t: t, st: st, ca: ca, eng: lease.New(st, nil, lease.Config{TTL: o.leaseTTL})}
	if r.tokens, err = ca.Tokens(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.sink = &pullSink{eng: r.eng}
	gate := &agent.Gate{Tokens: r.tokens, Bindings: st, Leases: st}
	r.minter = &recMinter{inner: r.tokens}
	r.drv = &agent.Driver{Gate: gate, Minter: r.minter, Leases: st, LogDir: filepath.Join(dir, "logs"), Poll: 50 * time.Millisecond, CancelGrace: 5 * time.Second}
	leaf := &agentapi.LeafSource{CA: ca, ID: "coord", IPs: []net.IP{net.ParseIP("127.0.0.1")}}
	srv, err := agentapi.New(agentapi.Config{CA: ca, Guard: pki.NewGuard(st), Cert: leaf.Get, Leases: r.eng, Sink: r.drv.Sink(), Source: r.drv, Store: st, Admit: r.drv.Admit,
		Blobs: r.drv.Blobs(), Heartbeat: o.heartbeat, CoordinatorTTL: o.coordTTL, PollTimeout: 2 * time.Second, SourcePoll: 30 * time.Millisecond, GuardEvery: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r.srv, r.base = srv, "https://"+ln.Addr().String()
	if o.proxy {
		r.px = newNetProxy(t, ln.Addr().String())
		r.base = "https://" + r.px.addr()
	}
	r.drv.SetCancel(srv.Cancel)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return r
}

// enroll registers the node and writes the enrollment `join` would have saved.
func (r *pullRig) enroll(state, nodeID, url string) {
	r.t.Helper()
	b, err := os.ReadFile("../../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		r.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		r.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = nodeID, nodeID, agent.Kind
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(c)
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = r.st.NodeAdd(context.Background(), store.NodeRecord{ID: nodeID, Name: nodeID, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		r.t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	is, err := r.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: nodeID, OfferID: "offer-" + nodeID})
	if err != nil {
		r.t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	res := enroll.JoinResult{NodeID: nodeID, CertDER: is.CertDER, Chain: is.Chain, RootDER: is.RootDER, KeyDER: keyDER, AgentURL: url, NotAfter: is.NotAfter, RootFingerprint: pki.CertFingerprint(is.RootDER)}
	if err = os.MkdirAll(state, 0o700); err != nil {
		r.t.Fatal(err)
	}
	if err = enroll.SaveCredential(state, res, time.Now()); err != nil {
		r.t.Fatal(err)
	}
}

// writeEnrollment enrolls a node of a throwaway coordinator with the given agent URL.
func writeEnrollment(t *testing.T, state, url string) {
	t.Helper()
	newPullRig(t).enroll(state, "n-1", url)
}

// seed creates a running attempt with a granted lease held by node.
func (r *pullRig) seed(n, node string) (string, string, uint64) {
	r.t.Helper()
	ctx := context.Background()
	p := store.PipelineRow{ID: "p" + n, Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for _, f := range []func() error{
		func() error { return r.st.CreatePipeline(ctx, p) },
		func() error {
			return r.st.CreateJob(ctx, store.JobRow{ID: "j" + n, PipelineID: p.ID, Name: n, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"})
		},
		func() error { return r.st.CreateAttempt(ctx, store.AttemptRow{ID: "a" + n, JobID: "j" + n, N: 1}, "") },
	} {
		if err := f(); err != nil {
			r.t.Fatal(err)
		}
	}
	fence, err := r.eng.Grant(ctx, "host", store.Demand{AttemptID: "a" + n, LeaseID: "l" + n, RunnerID: node, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: r.leaseTTL()}, 1)
	if err != nil {
		r.t.Fatal(err)
	}
	if err = r.st.Transition(ctx, "a"+n, "leased", "running", store.TransitionDetail{Epoch: attemptEpochOf(r.t, r.st, "a"+n)}); err != nil {
		r.t.Fatal(err)
	}
	return "l" + n, "a" + n, uint64(fence)
}

func attemptEpochOf(t *testing.T, st *store.Store, id string) int64 {
	t.Helper()
	_, ep, err := st.AttemptStateEpoch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ep
}

func pullWait(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(within)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// startPull runs `pull` in the background and returns its exit code channel and stderr.
func startPull(t *testing.T, ctx context.Context, args ...string) (<-chan int, *lockedBuf) {
	t.Helper()
	errb := &lockedBuf{}
	code := make(chan int, 1)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		code <- run(ctx, append([]string{"pull"}, args...), IO{Stdout: errb, Stderr: errb})
	}()
	// the loop writes into the state directory until it returns; TempDir removal must come after
	t.Cleanup(func() {
		select {
		case <-exited:
		case <-time.After(20 * time.Second):
			t.Error("the pull loop did not exit after its context ended")
		}
	})
	return code, errb
}

type lockedBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func TestPullRunsAShellJobEndToEndAndKeepsTheTokenOutOfIt(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	r := newPullRig(t)
	r.enroll(state, "n-1", r.base)
	ctx, cancel := context.WithCancel(context.Background())
	code, errb := startPull(t, ctx)
	pullWait(t, 15*time.Second, "the agent to connect", func() bool { return len(r.srv.NodesConnected()) == 1 })
	if _, err := nodekey.LoadOrCreate(state); err != nil {
		t.Fatal(err)
	}

	leaseID, att, ep := r.seed("1", "n-1")
	l := protocol.Lease{LeaseID: leaseID, AttemptID: att, Epoch: ep, CompatMode: protocol.Compat15,
		Job:      model.Job{Kind: "static", Run: []string{"sh", "-c", "echo hello-from-the-job; env; echo done-marker"}},
		Checkout: protocol.Checkout{Revision: strings.Repeat("a", 40)}, Secrets: []protocol.SecretRef{}, ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}
	var node model.Capability
	node.Identity.ID = "n-1"
	runErr := make(chan error, 1)
	go func() { runErr <- r.drv.Run(context.Background(), node, l, r.sink) }()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v\nagent stderr:\n%s", err, errb.String())
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("the job never finished\nagent stderr:\n%s\nlines:\n%s", errb.String(), r.sink.text())
	}
	out := r.sink.text()
	if !strings.Contains(out, "hello-from-the-job") || !strings.Contains(out, "done-marker") {
		t.Fatalf("job output missing:\n%s", out)
	}
	cl := mustClaims(t, r)
	if strings.Contains(out, cl.token) || strings.Contains(out, "NSELF_CI_TOKEN") {
		t.Fatalf("the job environment holds the token:\n%s", out)
	}
	if b, found, _ := r.st.TokenCheck(context.Background(), cl.jti); !found || !b.Spent {
		t.Fatalf("token binding %+v found=%v", b, found)
	}
	cancel()
	select {
	case c := <-code:
		if c != 0 {
			t.Fatalf("pull exited %d\n%s", c, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("pull did not stop")
	}
}

type tokenInfo struct{ token, jti string }

// mustClaims returns the last token the driver signed and its jti.
func mustClaims(t *testing.T, r *pullRig) tokenInfo {
	t.Helper()
	tok := r.minter.last()
	if tok == "" {
		t.Fatal("the driver signed no token")
	}
	cl, err := r.tokens.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	return tokenInfo{tok, cl.JTI}
}

// recMinter remembers the last token it signed.
type recMinter struct {
	inner agent.Minter
	mu    sync.Mutex
	tok   string
}

func (m *recMinter) Mint(cl pki.TokenClaims, d time.Duration) (string, error) {
	tok, err := m.inner.Mint(cl, d)
	m.mu.Lock()
	m.tok = tok
	m.mu.Unlock()
	return tok, err
}

func (m *recMinter) last() string { m.mu.Lock(); defer m.mu.Unlock(); return m.tok }

func (r *pullRig) leaseTTL() time.Duration {
	if r.eng.Config.TTL > 0 {
		return r.eng.Config.TTL
	}
	return 2 * time.Minute
}
