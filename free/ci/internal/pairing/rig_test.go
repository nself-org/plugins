package pairing

// Purpose: the shared rig of the pairing tests: a fake clock that the store, the CA, the listener and the
// joining node all read, a real SQLite store and pki.CA under t.TempDir(), fake approvers and the socket
// probes. Nothing here sleeps: the join's rate-limit wait advances the fake clock instead.
// Inputs:  none. Outputs: helpers only. Constraints: every secret-looking value is synthetic.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestMain quiets the standard logger: probing a listener with a bare dial makes net/http log a handshake
// EOF per probe. The one test that asserts on log output installs its own writer.
func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// fakeClock is the one clock of a rig.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock { return &fakeClock{t: time.Now().Truncate(time.Second)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// syncBuf is a goroutine-safe buffer (join output is read while join runs).
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type alertLog struct {
	mu sync.Mutex
	a  []Alert
}

func (l *alertLog) add(a Alert) {
	l.mu.Lock()
	l.a = append(l.a, a)
	l.mu.Unlock()
}

func (l *alertLog) all() []Alert {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Alert(nil), l.a...)
}

type rig struct {
	tb     testing.TB
	clock  *fakeClock
	home   string
	st     *store.Store
	ca     *pki.CA
	reg    *registry.Registry
	svc    *Service
	alerts *alertLog
}

// newRig builds the store, CA and service; mod may change the service Config before New.
func newRig(tb testing.TB, mod func(*Config)) *rig {
	tb.Helper()
	clock := newClock()
	home := filepath.Join(tb.TempDir(), "ci")
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{Clock: clock.Now})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = st.Close() })
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: home, Now: clock.Now})
	if err != nil {
		tb.Fatal(err)
	}
	r := &rig{tb: tb, clock: clock, home: home, st: st, ca: ca, reg: registry.New(st), alerts: &alertLog{}}
	cfg := Config{CA: ca, Store: st, Registry: r.reg, CoordinatorID: "local", AgentURL: "https://agent.test:3865", Actor: "cli:test",
		Poll: 20 * time.Millisecond, Alert: r.alerts.add}
	if mod != nil {
		mod(&cfg)
	}
	r.svc = New(cfg)
	return r
}

func (r *rig) offer(o OfferOptions) OfferResult {
	r.tb.Helper()
	res, err := r.svc.Offer(context.Background(), o)
	if err != nil {
		r.tb.Fatalf("Offer: %v", err)
	}
	return res
}

// query opens the database read-only and runs fn (the store has no audit reader).
func (r *rig) query(q string, args ...any) *sql.Rows {
	r.tb.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(r.home, "state.db")+"?mode=ro")
	if err != nil {
		r.tb.Fatal(err)
	}
	r.tb.Cleanup(func() { _ = db.Close() })
	rows, err := db.Query(q, args...)
	if err != nil {
		r.tb.Fatal(err)
	}
	return rows
}

func (r *rig) count(q string, args ...any) int {
	r.tb.Helper()
	rows := r.query(q, args...)
	defer rows.Close()
	var n int
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			r.tb.Fatal(err)
		}
	}
	return n
}

type auditRow struct{ Actor, Action, Target, Detail string }

// audit returns the audit rows of one action ("" = all), oldest first.
func (r *rig) audit(action string) []auditRow {
	r.tb.Helper()
	q, args := `SELECT actor,action,target,detail FROM audit ORDER BY seq`, []any{}
	if action != "" {
		q, args = `SELECT actor,action,target,detail FROM audit WHERE action=? ORDER BY seq`, []any{action}
	}
	rows := r.query(q, args...)
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var a auditRow
		if err := rows.Scan(&a.Actor, &a.Action, &a.Target, &a.Detail); err != nil {
			r.tb.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

// noCodeInAudit fails when any audit row, or the offer table, holds code (or its normalized form).
func (r *rig) noCodeInAudit(code string) {
	r.tb.Helper()
	for _, a := range r.audit("") {
		for _, f := range []string{a.Actor, a.Action, a.Target, a.Detail} {
			if bytes.Contains([]byte(f), []byte(Normalize(code))) || bytes.Contains([]byte(f), []byte(code)) {
				r.tb.Fatalf("audit row %+v holds the pairing code", a)
			}
		}
	}
}

// nodeMat is the key material of a joining node, for tests that call Redeem directly.
type nodeMat struct {
	key   *ecdsa.PrivateKey
	csr   []byte
	pub   ed25519.PublicKey
	priv  ed25519.PrivateKey
	csrFP string
}

func newNodeMat(tb testing.TB) nodeMat {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		tb.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	fp, err := pki.CSRKeyFingerprint(csr)
	if err != nil {
		tb.Fatal(err)
	}
	return nodeMat{key: key, csr: csr, pub: pub, priv: priv, csrFP: fp}
}

// withNodeKeyOf returns m with the node key (both halves) of o: the same node, a new TLS key and CSR.
func (m nodeMat) withNodeKeyOf(o nodeMat) nodeMat {
	m.pub, m.priv = o.pub, o.priv
	return m
}

// sign signs the enrollment transcript with the node key of m.
func (m nodeMat) sign(coordinatorID, rootFP, name string, clientTime int64) []byte {
	return ed25519.Sign(m.priv, enroll.Transcript(coordinatorID, rootFP, name, clientTime, m.csrFP, pki.NodeKeyID(m.pub)))
}

// request is the phase-enroll request of m under the rig clock, signed with the node key.
func (r *rig) request(code, name string, m nodeMat) enroll.Request {
	return r.requestAt(code, name, m, r.clock.Now().Unix())
}

func (r *rig) requestAt(code, name string, m nodeMat, clientTime int64) enroll.Request {
	return enroll.Request{Phase: enroll.PhaseEnroll, Version: enroll.Version, ClientTime: clientTime, Code: code,
		CSR: m.csr, NodeKey: m.pub, NodeKeySig: m.sign("local", r.ca.RootFingerprint(), name, clientTime), Name: name, OS: "linux", Arch: "amd64", MachineID: "machine-1", AgentVersion: "1.5.0"}
}

func (r *rig) redeem(code, name string, m nodeMat) (enroll.Response, error) {
	return r.svc.Redeem(context.Background(), r.request(code, name, m), "127.0.0.1:50000")
}

// joinOpts are the options of a scripted node (no terminal) against url.
func (r *rig) joinOpts(url, code, fp, name string, out *syncBuf) (enroll.JoinOptions, nodeMat) {
	m := newNodeMat(r.tb)
	return enroll.JoinOptions{
		URL: url, ExpectFingerprint: fp, Name: name, OS: "linux", Arch: "amd64", MachineID: "machine-1", AgentVersion: "1.5.0",
		NodeKey: m.pub, NodeKeyID: pki.NodeKeyID(m.pub), NodeSign: func(msg []byte) ([]byte, error) { return ed25519.Sign(m.priv, msg), nil },
		Out: out, Now: r.clock.Now,
		Code:  func() (string, error) { return code, nil },
		Sleep: r.clock.Advance,
	}, m
}

// join runs enroll.Join for a scripted node; the rate-limit clock is moved on first so earlier requests
// of the same source never count against this one.
func (r *rig) join(url, code, fp, name string) (enroll.JoinResult, error) {
	r.tb.Helper()
	r.clock.Advance(2 * time.Second)
	opts, _ := r.joinOpts(url, code, fp, name, &syncBuf{})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return enroll.Join(ctx, opts)
}

// window opens a foreground pairing window on 127.0.0.1:0 and returns it with a Wait result channel.
func (r *rig) window(o OfferOptions) (*Window, <-chan waitResult) {
	r.tb.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r.tb.Cleanup(cancel)
	w, err := r.svc.OpenWindow(ctx, "127.0.0.1:0", "https://127.0.0.1", o)
	if err != nil {
		r.tb.Fatal(err)
	}
	ch := make(chan waitResult, 1)
	go func() {
		ok, err := w.Wait(ctx, r.svc)
		ch <- waitResult{ok, err}
	}()
	return w, ch
}

type waitResult struct {
	redeemed bool
	err      error
}

func (r *rig) waitFor(ch <-chan waitResult, within time.Duration) waitResult {
	r.tb.Helper()
	select {
	case w := <-ch:
		return w
	case <-time.After(within):
		r.tb.Fatal("the pairing window did not end")
	}
	return waitResult{}
}

// fakeApprover records every request and answers with a fixed decision.
type fakeApprover struct {
	kind string
	dec  Decision
	err  error
	mu   sync.Mutex
	got  []ApprovalRequest
}

func approving(kind string, t TrustFields) *fakeApprover {
	return &fakeApprover{kind: kind, dec: Decision{Approve: true, Reason: "ok by " + kind, Trust: t}}
}

func denying(kind string) *fakeApprover {
	return &fakeApprover{kind: kind, dec: Decision{Reason: "no by " + kind}}
}

func (f *fakeApprover) Kind() string { return f.kind }

func (f *fakeApprover) Approve(_ context.Context, r ApprovalRequest) (Decision, error) {
	f.mu.Lock()
	f.got = append(f.got, r)
	f.mu.Unlock()
	return f.dec, f.err
}

func (f *fakeApprover) calls() []ApprovalRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ApprovalRequest(nil), f.got...)
}

// freeAddr returns a loopback address whose port was free a moment ago.
func freeAddr(tb testing.TB) string {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

// isOpen reports whether something accepts TCP connections at addr.
func isOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// eventually polls cond until it holds or within passes.
func eventually(tb testing.TB, within time.Duration, what string, cond func() bool) {
	tb.Helper()
	for end := time.Now().Add(within); ; time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
		if time.Now().After(end) {
			tb.Fatalf("not within %s: %s", within, what)
		}
	}
}

// runJoin runs enroll.Join with a 30 s ceiling.
func runJoin(opts enroll.JoinOptions) (enroll.JoinResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return enroll.Join(ctx, opts)
}

// coded returns the fabric code and reason of an E659 or E660 error from either side of the wire.
func coded(err error) (code, reason string) {
	var ce enroll.CodedError
	if errors.As(err, &ce) {
		return ce.ErrCode(), ce.ErrReason()
	}
	return "", ""
}

// wantCoded fails unless err carries the fabric code and reason.
func wantCoded(tb testing.TB, err error, code, reason string) {
	tb.Helper()
	if c, rs := coded(err); c != code || rs != reason {
		tb.Fatalf("want %s %s, got %q %q (%v)", code, reason, c, rs, err)
	}
}

// tbListen holds addr with a plain listener (a port someone else owns).
func (r *rig) tbListen(addr string) net.Listener {
	r.tb.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		r.tb.Fatal(err)
	}
	return ln
}

// exec runs a statement on a second read-write connection (the store has no hook for failure injection).
func (r *rig) exec(q string) {
	r.tb.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(r.home, "state.db")+"?_pragma=busy_timeout(10000)")
	if err != nil {
		r.tb.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(q); err != nil {
		r.tb.Fatal(err)
	}
}

func jsonDecode(r io.Reader, v any) error { return json.NewDecoder(r).Decode(v) }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
