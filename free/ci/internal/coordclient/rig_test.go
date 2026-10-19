package coordclient

// Purpose: shared fixtures of the coordinator client tests: a simulated TEAM coordinator (real SQLite
// store, real pki CA, real role service, the real user API over TLS 1.3 and a StoreBackend) and the
// credential files a user keeps.
// Inputs:  none. Outputs: helpers only.
// Constraints: 127.0.0.1 only; every key is generated per test; the dispatcher is a hand-driven stand-in
//          for the coordinator's run loop (it writes the same store rows a real dispatch writes).

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const testRev = "0123456789abcdef0123456789abcdef01234567"

type sim struct {
	t     *testing.T
	dir   string
	st    *store.Store
	ca    *pki.CA
	svc   *roles.Service
	coord *coordinator.Coordinator
	srv   *httptest.Server
	url   string
	allow map[string]bool
}

func newSim(t *testing.T) *sim {
	t.Helper()
	s := &sim{t: t, dir: t.TempDir(), allow: map[string]bool{"acme/app": true}}
	var err error
	if s.st, err = store.Open(filepath.Join(s.dir, "state.db"), store.Options{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.st.Close() })
	if s.ca, err = pki.New(context.Background(), pki.Options{Store: s.st, DataDir: s.dir}); err != nil {
		t.Fatal(err)
	}
	s.svc = roles.New(s.st, &audit.Log{St: s.st})
	s.svc.Self = "alice"
	if s.coord, err = coordinator.New(coordinator.Config{Store: s.st, PolicyDigest: "t"}); err != nil {
		t.Fatal(err)
	}
	leaf, err := s.ca.IssueServer("coord-test", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	api := clientapi.New(clientapi.Config{Roles: s.svc, Allow: func(p string) bool { return s.allow[p] }, Poll: 10 * time.Millisecond,
		Backend: &clientapi.StoreBackend{Store: s.st, Home: s.dir, Enqueue: s.coord.Enqueue}})
	// the listener is wrapped by hand: httptest.StartTLS would add its own certificate to the config, which a
	// client that dials an IP (no SNI) would then be given instead of the coordinator's
	s.srv = httptest.NewUnstartedServer(api.Handler())
	s.srv.Listener = tls.NewListener(s.srv.Listener, clientapi.TLSConfig(s.ca, func() (tls.Certificate, error) { return leaf, nil }, pki.NewGuard(s.st)))
	s.srv.Start()
	s.url = "https://" + s.srv.Listener.Addr().String()
	t.Cleanup(s.srv.Close)
	return s
}

func (s *sim) op() roles.Actor { return s.svc.Operator("cli:alice") }

// userFiles writes a user's bundle into a fresh directory and returns the credential paths.
func (s *sim) userFiles(id string) Creds {
	s.t.Helper()
	b, err := s.ca.IssueUser(context.Background(), id, time.Hour)
	if err != nil {
		s.t.Fatal(err)
	}
	d := s.t.TempDir()
	c := Creds{CA: filepath.Join(d, "root.pem"), Cert: filepath.Join(d, "cert.pem"), Key: filepath.Join(d, "key.pem")}
	for path, data := range map[string][]byte{c.CA: b.RootPEM, c.Cert: b.CertPEM, c.Key: b.KeyPEM} {
		if err = os.WriteFile(path, data, 0o600); err != nil {
			s.t.Fatal(err)
		}
	}
	return c
}

// rootFile writes the coordinator root, for token clients.
func (s *sim) rootFile() string {
	s.t.Helper()
	p := filepath.Join(s.t.TempDir(), "root.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.ca.RootDER()}), 0o600); err != nil {
		s.t.Fatal(err)
	}
	return p
}

// tokenFile issues a scoped token and writes it (0600) to a file.
func (s *sim) tokenFile(principal string, role roles.Role, ttl time.Duration, projects ...string) string {
	s.t.Helper()
	tok, _, err := s.svc.IssueToken(context.Background(), s.op(), roles.Request{Principal: principal, Role: role, Projects: projects, TTL: ttl})
	if err != nil {
		s.t.Fatal(err)
	}
	p := filepath.Join(s.t.TempDir(), "token")
	if err = os.WriteFile(p, []byte(tok+"\n"), 0o600); err != nil {
		s.t.Fatal(err)
	}
	return p
}

func (s *sim) target(c Creds) Target { return Target{Kind: Remote, URL: s.url, Creds: c} }

func (s *sim) client(c Creds) *Client {
	s.t.Helper()
	cl, err := New(s.target(c))
	if err != nil {
		s.t.Fatal(err)
	}
	return cl
}

// start claims the queued intake row and records a running pipeline for it.
func (s *sim) start(pid string) {
	s.t.Helper()
	ctx := context.Background()
	row, ok, err := s.st.IntakeClaim(ctx, "coord-sim")
	if err != nil || !ok {
		s.t.Fatalf("nothing queued: %v", err)
	}
	p := store.PipelineRow{ID: pid, Project: "/tmp/checkout-" + pid, Revision: row.Revision, Trigger: "api", SourceTrust: "owner", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err = s.st.CreatePipeline(ctx, p); err != nil {
		s.t.Fatal(err)
	}
	if err = s.st.CreateJob(ctx, store.JobRow{ID: "job-" + pid, PipelineID: pid, Name: "build", Kind: "test", InfraMax: 1, InputDigest: "i"}); err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() {})
}

// finish ends the run and stores its evidence document.
func (s *sim) finish(intakeID, pid, status string, doc []byte) {
	s.t.Helper()
	ctx := context.Background()
	if err := s.st.IntakeFinish(ctx, intakeID, "coord-sim", pid, "ok", true); err != nil {
		s.t.Fatal(err)
	}
	if err := s.st.FinishPipeline(ctx, pid, status); err != nil {
		s.t.Fatal(err)
	}
	if doc != nil {
		if err := s.st.RecordRunEvidence(ctx, "ev-"+pid, pid, testRev, "i", doc); err != nil {
			s.t.Fatal(err)
		}
	}
}

// goodDoc is a valid, signed evidence document from the attest golden set.
func goodDoc(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../attest/testdata/adversarial/good.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Evidence json.RawMessage `json:"evidence"`
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f.Evidence
}

func reasonIs(err error, want string) bool { return clientapi.ReasonOf(err) == want }

func lineCount(s string) int { return len(strings.Split(strings.TrimSpace(s), "\n")) }

// nodeFiles writes a node-kind certificate as if it were a user's credential.
func (s *sim) nodeFiles(id string) Creds {
	s.t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		s.t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		s.t.Fatal(err)
	}
	is, err := s.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, TTL: time.Hour, OfferID: "offer-" + id})
	if err != nil {
		s.t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	d := s.t.TempDir()
	c := Creds{CA: s.rootFile(), Cert: filepath.Join(d, "cert.pem"), Key: filepath.Join(d, "key.pem")}
	var chain []byte
	for _, b := range append(append([][]byte{}, is.Chain...), is.RootDER) {
		chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: b})...)
	}
	for path, data := range map[string][]byte{c.Cert: chain, c.Key: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})} {
		if err = os.WriteFile(path, data, 0o600); err != nil {
			s.t.Fatal(err)
		}
	}
	return c
}
