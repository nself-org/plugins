package clientapi

// Purpose: shared fixtures of the user API tests: a real SQLite store, a real pki CA, a role service,
// a scripted Backend and a TLS test server running TLSConfig.
// Inputs:  none. Outputs: helpers only.
// Constraints: no network beyond 127.0.0.1; all keys are generated per test; nothing here is a secret.

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// fakeBackend is a scripted Backend.
type fakeBackend struct {
	mu       sync.Mutex
	runs     []RunItem // newest first
	submits  []SubmitRequest
	cancels  []string
	evidence json.RawMessage
	lines    []logs.Line
}

func (f *fakeBackend) Submit(_ context.Context, r SubmitRequest, _ string) (SubmitResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, r)
	return SubmitResult{ID: "in-0001", State: "queued", Outcome: "enqueued"}, nil
}

func (f *fakeBackend) List(context.Context, int) ([]RunItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]RunItem, len(f.runs))
	for i, r := range f.runs {
		out[i] = cp(r)
	}
	return out, nil
}

// cp copies the job list, so a reader never shares memory with set.
func cp(r RunItem) RunItem {
	r.Jobs = append([]cirun.JobView(nil), r.Jobs...)
	return r
}

func (f *fakeBackend) Get(_ context.Context, ref string) (RunItem, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.runs {
		if r.ID == ref {
			return cp(r), true, nil
		}
	}
	return RunItem{}, false, nil
}

func (f *fakeBackend) set(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.runs {
		if f.runs[i].ID == id {
			f.runs[i].Status = status
			for j := range f.runs[i].Jobs {
				f.runs[i].Jobs[j].State = status
			}
		}
	}
}

func (f *fakeBackend) Cancel(_ context.Context, ref, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, ref)
	return 1, nil
}

func (f *fakeBackend) Logs(ctx context.Context, q LogQuery, fn func(logs.Line) error) (bool, error) {
	if q.Job != "build" {
		return false, nil
	}
	if q.Follow { // a followed log: one line, then it waits for the stream to end
		if err := fn(logs.Line{TS: "t", Stream: "out", Seq: 1, Text: "first"}); err != nil {
			return true, err
		}
		<-ctx.Done()
		return true, nil
	}
	for _, l := range f.lines {
		if l.Seq > q.After {
			if err := fn(l); err != nil {
				return true, err
			}
		}
	}
	return true, nil
}

func (f *fakeBackend) Evidence(context.Context, string) (json.RawMessage, error) {
	if f.evidence == nil {
		return nil, Failf(ReasonNotFound, "the run has no evidence yet")
	}
	return f.evidence, nil
}

func item(id, project, status string) RunItem {
	return RunItem{Project: project, RunView: cirun.RunView{ID: id, Revision: strings.Repeat("a", 40), Status: status,
		Jobs: []cirun.JobView{{Name: "build", State: status}}}}
}

type rig struct {
	t     *testing.T
	st    *store.Store
	ca    *pki.CA
	svc   *roles.Service
	be    *fakeBackend
	srv   *httptest.Server
	now   func() time.Time
	shift time.Duration // added to the role service clock
	allow map[string]bool
}

func newRig(t *testing.T) *rig {
	t.Helper()
	dir := t.TempDir()
	r := &rig{t: t, be: &fakeBackend{}, allow: map[string]bool{"acme/app": true, "acme/other": true}, now: time.Now}
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r.st = st
	if r.ca, err = pki.New(context.Background(), pki.Options{Store: st, DataDir: dir}); err != nil {
		t.Fatal(err)
	}
	r.svc = roles.New(st, &audit.Log{St: st})
	r.svc.Self = "alice"
	r.svc.Now = func() time.Time { return r.now().Add(r.shift) }
	leaf, err := r.ca.IssueServer("coord-test", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	api := New(Config{Backend: r.be, Roles: r.svc, Allow: func(p string) bool { return r.allow[p] }, Poll: 10 * time.Millisecond})
	r.srv = httptest.NewUnstartedServer(api.Handler())
	r.srv.TLS = TLSConfig(r.ca, func() (tls.Certificate, error) { return leaf, nil }, pki.NewGuard(st))
	r.srv.StartTLS()
	t.Cleanup(r.srv.Close)
	return r
}

func (r *rig) op() roles.Actor { return r.svc.Operator("cli:alice") }

// token issues a scoped token and returns its clear text.
func (r *rig) token(principal string, role roles.Role, ttl time.Duration, projects ...string) string {
	r.t.Helper()
	tok, _, err := r.svc.IssueToken(context.Background(), r.op(), roles.Request{Principal: principal, Role: role, Projects: projects, TTL: ttl})
	if err != nil {
		r.t.Fatal(err)
	}
	return tok
}

func (r *rig) client(certs ...tls.Certificate) *http.Client {
	cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: r.ca.RootDER(), ServerName: "localhost"})
	if err != nil {
		r.t.Fatal(err)
	}
	cfg.Certificates = certs
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
}

func (r *rig) userCert(id string) tls.Certificate {
	r.t.Helper()
	b, err := r.ca.IssueUser(context.Background(), id, time.Hour)
	if err != nil {
		r.t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(append(append([]byte{}, b.CertPEM...), b.RootPEM...), b.KeyPEM)
	if err != nil {
		r.t.Fatal(err)
	}
	return pair
}

func (r *rig) nodeCert(id string) tls.Certificate {
	r.t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		r.t.Fatal(err)
	}
	is, err := r.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, TTL: time.Hour, OfferID: "offer-" + id})
	if err != nil {
		r.t.Fatal(err)
	}
	chain := append(append([][]byte{}, is.Chain...), is.RootDER)
	return tls.Certificate{Certificate: chain, PrivateKey: key}
}

// do sends one request; a non-empty token is sent as a bearer token.
func (r *rig) do(c *http.Client, method, path, token, body string) (int, []byte) {
	r.t.Helper()
	req, err := http.NewRequest(method, r.srv.URL+path, strings.NewReader(body))
	if err != nil {
		r.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		r.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// refused asserts an E664 body with the given reason.
func refused(t *testing.T, status int, body []byte, want int, reason string) {
	t.Helper()
	var b Body
	if err := json.Unmarshal(body, &b); err != nil || status != want || b.Code != CodeUserAPI || b.Reason != reason {
		t.Fatalf("got %d %s, want %d E664 %s", status, body, want, reason)
	}
}

func lines(t *testing.T, b []byte) []string {
	t.Helper()
	var out []string
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}
