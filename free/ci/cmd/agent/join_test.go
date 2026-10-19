package main

// Purpose: `nself-ci-agent join` end to end against a real pairing window on loopback: enrollment saved,
// no terminal and no flag refused, the code never on argv, in the environment or in output, clock skew,
// hooks. Real TLS, a real pki.CA over a real store; the only real wait is join's own 1.1 s rate-limit pause.
// Inputs:  scripted stdin. Outputs: pass/fail. Constraints: the CA clock is shifted for the skew case only.

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type joinApprover struct {
	mu    sync.Mutex
	calls []pairing.ApprovalRequest
	yes   bool
}

func (a *joinApprover) Kind() string { return "policy:join-test" }

func (a *joinApprover) Approve(_ context.Context, r pairing.ApprovalRequest) (pairing.Decision, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, r)
	return pairing.Decision{Approve: a.yes, Reason: "scripted", Trust: pairing.DefaultTrust()}, nil
}

func (a *joinApprover) n() int { return len(a.snap()) }

func (a *joinApprover) snap() []pairing.ApprovalRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]pairing.ApprovalRequest(nil), a.calls...)
}

// coordinator is a pairing window on 127.0.0.1:0 with one offer.
type coordinator struct {
	svc  *pairing.Service
	st   *store.Store
	ca   *pki.CA
	ap   *joinApprover
	code string
	fp   string
	url  string
}

func newCoordinator(t *testing.T, caShift time.Duration) *coordinator {
	t.Helper()
	home := filepath.Join(t.TempDir(), "ci")
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: home, Now: func() time.Time { return time.Now().Add(caShift) }})
	if err != nil {
		t.Fatal(err)
	}
	c := &coordinator{st: st, ca: ca, ap: &joinApprover{yes: true}}
	c.svc = pairing.New(pairing.Config{CA: ca, Store: st, Registry: registry.New(st), CoordinatorID: "local", AgentURL: "https://agent.test:3865", Actor: "cli:test", Poll: 20 * time.Millisecond})
	c.url, c.code, c.fp = c.window(t)
	return c
}

// window opens another pairing window (its own listener and code) on the same coordinator.
func (c *coordinator) window(t *testing.T) (url, code, fp string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w, err := c.svc.OpenWindow(ctx, "127.0.0.1:0", "https://127.0.0.1", pairing.OfferOptions{Approver: c.ap})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = w.Wait(ctx, c.svc) }()
	return w.URL, w.Offer.Code, w.Offer.RootFingerprint
}

// join runs the subcommand with stdin and returns the exit code and both streams.
func joinRun(t *testing.T, stdin *os.File, in string, args ...string) (code int, stdout, stderr string, rest int) {
	t.Helper()
	var out, errb bytes.Buffer
	var r *strings.Reader
	sio := IO{Stdout: &out, Stderr: &errb}
	if stdin != nil {
		sio.Stdin = stdin
	} else {
		r = strings.NewReader(in)
		sio.Stdin = r
	}
	code = run(context.Background(), append([]string{"join"}, args...), sio)
	if r != nil {
		rest = r.Len()
	}
	return code, out.String(), errb.String(), rest
}

var blockJoin, failJoinHook atomic.Bool

func init() {
	must(RegisterPreStart("test-join-block", func(_ context.Context, command string) error {
		if blockJoin.Load() && command == "join" {
			return errors.New("host refused by the test guard")
		}
		return nil
	}))
	must(pairing.RegisterJoinHook("test-join-hook", func(_ context.Context, r enroll.JoinResult, state string) error {
		joinHookSeen.Store(&hookCall{r.NodeID, state})
		if failJoinHook.Load() {
			return errors.New("hook failed")
		}
		return nil
	}))
}

type hookCall struct{ node, state string }

var joinHookSeen atomic.Pointer[hookCall]

func TestJoinEnrollsAndSavesTheCredential(t *testing.T) {
	state := useState(t)
	c := newCoordinator(t, 0)
	joinHookSeen.Store(nil)
	code, stdout, stderr, rest := joinRun(t, nil, c.code+"\n", "--expect-fingerprint", c.fp, "--name", "Node-J", c.url)
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Joined as node-j.") || rest != 0 {
		t.Fatalf("stdout %q, unread stdin %d", stdout, rest)
	}
	cred, err := enroll.LoadCredential(state)
	if err != nil || cred.Enrollment.NodeID != "node-j" || cred.Enrollment.AgentURL != "https://agent.test:3865" || cred.Enrollment.RootFingerprint != c.fp {
		t.Fatalf("credential %+v %v", cred.Enrollment, err)
	}
	if pki.CertFingerprint(cred.RootDER) != c.fp {
		t.Fatal("the stored root is not the confirmed one")
	}
	// The node key join sent is the agent's persisted key, pinned by the coordinator.
	key, err := nodekey.LoadOrCreate(state)
	if err != nil {
		t.Fatal(err)
	}
	pinned, _, ok, err := c.st.PKINodeKeyGet(context.Background(), "node-j")
	if err != nil || !ok || pinned != key.KeyID() || pinned != pki.NodeKeyID(key.Public()) {
		t.Fatalf("pinned %q ok %v err %v, agent key id %q", pinned, ok, err, key.KeyID())
	}
	if got := c.ap.snap(); len(got) != 1 || got[0].NodeKeyID != key.KeyID() || got[0].NodeName != "node-j" {
		t.Fatalf("approver saw %+v", got)
	}
	// The sealed client key, the KEK next to it, nothing else secret in plain.
	filepath.WalkDir(state, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			raw, _ := os.ReadFile(p)
			if bytes.Contains(raw, []byte(c.code)) {
				t.Errorf("%s holds the code", p)
			}
			if strings.Contains(string(raw), "PRIVATE KEY") && !strings.Contains(filepath.Base(p), "key") {
				t.Errorf("%s holds a PEM private key", p)
			}
		}
		return nil
	})
	if _, err = os.Stat(filepath.Join(state, "pki", "kek")); err != nil {
		t.Fatal(err)
	}
	// Join hooks ran after the credential was saved, with the state dir.
	if h := joinHookSeen.Load(); h == nil || h.node != "node-j" || h.state != state {
		t.Fatalf("join hook saw %+v", h)
	}
	// The code appears in no output and not in the process environment.
	for _, s := range []string{stdout, stderr} {
		if strings.Contains(s, c.code) {
			t.Fatalf("the code was printed: %q", s)
		}
	}
	for _, e := range os.Environ() {
		if strings.Contains(e, c.code) {
			t.Fatal("the code is in the environment")
		}
	}
}

func TestJoinWithoutTTYAndWithoutFlagRefuses(t *testing.T) {
	state := useState(t)
	c := newCoordinator(t, 0)
	code, stdout, stderr, rest := joinRun(t, nil, c.code+"\n", "--name", "node-j", c.url)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "E660") || !strings.Contains(stderr, "fingerprint_unconfirmed") {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
	if rest != len(c.code)+1 || strings.Contains(stderr, "Pairing code") || strings.Contains(stderr, "CSR key fingerprint") {
		t.Fatalf("join read or asked for the code before confirming (unread %d): %q", rest, stderr)
	}
	assertNothingHappened(t, c, state)

	// /dev/null is a character device, so join treats it as a terminal: the prompt reads EOF and refuses.
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	code, _, stderr, _ = joinRun(t, null, "", "--name", "node-j", c.url)
	if code != 1 || !strings.Contains(stderr, "Type yes to trust it") || !strings.Contains(stderr, "fingerprint_unconfirmed") || !strings.Contains(stderr, c.fp) {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
	assertNothingHappened(t, c, state)
}

func assertNothingHappened(t *testing.T, c *coordinator, state string) {
	t.Helper()
	if _, err := enroll.LoadCredential(state); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a credential exists: %v", err)
	}
	live, _ := c.st.PKIOffersLive(context.Background(), false)
	if c.ap.n() != 0 || len(live) != 1 || live[0].Attempts != 0 {
		t.Fatalf("the coordinator saw a request: approver %d live %+v", c.ap.n(), live)
	}
}

func TestJoinMismatchedFingerprintSendsNothing(t *testing.T) {
	state := useState(t)
	c := newCoordinator(t, 0)
	other := newCoordinator(t, 0)
	for _, fp := range []string{other.fp, c.fp[:len(c.fp)-1] + "x", strings.ToLower(c.fp)} {
		code, _, stderr, rest := joinRun(t, nil, c.code+"\n", "--expect-fingerprint", fp, "--name", "node-j", c.url)
		if code != 1 || !strings.Contains(stderr, "fingerprint_mismatch") || strings.Contains(stderr, "CSR key") || rest != len(c.code)+1 {
			t.Fatalf("fp %q: exit %d stderr %q unread %d", fp, code, stderr, rest)
		}
	}
	assertNothingHappened(t, c, state)
}

func TestJoinHasNoCodeFlagAndTakesNoCodeOnArgv(t *testing.T) {
	useState(t)
	code, _, stderr, _ := joinRun(t, nil, "", "--help")
	if code != 0 {
		t.Fatalf("--help exit %d", code)
	}
	flags := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s+-(\S+)`).FindAllStringSubmatch(stderr, -1) {
		flags[m[1]] = true
	}
	if !flags["expect-fingerprint"] || !flags["name"] || len(flags) != 2 {
		t.Fatalf("flags %v\n%s", flags, stderr)
	}
	for f := range flags {
		if strings.Contains(strings.ToLower(f), "code") || strings.Contains(strings.ToLower(f), "token") || strings.Contains(strings.ToLower(f), "secret") {
			t.Fatalf("join has a secret-bearing flag %q", f)
		}
	}
	if strings.Contains(strings.ToLower(stderr), "pairing code") && strings.Contains(stderr, "--code") {
		t.Fatalf("usage mentions a code flag: %s", stderr)
	}
	const secret = "ABCDEFGHJK"
	for _, args := range [][]string{{"--code", secret, "https://127.0.0.1:1"}, {"--code=" + secret, "https://127.0.0.1:1"}, {"-pairing-code", secret, "https://127.0.0.1:1"}} {
		code, _, stderr, _ = joinRun(t, nil, "", args...)
		if code != 2 || strings.Contains(stderr, secret) || !strings.Contains(stderr, "not defined") {
			t.Fatalf("%v: exit %d stderr %q", args, code, stderr)
		}
	}
	for _, args := range [][]string{{}, {"a", "b"}} {
		if code, _, stderr, _ = joinRun(t, nil, "", args...); code != 2 || !strings.Contains(stderr, "usage: nself-ci-agent join") {
			t.Fatalf("%v: exit %d stderr %q", args, code, stderr)
		}
	}
}

func TestJoinWrongCodeIsRefusedWithoutEchoingIt(t *testing.T) {
	state := useState(t)
	c := newCoordinator(t, 0)
	const wrong = "ZZZZZZZZZZ"
	code, _, stderr, _ := joinRun(t, nil, wrong+"\n", "--expect-fingerprint", c.fp, "--name", "node-j", c.url)
	if code != 1 || !strings.Contains(stderr, "E659") || !strings.Contains(stderr, "invalid") || strings.Contains(stderr, wrong) {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
	if _, err := enroll.LoadCredential(state); !errors.Is(err, fs.ErrNotExist) || c.ap.n() != 0 {
		t.Fatalf("credential %v approver %d", err, c.ap.n())
	}
}

func TestJoinRefusesAClockSkewOver120Seconds(t *testing.T) {
	for _, shift := range []time.Duration{300 * time.Second, -300 * time.Second} {
		state := useState(t)
		c := newCoordinator(t, shift)
		code, _, stderr, rest := joinRun(t, nil, c.code+"\n", "--expect-fingerprint", c.fp, "--name", "node-j", c.url)
		if code != 1 || !strings.Contains(stderr, "E660") || !strings.Contains(stderr, "clock_skew") || strings.Contains(stderr, "CSR key") || rest != len(c.code)+1 {
			t.Fatalf("shift %s: exit %d stderr %q unread %d", shift, code, stderr, rest)
		}
		assertNothingHappened(t, c, state)
	}
}

func TestJoinStopsAtAFailingPreStartHook(t *testing.T) {
	state := useState(t)
	c := newCoordinator(t, 0)
	blockJoin.Store(true)
	defer blockJoin.Store(false)
	code, _, stderr, rest := joinRun(t, nil, c.code+"\n", "--expect-fingerprint", c.fp, c.url)
	if code != 1 || !strings.Contains(stderr, "host refused by the test guard") || rest != len(c.code)+1 {
		t.Fatalf("exit %d stderr %q unread %d", code, stderr, rest)
	}
	if _, err := os.Stat(state); err == nil {
		t.Fatal("the state dir was created before the guard passed")
	}
	assertNothingHappened(t, c, state)
}

func TestJoinReportsAFailingJoinHookAfterSaving(t *testing.T) {
	state := useState(t)
	c := newCoordinator(t, 0)
	failJoinHook.Store(true)
	defer failJoinHook.Store(false)
	code, stdout, stderr, _ := joinRun(t, nil, c.code+"\n", "--expect-fingerprint", c.fp, "--name", "node-j", c.url)
	if code != 1 || !strings.Contains(stderr, "enrolled, but join hook test-join-hook: hook failed") || !strings.Contains(stdout, "Joined as node-j") {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
	if _, err := enroll.LoadCredential(state); err != nil {
		t.Fatalf("the enrollment must be saved before the hooks run: %v", err)
	}
}

func TestJoinCannotChangeTheNodeKeyOfAKnownNode(t *testing.T) {
	useState(t)
	c := newCoordinator(t, 0)
	if code, _, stderr, _ := joinRun(t, nil, c.code+"\n", "--expect-fingerprint", c.fp, "--name", "node-j", c.url); code != 0 {
		t.Fatalf("first join: %d %s", code, stderr)
	}
	// Same node, same key, a second window: allowed (the node re-pairs).
	url2, code2, _ := c.window(t)
	if code, _, stderr, _ := joinRun(t, nil, code2+"\n", "--expect-fingerprint", c.fp, "--name", "node-j", url2); code != 0 {
		t.Fatalf("re-pair with the same key: %d %s", code, stderr)
	}
	// Same name, a new key (fresh state): refused, E660 node_key_changed.
	url3, code3, _ := c.window(t)
	useState(t)
	code, _, stderr, _ := joinRun(t, nil, code3+"\n", "--expect-fingerprint", c.fp, "--name", "node-j", url3)
	if code != 1 || !strings.Contains(stderr, "E660") || !strings.Contains(stderr, "node_key_changed") {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
}
