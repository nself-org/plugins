package main

// Purpose: `nself ci nodes pair` and the PKI hooks nodes_pair.go registers with the node lifecycle: the
// foreground window end to end (scripted admin on stdin, the URL parsed from stdout, a real join), its
// exit codes, and `nodes rotate --cert|--node-key` and `nodes revoke|remove` through lifecycle.Run against
// a pull (agent) node that has no ssh destination. Real TLS on 127.0.0.1:0, a real store and CA.
// Inputs:  scripted stdin and argv. Outputs: pass/fail. Constraints: the only real wait is join's own
//          1.1 s rate-limit pause; every state dir is under t.TempDir().

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
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type pairBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (p *pairBuf) Write(b []byte) (int, error) { p.mu.Lock(); defer p.mu.Unlock(); return p.b.Write(b) }
func (p *pairBuf) String() string              { p.mu.Lock(); defer p.mu.Unlock(); return p.b.String() }

var (
	failRevoke  atomic.Bool
	pairHookSee atomic.Pointer[string]
)

func init() {
	lifecycle.RegisterRevoker(func(context.Context, *store.RevokeTx, string) error {
		if failRevoke.Load() {
			return errors.New("injected revoker failure")
		}
		return nil
	})
	lifecycle.RegisterAddHook(lifecycle.PointPair, func(_ context.Context, e lifecycle.HookEvent) error {
		id := e.NodeID
		pairHookSee.Store(&id)
		return nil
	})
}

func pairHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "ci")
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_CI_REPO", t.TempDir()) // not a git repo: the project id resolves to nothing, hermetically
	return home
}

type banner struct{ url, code, fp string }

var bannerRE = map[string]*regexp.Regexp{
	"url":  regexp.MustCompile(`enroll URL:\s+(\S+)`),
	"code": regexp.MustCompile(`pairing code:\s+(\S+)`),
	"fp":   regexp.MustCompile(`root fingerprint:\s+(\S+)`),
}

func waitBanner(t *testing.T, out *pairBuf) banner {
	t.Helper()
	for end := time.Now().Add(90 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		s := out.String()
		u, c, f := bannerRE["url"].FindStringSubmatch(s), bannerRE["code"].FindStringSubmatch(s), bannerRE["fp"].FindStringSubmatch(s)
		if u != nil && c != nil && f != nil {
			return banner{u[1], c[1], f[1]}
		}
	}
	t.Fatalf("no banner on stdout: %q", out.String())
	return banner{}
}

func hostPort(t *testing.T, enrollURL string) string {
	t.Helper()
	m := regexp.MustCompile(`^https://([^/]+)/`).FindStringSubmatch(enrollURL)
	if m == nil {
		t.Fatalf("enroll URL %q", enrollURL)
	}
	return m[1]
}

func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// startPair runs `nodes pair` with an admin pipe on stdin; it returns the pipe, the streams and the exit channel.
func startPair(t *testing.T, args ...string) (*io.PipeWriter, *pairBuf, *pairBuf, <-chan int) {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close(); _ = pr.Close() })
	var out, errb pairBuf
	done := make(chan int, 1)
	base := []string{"--enroll-addr", "127.0.0.1:0", "--public-url", "https://127.0.0.1", "--coordinator-id", "local"}
	go func() { done <- runNodesPair(append(base, args...), pr, &out, &errb) }()
	return pw, &out, &errb, done
}

func exitWithin(t *testing.T, done <-chan int, d time.Duration) int {
	t.Helper()
	select {
	case c := <-done:
		return c
	case <-time.After(d):
		t.Fatal("nodes pair did not exit")
	}
	return -1
}

func TestNodesPairEndToEnd(t *testing.T) {
	home := pairHome(t)
	pairHookSee.Store(nil)
	admin, out, errb, done := startPair(t, "--label", "lab box", "--agent-url", "https://agent.test:3865", "--cert-ttl", "30m")
	b := waitBanner(t, out)
	if !strings.HasPrefix(b.url, "https://127.0.0.1:") || !strings.HasSuffix(b.url, "/v1/enroll") || len(b.code) != pairing.CodeLen || !strings.HasPrefix(b.fp, "SHA256:") {
		t.Fatalf("banner %+v", b)
	}
	if !strings.Contains(out.String(), "nself-ci-agent join "+b.url) {
		t.Fatalf("the join command is not shown: %s", out.String())
	}
	addr := hostPort(t, b.url)
	if !portOpen(addr) {
		t.Fatal("the window is not listening")
	}
	// The node: no terminal, --expect-fingerprint, the real clock; the admin types what join prints.
	var joinOut pairBuf
	go func() {
		re := regexp.MustCompile(`CSR key fingerprint: (SHA256:\S+)`)
		for end := time.Now().Add(90 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			if m := re.FindStringSubmatch(joinOut.String()); m != nil {
				_, _ = admin.Write([]byte(m[1] + "\nowner,internal\nnone\nprivate-infrastructure\nproj-1\n"))
				return
			}
		}
	}()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := enroll.Join(ctx, enroll.JoinOptions{URL: b.url, ExpectFingerprint: b.fp, Name: "node-pair", OS: "linux", Arch: "amd64", NodeKey: pub, NodeKeyID: pki.NodeKeyID(pub), NodeSign: func(m []byte) ([]byte, error) { return ed25519.Sign(priv, m), nil },
		Out: &joinOut, Code: func() (string, error) { return b.code, nil }})
	if err != nil {
		t.Fatalf("join: %v\nadmin screen:\n%s", err, out.String())
	}
	if code := exitWithin(t, done, 90*time.Second); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "A node joined. The pairing window is closed.") || !strings.Contains(out.String(), res.CSRFingerprint) {
		t.Fatalf("stdout %s", out.String())
	}
	if n := strings.Count(out.String(), b.code); n != 1 {
		t.Fatalf("the code appears %d times on stdout, want exactly once (the banner)", n)
	}
	if strings.Contains(errb.String(), b.code) || strings.Contains(joinOut.String(), b.code) {
		t.Fatal("the code leaked to stderr or join output")
	}
	if portOpen(addr) {
		t.Fatal("the listener is still open after the node joined")
	}
	if left := time.Until(res.NotAfter); left < 29*time.Minute || left > 31*time.Minute {
		t.Fatalf("--cert-ttl 30m gave a certificate valid for %s", left)
	}
	if res.AgentURL != "https://agent.test:3865" {
		t.Fatalf("agent URL %q", res.AgentURL)
	}
	if id := pairHookSee.Load(); id == nil || *id != "node-pair" {
		t.Fatalf("the pair lifecycle hook saw %v", id)
	}
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n, err := registry.New(st).Get(context.Background(), "node-pair")
	if err != nil {
		t.Fatal(err)
	}
	if got := *n.Capability.Trust.Accepts.Value; len(got) != 2 || *n.Capability.Trust.PrivacyZone.Value != "private-infrastructure" || n.Capability.Lifecycle.AuthorizedProjects[0] != "proj-1" {
		t.Fatalf("trust %+v lifecycle %+v", n.Capability.Trust, n.Capability.Lifecycle)
	}
	if _, err = os.Stat(pki.KEKPath(home)); err != nil {
		t.Fatalf("the coordinator KEK is not beside the store: %v", err)
	}
	// Nothing about the code, the CSR or the keys on disk next to the store besides sealed material.
	raw, _ := os.ReadFile(filepath.Join(home, "state.db"))
	if bytes.Contains(raw, []byte(b.code)) {
		t.Fatal("the code is in the database")
	}
}

// The URL printed without --public-url: ":port" names every interface, so the host is "localhost"; a
// "host:port" address names itself.
func TestNodesPairDefaultPublicURL(t *testing.T) {
	old := interfaceIPs
	interfaceIPs = func() []net.IP { return nil } // a machine with no usable address
	t.Cleanup(func() { interfaceIPs = old })
	for _, tc := range []struct{ addr, prefix string }{
		{":0", "https://localhost:"},
		{"127.0.0.1:0", "https://127.0.0.1:"},
	} {
		pairHome(t)
		pr, pw := io.Pipe()
		var out, errb pairBuf
		done := make(chan int, 1)
		go func() { done <- runNodesPair([]string{"--enroll-addr", tc.addr}, pr, &out, &errb) }()
		b := waitBanner(t, &out)
		if !strings.HasPrefix(b.url, tc.prefix) || !strings.HasSuffix(b.url, "/v1/enroll") || strings.HasSuffix(strings.TrimSuffix(b.url, "/v1/enroll"), ":0") {
			t.Errorf("--enroll-addr %s printed %q, want prefix %q and the bound port", tc.addr, b.url, tc.prefix)
		}
		if err := pairing.Withdraw(context.Background(), b.code); err != nil {
			t.Fatal(err)
		}
		if code := exitWithin(t, done, 90*time.Second); code != 1 {
			t.Errorf("--enroll-addr %s: exit %d, stderr %q", tc.addr, code, errb.String())
		}
		_ = pw.Close()
	}
}

func TestDefaultPublicURL(t *testing.T) {
	ips := func(ss ...string) (out []net.IP) {
		for _, s := range ss {
			out = append(out, net.ParseIP(s))
		}
		return out
	}
	for _, tc := range []struct {
		name, addr string
		ips        []net.IP
		want       string
		wantErr    bool
	}{
		{"wildcard, LAN address", ":3866", ips("127.0.0.1", "192.168.0.9"), "https://192.168.0.9:3866", false},
		{"0.0.0.0 wildcard", "0.0.0.0:3866", ips("10.1.2.3"), "https://10.1.2.3:3866", false},
		{"[::] wildcard", "[::]:3866", ips("10.1.2.3"), "https://10.1.2.3:3866", false},
		{"private beats CGNAT (Tailscale) and public", ":1", ips("100.100.1.1", "8.8.8.8", "172.20.0.5"), "https://172.20.0.5:1", false},
		{"public beats CGNAT", ":1", ips("100.100.1.1", "8.8.8.8"), "https://8.8.8.8:1", false},
		{"CGNAT when nothing else", ":1", ips("127.0.0.1", "100.100.1.1"), "https://100.100.1.1:1", false},
		{"loopback and link-local skipped", ":1", ips("127.0.0.1", "169.254.1.1", "::1", "fe80::1"), "https://localhost:1", false},
		{"no interface", ":1", nil, "https://localhost:1", false},
		{"named host", "ci.example.org:3866", ips("10.1.2.3"), "https://ci.example.org:3866", false},
		{"IPv4 literal", "10.9.9.9:3866", ips("10.1.2.3"), "https://10.9.9.9:3866", false},
		{"IPv6 literal", "[2001:db8::1]:3866", nil, "https://[2001:db8::1]:3866", false},
		{"no port", "ci.example.org", nil, "", true},
	} {
		got, err := defaultPublicURL(tc.addr, tc.ips)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: defaultPublicURL(%q) = %q, %v; want %q (error %v)", tc.name, tc.addr, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestNodesPairWindowWithdrawnExitsOne(t *testing.T) {
	pairHome(t)
	_, out, errb, done := startPair(t)
	b := waitBanner(t, out)
	addr := hostPort(t, b.url)
	if err := pairing.Withdraw(context.Background(), b.code); err != nil {
		t.Fatal(err)
	}
	if code := exitWithin(t, done, 90*time.Second); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errb.String(), "closed without a node joining") || strings.Contains(out.String(), "A node joined") {
		t.Fatalf("stderr %q stdout %q", errb.String(), out.String())
	}
	if portOpen(addr) {
		t.Fatal("the listener is still open")
	}
}

func TestNodesPairExitCodes(t *testing.T) {
	var null bytes.Buffer
	run := func(args ...string) (int, string, string) {
		var o, e pairBuf
		return runNodesPair(args, &null, &o, &e), o.String(), e.String()
	}
	pairHome(t)
	if code, _, errs := run("--help"); code != 0 || !strings.Contains(errs, "enroll-addr") {
		t.Fatalf("--help: %d %q", code, errs)
	} else {
		for _, f := range []string{"enroll-addr", "data-dir", "public-url", "label", "coordinator-id", "agent-url", "cert-ttl"} {
			if !strings.Contains(errs, "-"+f) {
				t.Errorf("usage lacks --%s", f)
			}
		}
		if strings.Contains(strings.ToLower(errs), "-code") {
			t.Error("a code flag exists")
		}
	}
	for name, args := range map[string][]string{
		"unknown flag":      {"--nope"},
		"a positional":      {"extra"},
		"a code flag":       {"--code", "ABCDEFGHJK"},
		"a bad duration":    {"--cert-ttl", "soon"},
		"enroll-addr empty": {"--enroll-addr"},
	} {
		if code, out, errs := run(args...); code != 2 || strings.Contains(out+errs, "ABCDEFGHJK") {
			t.Errorf("%s: exit %d %q", name, code, errs)
		}
	}
	// A store that cannot open is an infrastructure error.
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := run("--data-dir", filepath.Join(file, "sub"), "--enroll-addr", "127.0.0.1:0"); code != 2 || !strings.Contains(errs, "E607") {
		t.Fatalf("unopenable store: %d %q", code, errs)
	}
	// A certificate lifetime outside 1 minute..7 days is E660 cert_ttl and no window opens.
	for _, ttl := range []string{"192h", "30s", "-5m"} {
		pairHome(t)
		code, out, errs := run("--enroll-addr", "127.0.0.1:0", "--public-url", "https://127.0.0.1", "--cert-ttl", ttl)
		if code != 2 || !strings.Contains(errs, "E660") || !strings.Contains(errs, "cert_ttl") || strings.Contains(out, "pairing code") {
			t.Errorf("--cert-ttl %s: exit %d out %q err %q", ttl, code, out, errs)
		}
	}
}

func TestNodesPairIsRegisteredWithItsLifecycleHooks(t *testing.T) {
	e, ok := v15Subcommands["nodes pair"]
	if !ok || !e.V15Only || e.Handler == nil || e.Flags == nil {
		t.Fatalf("entry %+v", e)
	}
	if _, v14 := subcommands["nodes pair"]; v14 {
		t.Fatal("nodes pair leaked into the v1.4 table")
	}
	fs := lifecycle.FlagSet("rotate")
	for _, f := range []string{"cert", "node-key", "key-id", "mode"} {
		if fs.Lookup(f) == nil {
			t.Errorf("rotate has no --%s", f)
		}
	}
	for _, sub := range []string{"add", "revoke"} {
		if lifecycle.FlagSet(sub).Lookup("cert") != nil {
			t.Errorf("--cert leaked into %s", sub)
		}
	}
}

// pullRig is a coordinator store with a CA and one paired pull node that has no ssh destination.
type pullRig struct {
	t        *testing.T
	home     string
	st       *store.Store
	ca       *pki.CA
	reg      *registry.Registry
	svc      *pairing.Service
	out      *pairBuf
	errb     *pairBuf
	env      lifecycle.Env
	certs    []*pairedCert
	nodeID   string
	nodeKey  ed25519.PublicKey
	nodePriv ed25519.PrivateKey
}

type pairedCert struct {
	resp enroll.Response
	key  *ecdsa.PrivateKey
	csr  []byte
}

func newPullRig(t *testing.T) *pullRig {
	t.Helper()
	home := filepath.Join(t.TempDir(), "ci")
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: home})
	if err != nil {
		t.Fatal(err)
	}
	r := &pullRig{t: t, home: home, st: st, ca: ca, reg: registry.New(st), out: &pairBuf{}, errb: &pairBuf{}, nodeID: "node-pull"}
	r.nodeKey, r.nodePriv = ed25519Keys(t)
	r.svc = pairing.New(pairing.Config{CA: ca, Store: st, Registry: r.reg, CoordinatorID: "local", Actor: "cli:test", Approver: autoYes{}})
	r.env = lifecycle.Env{Out: r.out, Err: r.errb, Open: func(context.Context, string) (*lifecycle.Deps, func(), error) {
		return lifecycle.NewDeps(st, home, "proj-1", "cli:test"), func() {}, nil
	}}
	r.pair()
	return r
}

type autoYes struct{}

func (autoYes) Kind() string { return "policy:auto" }
func (autoYes) Approve(context.Context, pairing.ApprovalRequest) (pairing.Decision, error) {
	return pairing.Decision{Approve: true, Reason: "auto", Trust: pairing.DefaultTrust()}, nil
}

func ed25519Pub(t *testing.T) ed25519.PublicKey {
	pub, _ := ed25519Keys(t)
	return pub
}

func ed25519Keys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

// signedRequest is the phase-enroll request of the rig's node, signed with its node key.
func (r *pullRig) signedRequest(code string, csr []byte) enroll.Request {
	fp, err := pki.CSRKeyFingerprint(csr)
	if err != nil {
		r.t.Fatal(err)
	}
	now := time.Now().Unix()
	sig := ed25519.Sign(r.nodePriv, enroll.Transcript("local", r.ca.RootFingerprint(), r.nodeID, now, fp, pki.NodeKeyID(r.nodeKey)))
	return enroll.Request{Phase: enroll.PhaseEnroll, Version: 1, ClientTime: now, Code: code, CSR: csr, NodeKey: r.nodeKey, NodeKeySig: sig, Name: r.nodeID, OS: "linux", Arch: "amd64"}
}

// pair redeems a fresh offer for the rig's node with its pinned node key (the pairing flow without the socket).
func (r *pullRig) pair() *pairedCert {
	r.t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		r.t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		r.t.Fatal(err)
	}
	off, err := r.svc.Offer(context.Background(), pairing.OfferOptions{})
	if err != nil {
		r.t.Fatal(err)
	}
	resp, err := r.svc.Redeem(context.Background(), r.signedRequest(off.Code, csr), "127.0.0.1:1")
	if err != nil {
		r.t.Fatalf("pair: %v", err)
	}
	pc := &pairedCert{resp: resp, key: key, csr: csr}
	r.certs = append(r.certs, pc)
	return pc
}

func (r *pullRig) lifecycle(sub string, args ...string) int {
	r.t.Helper()
	r.out.b.Reset()
	r.errb.b.Reset()
	return lifecycle.Run(sub, args, r.env)
}

func (r *pullRig) denied(serial string) bool {
	d, err := r.st.PKIIsDenied(context.Background(), serial)
	if err != nil {
		r.t.Fatal(err)
	}
	return d
}

func (r *pullRig) serials() []string {
	var out []string
	for _, c := range r.certs {
		x, _ := x509.ParseCertificate(c.resp.Cert)
		out = append(out, hex.EncodeToString(x.SerialNumber.Bytes()))
	}
	return out
}

// handshake is a fresh mTLS handshake of a paired certificate against a node-kind server of the rig's CA.
func (r *pullRig) handshake(c *pairedCert) error {
	r.t.Helper()
	leaf, err := r.ca.IssueServer("local", nil, []net.IP{net.IPv4(127, 0, 0, 1)}, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	cfg, err := r.ca.TLSServerConfig(pki.KindNode, leaf, pki.NewGuard(r.st))
	if err != nil {
		r.t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		r.t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				if conn.(*tls.Conn).Handshake() == nil {
					_, _ = conn.Write([]byte("ok"))
				}
			}()
		}
	}()
	ccfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: r.ca.RootDER(), ServerName: "127.0.0.1",
		Cert: &tls.Certificate{Certificate: [][]byte{c.resp.Cert, c.resp.Chain[1]}, PrivateKey: c.key}})
	if err != nil {
		r.t.Fatal(err)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", ln.Addr().String(), ccfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = io.ReadFull(conn, make([]byte, 2))
	return err
}

func (r *pullRig) count(q string, args ...any) int {
	r.t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(r.home, "state.db")+"?mode=ro")
	if err != nil {
		r.t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err = db.QueryRow(q, args...).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func (r *pullRig) audit(action string) int {
	return r.count(`SELECT count(*) FROM audit WHERE action=?`, action)
}

func TestNodesRotateCertForAPullNodeWithoutSSHDestination(t *testing.T) {
	r := newPullRig(t)
	r.pair() // a re-pair: two certificates of the node
	n, err := r.reg.Get(context.Background(), r.nodeID)
	if err != nil || n.Capability.Identity.Transport != "agent" {
		t.Fatalf("the node is not a pull node: %v %+v", err, n.Capability.Identity)
	}
	for _, c := range r.certs {
		if err = r.handshake(c); err != nil {
			t.Fatalf("positive control: a fresh certificate was refused: %v", err)
		}
	}
	if code := r.lifecycle("rotate", r.nodeID, "--cert"); code != 0 {
		t.Fatalf("rotate --cert exit %d, out %q err %q", code, r.out.String(), r.errb.String())
	}
	if !strings.Contains(r.out.String(), "2 certificate(s) of node-pull denied") {
		t.Fatalf("stdout %q", r.out.String())
	}
	for i, sn := range r.serials() {
		if !r.denied(sn) {
			t.Fatalf("certificate %d is not on the deny list", i)
		}
		if err = r.handshake(r.certs[i]); err == nil {
			t.Fatalf("certificate %d still connects after rotate --cert", i)
		}
	}
	if r.audit("pki.deny") != 1 || r.count(`SELECT count(*) FROM pki_denied WHERE reason='rotate_cert' AND node_id=?`, r.nodeID) != 2 {
		t.Fatalf("deny audit %d", r.audit("pki.deny"))
	}
	if rec, _ := r.reg.Get(context.Background(), r.nodeID); rec.Record.State == "revoked" {
		t.Fatal("rotating a certificate revoked the node")
	}
	// The node pairs again with its pinned node key and works.
	fresh := r.pair()
	if err = r.handshake(fresh); err != nil {
		t.Fatalf("a re-paired node must connect: %v", err)
	}
	// Repeating the rotation denies only the new certificate.
	if code := r.lifecycle("rotate", r.nodeID, "--cert"); code != 0 || !strings.Contains(r.out.String(), "1 certificate(s)") {
		t.Fatalf("second rotate: %d %q", code, r.out.String())
	}
	if code := r.lifecycle("rotate", "nobody", "--cert"); code == 0 {
		t.Fatal("rotating an unknown node succeeded")
	}
}

func TestNodesRotateNodeKeyForAPullNodeWithoutSSHDestination(t *testing.T) {
	r := newPullRig(t)
	next := ed25519Pub(t)
	id := pki.NodeKeyDigest(next)
	if code := r.lifecycle("rotate", r.nodeID, "--node-key"); code == 0 || r.audit("pki.node_key.rotate") != 0 {
		t.Fatalf("--node-key without --key-id: exit %d, audit %d", code, r.audit("pki.node_key.rotate"))
	}
	for _, bad := range []string{"abc", strings.Repeat("0", 17), ""} {
		if code := r.lifecycle("rotate", r.nodeID, "--node-key", "--key-id", bad); code == 0 {
			t.Fatalf("--key-id %q accepted", bad)
		}
	}
	if r.audit("pki.node_key.rotate") != 0 {
		t.Fatal("a refused rotation left an audit row")
	}
	ctx := context.Background()
	if err := r.ca.CheckNodeKey(ctx, r.nodeID, r.nodeKey); err != nil {
		t.Fatalf("positive control: the pinned key: %v", err)
	}
	if code := r.lifecycle("rotate", r.nodeID, "--node-key", "--key-id", id); code != 0 || !strings.Contains(r.out.String(), id) {
		t.Fatalf("rotate --node-key exit %d out %q err %q", code, r.out.String(), r.errb.String())
	}
	if r.audit("pki.node_key.rotate") != 1 {
		t.Fatalf("audit rows %d", r.audit("pki.node_key.rotate"))
	}
	if err := r.ca.CheckNodeKey(ctx, r.nodeID, r.nodeKey); pki.ReasonOf(err) != pki.ReasonNodeKeyChanged {
		t.Fatalf("the old key after rotation: %v", err)
	}
	if err := r.ca.CheckNodeKey(ctx, r.nodeID, ed25519Pub(t)); pki.ReasonOf(err) != pki.ReasonNodeKeyChanged {
		t.Fatalf("an unrelated key after rotation: %v", err)
	}
	if err := r.ca.CheckNodeKey(ctx, r.nodeID, next); err != nil {
		t.Fatalf("the new key: %v", err)
	}
	if err := r.ca.CheckNodeKey(ctx, r.nodeID, r.nodeKey); pki.ReasonOf(err) != pki.ReasonNodeKeyChanged {
		t.Fatalf("the old key once the new one is pinned: %v", err)
	}
}

func TestNodesRevokeDeniesTheCertificatesInsideTheRevokeTransaction(t *testing.T) {
	r := newPullRig(t)
	r.pair()
	other := *r
	other.nodeID, other.certs = "node-bystander", nil
	other.nodeKey, other.nodePriv = ed25519Keys(t)
	other.pair()
	for _, c := range append(append([]*pairedCert(nil), r.certs...), other.certs...) {
		if err := r.handshake(c); err != nil {
			t.Fatalf("positive control: %v", err)
		}
	}
	// A revoker that fails after the PKI revoker rolls the whole transaction back: nothing is denied, the
	// node is not revoked.
	failRevoke.Store(true)
	code := r.lifecycle("revoke", r.nodeID)
	failRevoke.Store(false)
	if code == 0 {
		t.Fatal("a failing revoker did not fail the revoke")
	}
	if got := r.count(`SELECT count(*) FROM pki_denied`); got != 0 {
		t.Fatalf("%d deny rows survived a rolled-back revoke", got)
	}
	if n, _ := r.reg.Get(context.Background(), r.nodeID); n.Record.State == "revoked" {
		t.Fatal("the node is revoked although the transaction failed")
	}
	if err := r.handshake(r.certs[0]); err != nil {
		t.Fatalf("the rolled-back revoke cut the node off: %v", err)
	}
	// The real revoke.
	if code = r.lifecycle("revoke", r.nodeID); code != 0 {
		t.Fatalf("revoke exit %d out %q err %q", code, r.out.String(), r.errb.String())
	}
	for i, sn := range r.serials() {
		if !r.denied(sn) {
			t.Fatalf("certificate %d of the revoked node is not denied", i)
		}
		if err := r.handshake(r.certs[i]); err == nil {
			t.Fatalf("certificate %d still connects", i)
		}
	}
	if got := r.count(`SELECT count(*) FROM pki_denied WHERE reason='node_revoked' AND node_id=?`, r.nodeID); got != 2 {
		t.Fatalf("%d node_revoked rows, want 2", got)
	}
	if got := r.count(`SELECT count(*) FROM pki_denied WHERE node_id=?`, other.nodeID); got != 0 {
		t.Fatalf("%d deny rows for a node that was not revoked", got)
	}
	if err := r.handshake(other.certs[0]); err != nil {
		t.Fatalf("revoking one node cut off another: %v", err)
	}
	if r.audit("node.revoke") != 1 {
		t.Fatalf("node.revoke audit rows %d", r.audit("node.revoke"))
	}
	// A revoked node cannot rotate and cannot pair again.
	if code = r.lifecycle("rotate", r.nodeID, "--cert"); code == 0 {
		t.Fatal("a revoked node rotated its certificate")
	}
	off, err := r.svc.Offer(context.Background(), pairing.OfferOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m := r.pair2(off.Code)
	if pki.ReasonOf(m) != pki.ReasonRevoked {
		t.Fatalf("a revoked node paired again: %v", m)
	}
}

func TestNodesRemoveDeniesTheCertificatesToo(t *testing.T) {
	r := newPullRig(t)
	if code := r.lifecycle("remove", r.nodeID); code != 0 {
		t.Fatalf("remove exit %d out %q err %q", code, r.out.String(), r.errb.String())
	}
	if !r.denied(r.serials()[0]) || r.handshake(r.certs[0]) == nil {
		t.Fatal("a removed node's certificate still works")
	}
	if _, err := r.reg.Get(context.Background(), r.nodeID); err == nil {
		t.Fatal("the node record survived remove")
	}
}

// pair2 redeems code for the rig's node and returns only the error.
func (r *pullRig) pair2(code string) error {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	_, err := r.svc.Redeem(context.Background(), r.signedRequest(code, csr), "127.0.0.1:1")
	return err
}
