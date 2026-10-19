package coordinator

// Purpose: tests of the V15 coordinator (P7-NODE-10): delivery and semantic dedup, idempotency keys,
// the v1.4 refusals, /healthz, the Reconciler schedule, listeners, the start guard and the kill -9 fault.
// Inputs:  a temporary store, a fake clock, signed fake GitHub payloads.
// Outputs: pass or fail.
// Constraints: no network beyond 127.0.0.1; time moves only through the fake clock; subtests are named
//          after the acceptance line they prove.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

const testSecret = "test-secret-not-real"

// fakeClock fires After channels only when Advance passes their deadline.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
}
type fakeWaiter struct {
	at time.Time
	ch chan time.Time
}

func newClock() *fakeClock { return &fakeClock{now: time.Unix(2_000_000, 0)} }
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.waiters = append(c.waiters, fakeWaiter{c.now.Add(d), ch})
	return ch
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	keep := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.at.After(c.now) {
			w.ch <- c.now
		} else {
			keep = append(keep, w)
		}
	}
	c.waiters = keep
}
func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

func openStore(t *testing.T, clk *fakeClock) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

type rig struct {
	c    *Coordinator
	st   *store.Store
	clk  *fakeClock
	logs *lockedBuf
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) add(s string) { l.mu.Lock(); l.b.WriteString(s + "\n"); l.mu.Unlock() }
func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func newRig(t *testing.T, edit func(*Config)) *rig {
	t.Helper()
	clk, logs := newClock(), &lockedBuf{}
	cfg := Config{Store: openStore(t, clk), Secret: testSecret, AllowedRepos: []string{"o/r"}, PolicyDigest: "pd", Clock: clk,
		Logf: func(f string, a ...any) { logs.add(strings.TrimSpace(fmt.Sprintf(f, a...))) }}
	if edit != nil {
		edit(&cfg)
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &rig{c, cfg.Store, clk, logs}
}

func sign(body []byte) string {
	m := hmac.New(sha256.New, []byte(testSecret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func pushBody(sha string) []byte {
	b, _ := json.Marshal(map[string]any{"ref": "refs/heads/main", "after": sha,
		"repository": map[string]string{"full_name": "o/r", "clone_url": "https://github.com/o/r.git"}})
	return b
}

func prBody(sha string) []byte {
	b, _ := json.Marshal(map[string]any{"action": "synchronize", "pull_request": map[string]any{"head": map[string]any{
		"sha": sha, "ref": "feature", "repo": map[string]string{"full_name": "o/r", "clone_url": "https://github.com/o/r.git"}}}})
	return b
}

func (r *rig) post(path, event, delivery string, body []byte, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-GitHub-Delivery", delivery)
	req.Header.Set("X-Hub-Signature-256", sign(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	r.c.Handler(true).ServeHTTP(w, req)
	return w
}

func outcome(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("answer %d %q is not JSON", w.Code, w.Body.String())
	}
	return m["outcome"]
}

func queued(t *testing.T, st *store.Store) int {
	t.Helper()
	s, err := st.IntakeStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s.Queued
}

func TestDedupDeliveryID(t *testing.T) {
	r := newRig(t, nil)
	body := pushBody("abc1234def")
	if w := r.post("/webhook", "push", "dlv-1", body); w.Code != 202 || outcome(t, w) != "enqueued" {
		t.Fatalf("first = %d %s", w.Code, w.Body)
	}
	if w := r.post("/webhook", "push", "dlv-1", body); w.Code != 200 || outcome(t, w) != "duplicate" {
		t.Fatalf("same delivery id = %d %s, want 200 duplicate", w.Code, w.Body)
	}
	if n := queued(t, r.st); n != 1 {
		t.Fatalf("queued = %d, want 1", n)
	}
}

func TestDedupSemanticCoalesceAndForce(t *testing.T) {
	r := newRig(t, nil)
	sha := "abc1234def"
	first := r.post("/webhook", "push", "p1", pushBody(sha))
	pr := r.post("/", "pull_request", "p2", prBody(sha))
	if outcome(t, pr) != "coalesced" {
		t.Fatalf("PR synchronize for the same SHA = %s", pr.Body)
	}
	manual := r.post("/v1/trigger", "", "", []byte(`{"project":"o/r","revision":"`+sha+`"}`), "Idempotency-Key", "manual-key-1")
	if outcome(t, manual) != "coalesced" {
		t.Fatalf("manual trigger for the same SHA = %d %s", manual.Code, manual.Body)
	}
	if n := queued(t, r.st); n != 1 {
		t.Fatalf("push, PR and manual made %d pipelines, want 1", n)
	}
	forced := r.post("/v1/trigger", "", "", []byte(`{"project":"o/r","revision":"`+sha+`","force":true}`), "Idempotency-Key", "manual-key-2")
	if outcome(t, forced) != "enqueued" || forced.Code != 202 {
		t.Fatalf("--force = %d %s, want a second pipeline", forced.Code, forced.Body)
	}
	if n := queued(t, r.st); n != 2 {
		t.Fatalf("after --force queued = %d, want 2", n)
	}
	var ids [2]string
	for i, w := range []*httptest.ResponseRecorder{first, forced} {
		var m map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		ids[i] = m["intake"]
	}
	row, err := r.st.IntakeGet(context.Background(), ids[1])
	if err != nil || !row.Forced || ids[0] == ids[1] {
		t.Fatalf("forced row = %+v, %v (ids %v)", row, err, ids)
	}
}

func TestDedupConfigForce(t *testing.T) {
	r := newRig(t, func(c *Config) { c.Force = true })
	r.post("/webhook", "push", "f1", pushBody("abc1234def"))
	if w := r.post("/webhook", "push", "f2", pushBody("abc1234def")); outcome(t, w) != "enqueued" {
		t.Fatalf("with --force a second trigger = %s", w.Body)
	}
}

func TestDedupRetentionHorizon(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	body := pushBody("abc1234def")
	r.post("/webhook", "push", "h1", body)
	row, ok, err := r.st.IntakeClaim(ctx, "w")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = r.st.IntakeFinish(ctx, row.ID, "w", "run-1", "exit=0", true); err != nil {
		t.Fatal(err)
	}
	r.clk.Advance(71 * time.Hour)
	if w := r.post("/webhook", "push", "h2", body); outcome(t, w) != "coalesced" {
		t.Fatalf("webhook for a pipeline completed 71 h ago = %s, want coalesced", w.Body)
	}
	r.c.cfg.Force = true
	if w := r.post("/webhook", "push", "h3", body); outcome(t, w) != "enqueued" {
		t.Fatalf("--force after 71 h = %s, want enqueued", w.Body)
	}
}

func TestIdempotencyKeyAPI(t *testing.T) {
	r := newRig(t, nil)
	req := []byte(`{"project":"o/r","revision":"abc1234def"}`)
	a := r.post("/v1/trigger", "", "", req, "Idempotency-Key", "same-key-001")
	b := r.post("/v1/trigger", "", "", req, "Idempotency-Key", "same-key-001")
	if a.Code != 202 || b.Code != 200 || outcome(t, b) != "replay" {
		t.Fatalf("replay = %d/%d %s", a.Code, b.Code, b.Body)
	}
	other := []byte(`{"project":"o/r","revision":"abc1234def","pipeline":"deploy"}`)
	if w := r.post("/v1/trigger", "", "", other, "Idempotency-Key", "same-key-001"); w.Code != 409 {
		t.Fatalf("key reused for another request = %d, want 409", w.Code)
	}
	if w := r.post("/v1/trigger", "", "", req); w.Code != 400 {
		t.Fatalf("no Idempotency-Key = %d, want 400", w.Code)
	}
}

func TestIntakeRefusalsMatchV14(t *testing.T) {
	r := newRig(t, nil)
	body := pushBody("abc1234def")
	if w := r.post("/webhook", "push", "u1", body, "X-Hub-Signature-256", "sha256=00"); w.Code != 403 {
		t.Fatalf("bad signature = %d, want 403", w.Code)
	}
	other, _ := json.Marshal(map[string]any{"ref": "refs/heads/m", "after": "abc1234def", "repository": map[string]string{"full_name": "x/y"}})
	if w := r.post("/webhook", "push", "u2", other); w.Code != 403 {
		t.Fatalf("repo not allowlisted = %d, want 403", w.Code)
	}
	if w := r.post("/webhook", "ping", "u3", []byte(`{}`)); w.Code != 200 || strings.Contains(w.Body.String(), `"true"`) {
		t.Fatalf("uninteresting event = %d %s, want a quiet 200", w.Code, w.Body)
	}
	if n := queued(t, r.st); n != 0 {
		t.Fatalf("refused deliveries left %d rows", n)
	}
}

func TestIntakeStoreDownGives503(t *testing.T) {
	r := newRig(t, nil)
	_ = r.st.Close()
	if w := r.post("/webhook", "push", "s1", pushBody("abc1234def")); w.Code != 503 {
		t.Fatalf("store closed = %d, want 503", w.Code)
	}
}

func TestHealthzBodyByBind(t *testing.T) {
	r := newRig(t, nil)
	r.post("/webhook", "push", "z1", pushBody("abc1234def"))
	get := func(loop bool, hdr ...string) map[string]any {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = "127.0.0.1:50000"
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		r.c.Handler(loop).ServeHTTP(w, req)
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return m
	}
	if m := get(true); m["queue_depth"] != float64(1) || m["leases"] == nil || m["status"] != "ok" {
		t.Fatalf("loopback body = %v", m)
	}
	if m := get(false); m["queue_depth"] != nil || m["leases"] != nil || m["status"] != "ok" || m["service"] != "nself-ci-serve" {
		t.Fatalf("non-loopback body = %v, want only the v1.4 fields", m)
	}
	if m := get(true, "X-Forwarded-For", "203.0.113.9"); m["queue_depth"] != nil || m["status"] != "ok" {
		t.Fatalf("a request relayed by a proxy got the loopback body: %v", m)
	}
	for addr, want := range map[string]bool{"127.0.0.1:3845": true, "[::1]:80": true, "0.0.0.0:3845": false, ":3845": false, "10.0.0.5:1": false} {
		if isLoopback(addr) != want {
			t.Fatalf("isLoopback(%s) != %v", addr, want)
		}
	}
}

func TestAccessLogRedactsAuthorization(t *testing.T) {
	r := newRig(t, nil)
	r.post("/webhook", "push", "l1", pushBody("abc1234def"), "Authorization", "Bearer super-secret-token")
	r.post("/healthz?token=abc", "", "", nil)
	logs := r.logs.String()
	if !strings.Contains(logs, "POST /webhook 202") {
		t.Fatalf("no access line in %q", logs)
	}
	for _, leak := range []string{"super-secret-token", "Authorization", testSecret, "token=abc"} {
		if strings.Contains(logs, leak) {
			t.Fatalf("access log leaked %q: %s", leak, logs)
		}
	}
}

// --- Reconciler schedule ---------------------------------------------------------------------------

type fakeRec struct {
	name  string
	every time.Duration // 0: no Every method answer
	calls atomic.Int32
	run   func(ctx context.Context, enq Enqueuer)
}

func (f *fakeRec) Name() string         { return f.name }
func (f *fakeRec) Every() time.Duration { return f.every }
func (f *fakeRec) Reconcile(ctx context.Context, enq Enqueuer) error {
	f.calls.Add(1)
	if f.run != nil {
		f.run(ctx, enq)
	}
	return nil
}

// start runs the coordinator on a loopback listener and returns a stop function.
func (r *rig) start(t *testing.T) (stop func()) {
	t.Helper()
	r.c.cfg.Addr = "127.0.0.1:0"
	r.c.cfg.Poll, r.c.cfg.Beat = 5*time.Millisecond, time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.c.Run(ctx) }()
	return func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Run did not stop")
		}
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(2 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestReconcilerSchedule(t *testing.T) {
	def, fast := &fakeRec{name: "default"}, &fakeRec{name: "fast", every: 30 * time.Second}
	tiny := &fakeRec{name: "tiny", every: 5 * time.Second}
	r := newRig(t, func(c *Config) { c.Reconcilers = []Reconciler{def, fast, tiny} })
	defer r.start(t)()
	waitFor(t, "first passes", func() bool { return def.calls.Load() == 1 && fast.calls.Load() == 1 && tiny.calls.Load() == 1 })
	step := func(d time.Duration, d1, f1, t1 int32) {
		waitFor(t, "timers", func() bool { return r.clk.pending() >= 4 })
		r.clk.Advance(d)
		waitFor(t, "passes", func() bool { return def.calls.Load() == d1 && fast.calls.Load() == f1 && tiny.calls.Load() == t1 })
	}
	step(15*time.Second, 1, 1, 2)               // Every() below 15 s is raised to 15 s
	step(15*time.Second, 1, 2, 3)               // 30 s
	step(4*time.Minute+30*time.Second, 2, 3, 4) // the 5 min default is due now
}

func TestReconcilerSlowPassNeverOverlaps(t *testing.T) {
	var inFlight, peak atomic.Int32
	gate := make(chan struct{})
	slow := &fakeRec{name: "slow", every: 15 * time.Second}
	slow.run = func(ctx context.Context, _ Enqueuer) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		select {
		case <-gate:
		case <-ctx.Done():
		}
		inFlight.Add(-1)
	}
	r := newRig(t, func(c *Config) { c.Reconcilers = []Reconciler{slow} })
	stop := r.start(t)
	defer stop()
	waitFor(t, "the slow pass to start", func() bool { return slow.calls.Load() == 1 })
	for i := 0; i < 3; i++ {
		r.clk.Advance(time.Minute) // several ticks while the pass is still running
	}
	time.Sleep(30 * time.Millisecond)
	if n := slow.calls.Load(); n != 1 {
		t.Fatalf("a second pass started while the first was running (calls=%d)", n)
	}
	close(gate)
	waitFor(t, "the next timer", func() bool { return r.clk.pending() >= 2 })
	r.clk.Advance(15 * time.Second)
	waitFor(t, "the second pass", func() bool { return slow.calls.Load() == 2 })
	if peak.Load() != 1 {
		t.Fatalf("peak concurrent passes = %d", peak.Load())
	}
}

func TestReconcilerTriggersDedupAgainstLivePipelines(t *testing.T) {
	var kinds []string
	var mu sync.Mutex
	rec := &fakeRec{name: "sweep"}
	rec.run = func(ctx context.Context, enq Enqueuer) {
		out, err := enq.Enqueue(ctx, Delivery{Source: "reconcile", Project: "o/r", Revision: "abc1234def"})
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			kinds = append(kinds, "error:"+err.Error())
			return
		}
		kinds = append(kinds, out.Kind)
	}
	r := newRig(t, func(c *Config) { c.Reconcilers = []Reconciler{rec} })
	r.post("/webhook", "push", "live-1", pushBody("abc1234def")) // a live pipeline for the same SHA
	defer r.start(t)()
	waitFor(t, "the pass", func() bool { mu.Lock(); defer mu.Unlock(); return len(kinds) == 1 })
	if kinds[0] != store.IntakeCoalesced {
		t.Fatalf("reconciler trigger = %q, want coalesced into the live pipeline", kinds[0])
	}
}

func TestEnqueueFromScheduleSource(t *testing.T) {
	r := newRig(t, nil)
	out, err := r.c.Enqueue(context.Background(), Delivery{Source: "schedule", Project: "o/r", Revision: "abc1234def", TriggerClass: "schedule"})
	if err != nil || out.Kind != store.IntakeEnqueued || queued(t, r.st) != 1 {
		t.Fatalf("Enqueue = %+v, %v", out, err)
	}
	if _, err = r.c.Enqueue(context.Background(), Delivery{Source: "schedule"}); ReasonOf(err) != ReasonInvalidDelivery {
		t.Fatalf("an incomplete delivery = %v", err)
	}
}

func TestStartRefusalsHappenBeforeBind(t *testing.T) {
	var binds atomic.Int32
	listen := func(string, string) (net.Listener, error) { binds.Add(1); return net.Listen("tcp", "127.0.0.1:0") }
	guard := newErr(CodeDeployHost, "deploy_host", "this host is in the exclusion set")
	r := newRig(t, func(c *Config) {
		c.Listen, c.Guard = listen, func(context.Context) error { return guard }
	})
	err := r.c.Run(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != "E663" || binds.Load() != 0 {
		t.Fatalf("guard refusal = %v, binds=%d; want E663 and no bind", err, binds.Load())
	}
	r = newRig(t, func(c *Config) { c.Secret, c.Listen = "", listen })
	if err = r.c.Run(context.Background()); ReasonOf(err) != ReasonNoSecret || !strings.Contains(err.Error(), "refusing to start: no webhook secret configured") || binds.Load() != 0 {
		t.Fatalf("missing secret = %v, binds=%d", err, binds.Load())
	}
	r = newRig(t, func(c *Config) { c.Secret, c.Insecure = "", true })
	defer r.start(t)() // --insecure still starts
}

func TestRegisteredListenerGetsItsFlagAndStarts(t *testing.T) {
	var gotAddr atomic.Value
	var stopped atomic.Bool
	err := RegisterListener("zz-test-agent", "zz-test-agent-addr", "", func(_ context.Context, addr string, _ *Coordinator) (func(context.Context) error, error) {
		gotAddr.Store(addr)
		return func(context.Context) error { stopped.Store(true); return nil }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if RegisterListener("zz-test-agent", "other-flag", "", func(context.Context, string, *Coordinator) (func(context.Context) error, error) { return nil, nil }) == nil {
		t.Fatal("a repeated listener name was accepted")
	}
	var seen string
	RegisterFlag(func(fs *flag.FlagSet) { fs.StringVar(&seen, "zz-test-extra", "", "") })
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	addrs := AddFlags(fs)
	if err = fs.Parse([]string{"--zz-test-agent-addr", "127.0.0.1:9", "--zz-test-extra", "x"}); err != nil || seen != "x" {
		t.Fatalf("parse = %v, extra=%q", err, seen)
	}
	r := newRig(t, func(c *Config) { c.Listeners = addrs() })
	stop := r.start(t)
	waitFor(t, "the listener", func() bool { return gotAddr.Load() != nil })
	stop()
	if gotAddr.Load() != "127.0.0.1:9" || !stopped.Load() {
		t.Fatalf("listener addr=%v stopped=%v", gotAddr.Load(), stopped.Load())
	}
}

func TestDispatchRecordsResult(t *testing.T) {
	ran := make(chan string, 4)
	r := newRig(t, func(c *Config) {
		c.Dispatch = func(_ context.Context, row store.IntakeRow) (RunResult, error) {
			ran <- row.Revision
			return RunResult{PipelineID: "run-9", Exit: 1}, nil
		}
	})
	stop := r.start(t)
	var m map[string]string
	_ = json.Unmarshal(r.post("/webhook", "push", "x1", pushBody("abc1234def")).Body.Bytes(), &m)
	select {
	case rev := <-ran:
		if rev != "abc1234def" {
			t.Fatalf("dispatched %q", rev)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("nothing was dispatched")
	}
	waitFor(t, "the result", func() bool {
		row, err := r.st.IntakeGet(context.Background(), m["intake"])
		return err == nil && row.State == store.IntakeDone && row.PipelineID == "run-9" && row.Result == "exit=1"
	})
	stop()
}

// --- kill -9 fault ---------------------------------------------------------------------------------

// TestIntakeRestart proves that a coordinator killed with SIGKILL loses no job: a trigger stored before
// the kill runs after the restart, whether it was still queued or already claimed by the dead process.
func TestIntakeRestart(t *testing.T) {
	if mode := os.Getenv("NODE10_CHILD"); mode != "" {
		childAndDie(mode, os.Getenv("NODE10_DB"))
		return
	}
	for _, mode := range []string{"enqueued-before-dispatch", "claimed-mid-dispatch"} {
		t.Run(mode, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "state.db")
			cmd := exec.Command(os.Args[0], "-test.run=^TestIntakeRestart$")
			cmd.Env = append(os.Environ(), "NODE10_CHILD="+mode, "NODE10_DB="+db)
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("child survived its own SIGKILL: %s", out)
			}
			st, err := store.Open(db, store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			ran := make(chan string, 2)
			c, err := New(Config{Store: st, Insecure: true, Addr: "127.0.0.1:0", Beat: 50 * time.Millisecond, StaleAfter: 100 * time.Millisecond,
				Poll: 10 * time.Millisecond, Dispatch: func(_ context.Context, row store.IntakeRow) (RunResult, error) {
					ran <- row.Revision
					return RunResult{PipelineID: "after-restart"}, nil
				}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx) }()
			select {
			case rev := <-ran:
				if rev != "feedface00" {
					t.Fatalf("restart ran %q", rev)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("the pipeline was lost: nothing ran after the restart")
			}
			cancel()
			if err = <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func childAndDie(mode, db string) {
	st, err := store.Open(db, store.Options{})
	if err != nil {
		os.Exit(3)
	}
	c, err := New(Config{Store: st, ID: "doomed"})
	if err != nil {
		os.Exit(3)
	}
	if _, err = c.Enqueue(context.Background(), Delivery{Source: "github", DeliveryID: "kill-1", Project: "o/r", Revision: "feedface00"}); err != nil {
		os.Exit(3)
	}
	if mode == "claimed-mid-dispatch" {
		if _, ok, err := st.IntakeClaim(context.Background(), "doomed"); err != nil || !ok {
			os.Exit(3)
		}
	}
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Kill() // SIGKILL: no defer, no Close, no checkpoint
	select {}
}

func TestUnmatchedWebhookNeedsSecretUnlessInsecure(t *testing.T) {
	// ingress secrets let the process start with no shared secret; an unmatched request must still be refused
	register(t, &fakeIngress{name: "zz-fake-m1", secrets: true})
	r := newRig(t, func(c *Config) { c.Secret = "" })
	body := pushBody("abc1234def")
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	r.c.Handler(true).ServeHTTP(w, req)
	if w.Code != 403 || queued(t, r.st) != 0 {
		t.Fatalf("unsigned webhook with no shared secret = %d, queued=%d; want 403 and no row", w.Code, queued(t, r.st))
	}
	r = newRig(t, func(c *Config) { c.Secret, c.Insecure = "", true })
	req = httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w = httptest.NewRecorder()
	r.c.Handler(true).ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatalf("--insecure webhook = %d, want 202", w.Code)
	}
}

func TestSemanticKeyIncludesRequestedPipeline(t *testing.T) {
	r := newRig(t, nil)
	sha := "abc1234def"
	r.post("/webhook", "push", "k1", pushBody(sha))
	rel := r.post("/v1/trigger", "", "", []byte(`{"project":"o/r","revision":"`+sha+`","pipeline":"release"}`), "Idempotency-Key", "release-key-1")
	if outcome(t, rel) != "enqueued" {
		t.Fatalf("a manual run of another pipeline = %s, want its own run", rel.Body)
	}
	if semanticKey("o/r", sha, "change", "pd", "") == semanticKey("o/r", sha, "change", "pd", "release") {
		t.Fatal("the pipeline is not part of the key")
	}
}

func TestIntakeBodyLimits(t *testing.T) {
	r := newRig(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(nil))
	req.ContentLength = MaxBody + 1
	w := httptest.NewRecorder()
	r.c.Handler(true).ServeHTTP(w, req)
	if w.Code != 413 {
		t.Fatalf("declared 26 MiB = %d, want 413 before the body is read", w.Code)
	}
	for i := 0; i < cap(bodySlots); i++ {
		bodySlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(bodySlots); i++ {
			<-bodySlots
		}
	}()
	req = httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(make([]byte, 2<<20)))
	w = httptest.NewRecorder()
	r.c.Handler(true).ServeHTTP(w, req)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("large body with every slot taken = %d, want 429", w.Code)
	}
	long := strings.Repeat("d", maxDeliveryID+1)
	if w = r.post("/webhook", "push", long, pushBody("abc1234def")); w.Code != 400 {
		t.Fatalf("over-long delivery id = %d, want 400", w.Code)
	}
}

func TestDispatchResultSurvivesShutdown(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	r := newRig(t, func(c *Config) {
		c.Dispatch = func(context.Context, store.IntakeRow) (RunResult, error) {
			close(started)
			<-release
			return RunResult{PipelineID: "finished", Exit: 0}, nil // returns a result even though ctx ended meanwhile
		}
	})
	var m map[string]string
	_ = json.Unmarshal(r.post("/webhook", "push", "sd1", pushBody("abc1234def")).Body.Bytes(), &m)
	ctx, cancel := context.WithCancel(context.Background())
	r.c.cfg.Addr, r.c.cfg.Beat, r.c.cfg.Poll = "127.0.0.1:0", time.Hour, 5*time.Millisecond
	done := make(chan error, 1)
	go func() { done <- r.c.Run(ctx) }()
	<-started
	cancel()
	time.Sleep(20 * time.Millisecond)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	row, err := r.st.IntakeGet(context.Background(), m["intake"])
	if err != nil || row.State != store.IntakeDone || row.PipelineID != "finished" {
		t.Fatalf("row after shutdown = %+v, %v; want done (a finished pipeline is not run twice)", row, err)
	}
}
