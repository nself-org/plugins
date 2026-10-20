//go:build security

package security

// Purpose: node identity and revocation (directive: scoped runner credentials, a revoked
// runner gets no jobs; TM-20, TM-21, TM-23). A revoked node gets no new lease through the
// real dispatcher and its in-flight lease expires; the user API refuses a node certificate
// even with a good token; the node listener refuses a user certificate, a coordinator
// certificate, no certificate and a revoked node certificate at the handshake.
// Inputs:  a real store, CA, role service and TLS listeners on 127.0.0.1.
// Outputs: pass or fail.
// Constraints: all keys are generated per test; nothing here is a secret.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestRevokedNodeGetsNoJob revokes a node while the dispatcher still lists it (a stale
// world): the admission re-reads the node row and refuses with node.revoked, nothing is
// delivered, and the lease the node held is expired by the revoke.
func TestRevokedNodeGetsNoJob(t *testing.T) {
	r := newRig(t, repoPolicy(t, "private", "owner"), lanRunner("lan1", 4))
	id, a, err := r.submit(newJob("before", "owner", "lan"), "owner", 2*time.Second)
	if got := runnerOf(t, a, err); got != "lan1" {
		t.Fatalf("job before the revoke on %q", got)
	}
	res, err := r.st.NodeRevoke(context.Background(), "lan1", "cli:test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.LeasesExpired < 1 {
		t.Fatalf("the revoke expired %d leases, the in-flight lease of %s must expire", res.LeasesExpired, id)
	}
	sent := len(r.tr.deliveries())
	id2, a2, err2 := r.submit(newJob("after", "owner", "lan"), "owner", settle)
	if err2 == nil {
		t.Fatalf("a revoked node was given a lease: %+v", a2)
	}
	d, ok := r.sink.last(id2)
	if !ok || d.Reason != model.NodeRevoked {
		t.Fatalf("decision for the job after the revoke: ok=%v reason=%q outcome=%q, want %s", ok, d.Reason, d.Outcome, model.NodeRevoked)
	}
	if n := len(r.tr.deliveries()); n != sent {
		t.Fatalf("%d deliveries after the revoke, want none", n-sent)
	}
}

// userAPIBackend is a Backend with nothing behind it: the test is about who may call.
type userAPIBackend struct{}

func (userAPIBackend) Submit(context.Context, clientapi.SubmitRequest, string) (clientapi.SubmitResult, error) {
	return clientapi.SubmitResult{ID: "in-0001", State: "queued", Outcome: "enqueued"}, nil
}
func (userAPIBackend) List(context.Context, int) ([]clientapi.RunItem, error) { return nil, nil }
func (userAPIBackend) Get(context.Context, string) (clientapi.RunItem, bool, error) {
	return clientapi.RunItem{}, false, nil
}
func (userAPIBackend) Cancel(context.Context, string, string) (int, error) { return 0, nil }
func (userAPIBackend) Logs(context.Context, clientapi.LogQuery, func(logs.Line) error) (bool, error) {
	return false, nil
}
func (userAPIBackend) Evidence(context.Context, string) (json.RawMessage, error) {
	return nil, clientapi.Failf(clientapi.ReasonNotFound, "no evidence")
}

func issueNodeCert(t *testing.T, ca *pki.CA, id string) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	is, err := ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, TTL: time.Hour, OfferID: "offer-" + id})
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	chain := append(append([][]byte{}, is.Chain...), is.RootDER)
	return tls.Certificate{Certificate: chain, PrivateKey: key, Leaf: leaf}
}

func issueUserCert(t *testing.T, ca *pki.CA, id string) tls.Certificate {
	t.Helper()
	b, err := ca.IssueUser(context.Background(), id, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(append(append([]byte{}, b.CertPEM...), b.RootPEM...), b.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return pair
}

func mtlsClient(t *testing.T, ca *pki.CA, serverName string, certs ...tls.Certificate) *http.Client {
	t.Helper()
	cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: ca.RootDER(), ServerName: serverName})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Certificates = certs
	tr := &http.Transport{TLSClientConfig: cfg}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}

// TestNodeCertRefusedByAdminAPI: the user API never accepts a node certificate, and a token
// does not rescue it (the certificate wins).
func TestNodeCertRefusedByAdminAPI(t *testing.T) {
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
	svc := roles.New(st, &audit.Log{St: st})
	svc.Self = "alice"
	leaf, err := ca.IssueServer("coord-test", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	api := clientapi.New(clientapi.Config{Backend: userAPIBackend{}, Roles: svc, SubmitRole: roles.Member, Allow: func(p string) bool { return p == "acme/app" }, Poll: 10 * time.Millisecond})
	srv := httptest.NewUnstartedServer(api.Handler())
	srv.TLS = clientapi.TLSConfig(ca, func() (tls.Certificate, error) { return leaf, nil }, pki.NewGuard(st))
	srv.StartTLS()
	t.Cleanup(srv.Close)

	tok, _, err := svc.IssueToken(context.Background(), svc.Operator("cli:alice"), roles.Request{Principal: "bob", Role: roles.Member, Projects: []string{"acme/app"}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	get := func(c *http.Client, token string) (int, string) {
		req, _ := http.NewRequest("GET", srv.URL+clientapi.PathRuns, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := c.Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return resp.StatusCode, buf.String()
	}
	node := issueNodeCert(t, ca, "node-1")
	for _, tc := range []struct {
		name  string
		certs []tls.Certificate
		token string
	}{
		{"node certificate alone", []tls.Certificate{node}, ""},
		{"node certificate with a valid member token", []tls.Certificate{node}, tok},
	} {
		code, body := get(mtlsClient(t, ca, "localhost", tc.certs...), tc.token)
		if code != 403 || !strings.Contains(body, clientapi.ReasonNodeKind) {
			t.Fatalf("%s: got %d %s, want 403 %s", tc.name, code, body, clientapi.ReasonNodeKind)
		}
	}
	// Control: the same token without a node certificate works, so the refusal above is the certificate's.
	if code, body := get(mtlsClient(t, ca, "localhost"), tok); code != 200 {
		t.Fatalf("control request with the token alone: %d %s", code, body)
	}
}

// TestNodeListenerRefusesForeignAndRevokedCerts: the node listener serves a valid node
// certificate and refuses a missing, user-kind, coordinator-kind and revoked one.
func TestNodeListenerRefusesForeignAndRevokedCerts(t *testing.T) {
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
	loop := []net.IP{net.ParseIP("127.0.0.1")}
	leafSrc := &agentapi.LeafSource{CA: ca, ID: "coord-1", IPs: loop}
	srv, err := agentapi.New(agentapi.Config{CA: ca, Guard: pki.NewGuard(st), Cert: leafSrc.Get, Leases: lease.New(st, nil, lease.Config{}), Sink: &nullSink{}, Store: st, PollTimeout: time.Second, GuardEvery: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		<-done
	})
	addNode := func(id string) {
		raw, err := os.ReadFile("../../internal/model/testdata/capability/valid/laptop.valid.json")
		if err != nil {
			t.Fatal(err)
		}
		var c model.Capability
		if err = json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		c.Identity.ID, c.Identity.Name = id, id
		c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
		doc, _ := json.Marshal(c)
		admin, _ := store.NewAdminAuthority("cli:test")
		if err = st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
			t.Fatal(err)
		}
	}
	addNode("n-valid")
	addNode("n-revoked")
	valid, revoked := issueNodeCert(t, ca, "n-valid"), issueNodeCert(t, ca, "n-revoked")
	if _, err = ca.Revoke(context.Background(), "cli:test", "n-revoked", "stolen"); err != nil {
		t.Fatal(err)
	}
	user := issueUserCert(t, ca, "u-1")
	coord, err := ca.IssueServer("coord-2", nil, loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	base := "https://" + ln.Addr().String()
	try := func(c *http.Client) (int, error) {
		resp, err := c.Get(base + "/v1/agent/leases")
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		return resp.StatusCode, nil
	}
	for _, tc := range []struct {
		name  string
		certs []tls.Certificate
	}{
		{"no certificate", nil},
		{"revoked node certificate", []tls.Certificate{revoked}},
		{"user-kind certificate", []tls.Certificate{user}},
		{"coordinator-kind certificate", []tls.Certificate{coord}},
	} {
		if code, err := try(mtlsClient(t, ca, "127.0.0.1", tc.certs...)); err == nil {
			t.Fatalf("%s: the listener answered %d, it must refuse the handshake", tc.name, code)
		}
	}
	if _, err = try(mtlsClient(t, ca, "127.0.0.1", valid)); err != nil {
		t.Fatalf("control: a valid node certificate was refused: %v", err)
	}
}
