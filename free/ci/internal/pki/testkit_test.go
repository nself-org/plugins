package pki

// Purpose: shared fixtures of the pki tests: a CA over a real SQLite store in t.TempDir() with an
//          injected clock, CSR and node helpers, handshake helpers and an audit reader.
// Inputs:  none. Outputs: helpers only.
// Constraints: no sleeps here; every key is generated per test; all time comes from the rig clock.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	_ "modernc.org/sqlite"
)

var epoch = time.Unix(1_800_000_000, 0)

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func (c *testClock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

type rig struct {
	t      *testing.T
	st     *store.Store
	dir    string
	dbPath string
	clk    *testClock
	ca     *CA
}

func newRig(t *testing.T) *rig {
	t.Helper()
	dir := t.TempDir()
	clk := &testClock{t: epoch}
	dbPath := filepath.Join(dir, "state.db")
	st, err := store.Open(dbPath, store.Options{Clock: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := &rig{t: t, st: st, dir: dir, dbPath: dbPath, clk: clk}
	r.ca, err = New(context.Background(), Options{Store: st, DataDir: dir, Now: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// reload builds a second CA over the same store and data directory.
func (r *rig) reload() (*CA, error) {
	return New(context.Background(), Options{Store: r.st, DataDir: r.dir, Now: r.clk.now})
}

func newCSR(t *testing.T) (csr []byte, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	csr, err = x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "attacker-chosen"}, DNSNames: []string{"evil.example"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return csr, key
}

// issued is one node certificate with everything a test needs.
type issued struct {
	*Issued
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey
	TLS  tls.Certificate
}

func (r *rig) issueNode(id string, ttl time.Duration) issued {
	r.t.Helper()
	csr, key := newCSR(r.t)
	is, err := r.ca.IssueNode(context.Background(), NodeRequest{CSR: csr, NodeID: id, TTL: ttl, OfferID: "offer-" + id})
	if err != nil {
		r.t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(is.CertDER)
	if err != nil {
		r.t.Fatal(err)
	}
	chain := append(append([][]byte{}, is.Chain...), is.RootDER)
	return issued{Issued: is, Cert: cert, Key: key, TLS: tls.Certificate{Certificate: chain, PrivateKey: key, Leaf: cert}}
}

// issueUser returns the user bundle, its parsed leaf and a tls.Certificate.
func (r *rig) issueUser(id string, ttl time.Duration) (Bundle, *x509.Certificate, tls.Certificate) {
	r.t.Helper()
	b, err := r.ca.IssueUser(context.Background(), id, ttl)
	if err != nil {
		r.t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(append(append([]byte{}, b.CertPEM...), b.RootPEM...), b.KeyPEM)
	if err != nil {
		r.t.Fatal(err)
	}
	pair.Leaf, err = x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		r.t.Fatal(err)
	}
	return b, pair.Leaf, pair
}

// addNode registers a node row (state "online") so Guard.Check finds it.
func (r *rig) addNode(id string) {
	r.t.Helper()
	b, err := os.ReadFile("../model/testdata/capability/valid/laptop.valid.json")
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
	admin, err := store.NewAdminAuthority("cli:test")
	if err != nil {
		r.t.Fatal(err)
	}
	if err = r.st.NodeAdd(context.Background(), store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		r.t.Fatal(err)
	}
}

// ro opens the database read-only for raw assertions.
func (r *rig) ro() *sql.DB {
	r.t.Helper()
	db, err := sql.Open("sqlite", "file:"+r.dbPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = db.Close() })
	return db
}

// exec writes straight to the database (tests that damage stored rows).
func (r *rig) exec(q string, args ...any) {
	r.t.Helper()
	db, err := sql.Open("sqlite", "file:"+r.dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(q, args...); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) audit(action string) (rows [][3]string) {
	r.t.Helper()
	q, err := r.ro().Query(`SELECT actor,target,detail FROM audit WHERE action=? ORDER BY seq`, action)
	if err != nil {
		r.t.Fatal(err)
	}
	defer q.Close()
	for q.Next() {
		var a [3]string
		if err = q.Scan(&a[0], &a[1], &a[2]); err != nil {
			r.t.Fatal(err)
		}
		rows = append(rows, a)
	}
	return rows
}

func (r *rig) count(query string, args ...any) (n int) {
	r.t.Helper()
	if err := r.ro().QueryRow(query, args...).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func wantReason(t *testing.T, err error, reason string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want E660 %s", reason)
	}
	if got := ReasonOf(err); got != reason {
		t.Fatalf("reason %q (%v), want %q", got, err, reason)
	}
}

func (r *rig) serverCert(dns []string, ips []net.IP, ttl time.Duration) tls.Certificate {
	r.t.Helper()
	c, err := r.ca.IssueServer("coord-1", dns, ips, ttl)
	if err != nil {
		r.t.Fatal(err)
	}
	return c
}

// handshake runs one TLS handshake over a loopback listener and returns the client and server errors.
// The client also reads one byte so a TLS 1.3 server-side rejection reaches it.
func handshake(t *testing.T, srv, cli *tls.Config) (cliErr, srvErr error, cs tls.ConnectionState) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			done <- aerr
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		s := tls.Server(c, srv)
		herr := s.Handshake()
		if herr == nil {
			_, _ = s.Write([]byte{1})
		}
		done <- herr
	}()
	raw, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
	c := tls.Client(raw, cli)
	cliErr = c.Handshake()
	if cliErr == nil {
		buf := make([]byte, 1)
		_, cliErr = c.Read(buf)
	}
	cs = c.ConnectionState()
	return cliErr, <-done, cs
}

// clientCfg is a client config for the coordinator on 127.0.0.1.
func (r *rig) clientCfg(cert *tls.Certificate) *tls.Config {
	r.t.Helper()
	cfg, err := TLSClientConfig(ClientOptions{RootDER: r.ca.RootDER(), Cert: cert, ServerName: "127.0.0.1", Now: r.clk.now})
	if err != nil {
		r.t.Fatal(err)
	}
	return cfg
}

var loopback = []net.IP{net.ParseIP("127.0.0.1")}
