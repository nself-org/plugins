package agentapi

// Purpose: shared fixtures of the agentapi tests: a real SQLite store, a real CA, the real lease engine,
//          a real TLS listener on loopback, node and user certificates and an mTLS test client.
// Inputs:  none. Outputs: helpers only.
// Constraints: fake synthetic values only; every goroutine and listener is closed by t.Cleanup.

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
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	_ "modernc.org/sqlite"
)

type recSink struct {
	mu      sync.Mutex
	logs    []protocol.Log
	results []protocol.Result
}

func (r *recSink) Log(_ context.Context, l protocol.Log) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, l)
	return nil
}
func (r *recSink) Result(_ context.Context, x protocol.Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, x)
	return nil
}
func (r *recSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (r *recSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (r *recSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, fmt.Errorf("unused")
}

type rig struct {
	t     *testing.T
	st    *store.Store
	ca    *pki.CA
	eng   *lease.Engine
	sink  *recSink
	srv   *Server
	base  string
	guard *pki.Guard
	db    string
}

func newRig(t *testing.T, tune func(*Config)) *rig {
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
	r := &rig{t: t, db: filepath.Join(dir, "state.db"), st: st, ca: ca, eng: lease.New(st, nil, lease.Config{}), sink: &recSink{}, guard: pki.NewGuard(st)}
	leaf := &LeafSource{CA: ca, ID: "coord-1", IPs: loopbackIPs}
	cfg := Config{CA: ca, Guard: r.guard, Cert: leaf.Get, Leases: r.eng, Sink: r.sink, Store: st, PollTimeout: 5 * time.Second, GuardEvery: 100 * time.Millisecond}
	if tune != nil {
		tune(&cfg)
	}
	if r.srv, err = New(cfg); err != nil {
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
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	r.base = "https://" + ln.Addr().String()
	return r
}

func (r *rig) addNode(id string) {
	r.t.Helper()
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
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
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = r.st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) nodeCert(id string) tls.Certificate {
	r.t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		r.t.Fatal(err)
	}
	is, err := r.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id})
	if err != nil {
		r.t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	chain := append(append([][]byte{}, is.Chain...), is.RootDER)
	return tls.Certificate{Certificate: chain, PrivateKey: key, Leaf: leaf}
}

func (r *rig) userCert(id string) tls.Certificate {
	r.t.Helper()
	b, err := r.ca.IssueUser(context.Background(), id, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(append(append([]byte{}, b.CertPEM...), b.RootPEM...), b.KeyPEM)
	if err != nil {
		r.t.Fatal(err)
	}
	return pair
}

// client is an mTLS HTTP client of one certificate (nil: no client certificate).
func (r *rig) client(cert *tls.Certificate) *http.Client {
	r.t.Helper()
	cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: r.ca.RootDER(), Cert: cert, ServerName: "127.0.0.1"})
	if err != nil {
		r.t.Fatal(err)
	}
	tr := &http.Transport{TLSClientConfig: cfg, ForceAttemptHTTP2: true}
	r.t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}

// agent is one test agent: a node id, its Ed25519 key, a client and (after hello) a session id.
type agent struct {
	r       *rig
	id      string
	cl      *http.Client
	pub     ed25519.PublicKey
	cert    tls.Certificate
	session string
}

func (r *rig) newAgent(id string) *agent {
	r.t.Helper()
	r.addNode(id)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	c := r.nodeCert(id)
	return &agent{r: r, id: id, cl: r.client(&c), pub: pub, cert: c}
}

func frame(t *testing.T, m protocol.Message) []byte {
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

func (a *agent) hello(mut func(*protocol.Hello)) *http.Response {
	a.r.t.Helper()
	resp, err := a.helloErr(mut)
	if err != nil {
		a.r.t.Fatalf("hello: %v", err)
	}
	return resp
}

var loopbackIPs = []net.IP{net.ParseIP("127.0.0.1")}

func (a *agent) helloErr(mut func(*protocol.Hello)) (*http.Response, error) {
	a.r.t.Helper()
	h := protocol.Hello{Versions: []int{1}, AgentVersion: "1.5.0", NodeKey: protocol.NodeKey{Alg: "ed25519", KeyID: pki.NodeKeyID(a.pub), Public: base64.StdEncoding.EncodeToString(a.pub)},
		CapabilityDigest: "sha256:00", Identity: protocol.Identity{MachineID: "m-1", HostKeyFingerprints: []string{}}, CompatModeSupported: []protocol.CompatMode{protocol.Compat15}, Slots: protocol.Slots{Total: 2}}
	if mut != nil {
		mut(&h)
	}
	return a.cl.Post(a.r.base+"/v1/agent/hello", "application/x-ndjson", bytes.NewReader(frame(a.r.t, protocol.Message{Type: "hello", Body: h})))
}

// connect says hello and keeps the session id.
func (a *agent) connect() {
	a.r.t.Helper()
	resp := a.hello(nil)
	defer func() { _ = resp.Body.Close() }()
	m := readFrame(a.r.t, resp.Body)
	w, ok := m.Body.(*protocol.Welcome)
	if resp.StatusCode != 200 || !ok {
		a.r.t.Fatalf("hello = %d %s %+v", resp.StatusCode, m.Type, m.Body)
	}
	a.session = w.SessionID
	if resp.Header.Get(HeaderSession) != a.session {
		a.r.t.Fatal("session header differs from welcome")
	}
}

func (a *agent) req(method, path string, body io.Reader, hdr map[string]string) *http.Request {
	rq, err := http.NewRequest(method, a.r.base+path, body)
	if err != nil {
		a.r.t.Fatal(err)
	}
	rq.Header.Set(HeaderSession, a.session)
	for k, v := range hdr {
		rq.Header.Set(k, v)
	}
	return rq
}

// frameReader decodes frames of a response body in order.
type frameReader struct {
	d *protocol.Decoder
}

func newFrameReader(r io.Reader) *frameReader {
	return &frameReader{d: protocol.NewDecoder(r, protocol.CarrierPolicy{Carrier: "B"})}
}

func (f *frameReader) next() (protocol.Message, error) { return f.d.Decode() }

func readFrame(t *testing.T, r io.Reader) protocol.Message {
	t.Helper()
	m, err := newFrameReader(r).next()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return m
}

// seedLease creates one running attempt with a granted lease and returns its lease id and epoch.
func (r *rig) seedLease(n string, holder ...string) (string, uint64) {
	r.t.Helper()
	runner := "n-" + n // the node that holds the lease (the agent of the test by default)
	if len(holder) > 0 {
		runner = holder[0]
	}
	ctx := context.Background()
	p := store.PipelineRow{ID: "p" + n, Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	steps := []func() error{
		func() error { return r.st.CreatePipeline(ctx, p) },
		func() error {
			return r.st.CreateJob(ctx, store.JobRow{ID: "j" + n, PipelineID: p.ID, Name: n, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"})
		},
		func() error { return r.st.CreateAttempt(ctx, store.AttemptRow{ID: "a" + n, JobID: "j" + n, N: 1}, "") },
	}
	for _, f := range steps {
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
	return "l" + n, uint64(fence)
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

// auditDetail returns the audit details recorded for a target.
func (r *rig) auditDetail(target string) []string {
	r.t.Helper()
	db, err := sql.Open("sqlite", "file:"+r.db+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query(`SELECT detail FROM audit WHERE target=? ORDER BY seq`, target)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err != nil {
			r.t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}
