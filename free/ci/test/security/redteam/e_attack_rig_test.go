//go:build security

package redteam

// Purpose: shared fixtures of red team group E (agent pull carrier, TM-13/20/21/22/23/26): a real SQLite
// store, a real CA with token keys, the real lease engine, the real job-token Gate wired as Admit, and a
// real mTLS listener on loopback. The attacker plays a node agent with its own certificate.
// Inputs:  none. Outputs: eRig and its helpers.
// Constraints: loopback and t.TempDir only; synthetic values; every listener and goroutine is closed by t.Cleanup.

import (
	"bytes"
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
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

type eSink struct {
	mu      sync.Mutex
	logs    []protocol.Log
	results []protocol.Result
}

func (s *eSink) Log(_ context.Context, l protocol.Log) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, l)
	return nil
}
func (s *eSink) Result(_ context.Context, x protocol.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, x)
	return nil
}
func (s *eSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (s *eSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (s *eSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, io.ErrClosedPipe
}
func (s *eSink) counts() (logs, results int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.logs), len(s.results)
}

type eRig struct {
	t      *testing.T
	st     *store.Store
	ca     *pki.CA
	tokens *pki.Tokens
	eng    *lease.Engine
	gate   *agent.Gate
	sink   *eSink
	srv    *agentapi.Server
	base   string
	addr   string
}

// newERig starts the listener; withGate wires the production Admit (the Gate), false leaves Admit nil.
func newERig(t *testing.T, withGate bool) *eRig {
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
	tk, err := ca.Tokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := &eRig{t: t, st: st, ca: ca, tokens: tk, eng: lease.New(st, nil, lease.Config{}), sink: &eSink{}}
	r.gate = &agent.Gate{Tokens: tk, Bindings: st, Leases: st}
	leaf := &agentapi.LeafSource{CA: ca, ID: "coord-e", IPs: []net.IP{net.ParseIP("127.0.0.1")}}
	cfg := agentapi.Config{CA: ca, Guard: pki.NewGuard(st), Cert: leaf.Get, Leases: r.eng, Sink: r.sink, Store: st, PollTimeout: 3 * time.Second, GuardEvery: 100 * time.Millisecond}
	if withGate {
		cfg.Admit = r.gate.Check
	}
	if r.srv, err = agentapi.New(cfg); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- r.srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.srv.Shutdown(ctx)
		<-done
	})
	r.addr, r.base = ln.Addr().String(), "https://"+ln.Addr().String()
	return r
}

func (r *eRig) addNode(id string) {
	r.t.Helper()
	b, err := os.ReadFile("../../../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		r.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		r.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = id, id
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(c)
	admin, _ := store.NewAdminAuthority("cli:redteam")
	if err = r.st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		r.t.Fatal(err)
	}
}

func (r *eRig) nodeCert(id string) tls.Certificate {
	r.t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		r.t.Fatal(err)
	}
	is, err := r.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id + strconv.Itoa(int(time.Now().UnixNano()%1e6))})
	if err != nil {
		r.t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	return tls.Certificate{Certificate: append(append([][]byte{}, is.Chain...), is.RootDER), PrivateKey: key, Leaf: leaf}
}

func (r *eRig) client(cert *tls.Certificate) *http.Client {
	r.t.Helper()
	cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: r.ca.RootDER(), Cert: cert, ServerName: "127.0.0.1"})
	if err != nil {
		r.t.Fatal(err)
	}
	tr := &http.Transport{TLSClientConfig: cfg, ForceAttemptHTTP2: true}
	r.t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}

// seedLease creates a running attempt "a<n>" with lease "l<n>" granted to runner and returns lease, attempt, epoch.
func (r *eRig) seedLease(n, runner string) (string, string, uint64) {
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
	d := store.Demand{AttemptID: "a" + n, LeaseID: "l" + n, RunnerID: runner, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: time.Minute}
	fence, err := r.eng.Grant(ctx, "host", d, 1)
	if err != nil {
		r.t.Fatal(err)
	}
	if err = r.st.Transition(ctx, "a"+n, "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
		r.t.Fatal(err)
	}
	return "l" + n, "a" + n, uint64(fence)
}

func (r *eRig) mint(jti, runner, attempt string, epoch uint64) string {
	r.t.Helper()
	tok, err := r.tokens.Mint(pki.TokenClaims{JTI: jti, JobID: attempt, AttemptID: attempt, Revision: "r", RunnerID: runner, Epoch: int64(epoch)}, time.Minute)
	if err != nil {
		r.t.Fatal(err)
	}
	return tok
}

// eAgent is a node agent: its client, certificate and (after connect) session.
type eAgent struct {
	r       *eRig
	id      string
	cl      *http.Client
	cert    tls.Certificate
	pub     ed25519.PublicKey
	session string
	last    uint64 // the last upstream seq the coordinator accepted in this session, learned from 409 answers
}

func (r *eRig) newAgent(id string) *eAgent {
	r.t.Helper()
	r.addNode(id)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	c := r.nodeCert(id)
	return &eAgent{r: r, id: id, cl: r.client(&c), pub: pub, cert: c}
}

func eFrame(t *testing.T, m protocol.Message) []byte {
	t.Helper()
	var b bytes.Buffer
	m.V = 1
	if m.Seq == 0 {
		m.Seq = 1
	}
	enc, _ := protocol.NewResumedEncoder(&b, m.Seq-1, m.Seq-1)
	if err := enc.Encode(m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (a *eAgent) connect() {
	a.r.t.Helper()
	h := protocol.Hello{Versions: []int{1}, AgentVersion: "1.5.0", NodeKey: protocol.NodeKey{Alg: "ed25519", KeyID: pki.NodeKeyID(a.pub), Public: base64.StdEncoding.EncodeToString(a.pub)},
		CapabilityDigest: "sha256:00", Identity: protocol.Identity{MachineID: "m-1", HostKeyFingerprints: []string{}}, CompatModeSupported: []protocol.CompatMode{protocol.Compat15}, Slots: protocol.Slots{Total: 2}}
	resp, err := a.cl.Post(a.r.base+"/v1/agent/hello", "application/x-ndjson", bytes.NewReader(eFrame(a.r.t, protocol.Message{Type: "hello", Body: h})))
	if err != nil {
		a.r.t.Fatalf("hello: %v", err)
	}
	defer resp.Body.Close()
	m, err := protocol.NewDecoder(resp.Body, protocol.CarrierPolicy{Carrier: "B"}).Decode()
	w, ok := m.Body.(*protocol.Welcome)
	if err != nil || !ok {
		a.r.t.Fatalf("hello answer %d %v %+v", resp.StatusCode, err, m.Body)
	}
	a.session, a.last = w.SessionID, 0
}

// eStream is one open upstream request: send frames, read answers.
type eStream struct {
	t    *testing.T
	w    *io.PipeWriter
	fr   *protocol.Decoder
	resp *http.Response
	seq  uint64
}

// openStream posts /v1/agent/stream with the given X-Nself-Job-Token header values; on a 409 resume_seq it
// adopts the coordinator's last seq (as the real client does) and posts again.
func (a *eAgent) openStream(tokens ...string) *eStream {
	a.r.t.Helper()
	for try := 0; try < 4; try++ {
		pr, pw := io.Pipe()
		rq, err := http.NewRequest(http.MethodPost, a.r.base+"/v1/agent/stream", pr)
		if err != nil {
			a.r.t.Fatal(err)
		}
		rq.Header.Set(agentapi.HeaderSession, a.session)
		rq.Header.Set(agentapi.HeaderLastSeq, strconv.FormatUint(a.last, 10))
		for _, tk := range tokens {
			rq.Header.Add(agentapi.HeaderToken, tk)
		}
		type out struct {
			resp *http.Response
			err  error
		}
		ch := make(chan out, 1)
		go func() { resp, err := a.cl.Do(rq); ch <- out{resp, err} }()
		select {
		case o := <-ch:
			if o.err != nil {
				a.r.t.Fatalf("stream: %v", o.err)
			}
			if o.resp.StatusCode == http.StatusConflict {
				n, _ := strconv.ParseUint(o.resp.Header.Get(agentapi.HeaderLastSeq), 10, 64)
				o.resp.Body.Close()
				pw.Close()
				a.last = n
				continue
			}
			if o.resp.StatusCode != 200 {
				a.r.t.Fatalf("stream = %d", o.resp.StatusCode)
			}
			a.r.t.Cleanup(func() { pw.Close(); o.resp.Body.Close() })
			return &eStream{t: a.r.t, w: pw, fr: protocol.NewDecoder(o.resp.Body, protocol.CarrierPolicy{Carrier: "B"}), resp: o.resp, seq: a.last}
		case <-time.After(5 * time.Second):
			a.r.t.Fatal("stream headers never arrived")
		}
	}
	a.r.t.Fatal("stream: resume_seq kept disagreeing")
	return nil
}

func (s *eStream) send(m protocol.Message) {
	s.t.Helper()
	s.seq++
	m.Seq = s.seq
	if _, err := s.w.Write(eFrame(s.t, m)); err != nil {
		s.t.Fatalf("write frame: %v", err)
	}
}

// read returns the next answer or ok=false when nothing arrives within d (or the stream ended).
func (s *eStream) read(d time.Duration) (protocol.Message, bool) {
	type res struct {
		m   protocol.Message
		err error
	}
	ch := make(chan res, 1)
	go func() { m, err := s.fr.Decode(); ch <- res{m, err} }()
	select {
	case x := <-ch:
		return x.m, x.err == nil
	case <-time.After(d):
		return protocol.Message{}, false
	}
}

func eLog(lease string, epoch uint64, text string) protocol.Message {
	return protocol.Message{Type: "log", LeaseID: lease, Epoch: &epoch, Body: protocol.Log{LeaseID: lease, LineSeq: 1, Stream: "stdout", Text: text}}
}

func eHeartbeat(ls ...protocol.LeaseState) protocol.Message {
	if ls == nil {
		ls = []protocol.LeaseState{}
	}
	return protocol.Message{Type: "heartbeat", Body: protocol.Heartbeat{Leases: ls, Slots: protocol.Slots{Total: 2}}}
}

func eResult(lease string, epoch uint64) protocol.Message {
	return protocol.Message{Type: "result", LeaseID: lease, Epoch: &epoch, Body: protocol.Result{LeaseID: lease, Epoch: epoch, Evidence: []byte(`{}`), Signature: protocol.Signature{KeyID: "k", Value: "v"}}}
}
