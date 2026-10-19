package run

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/notify"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// sinkLog is what the test sinks saw.
type sinkLog struct {
	mu     sync.Mutex
	events []notify.Event
}

func (l *sinkLog) add(e notify.Event) { l.mu.Lock(); l.events = append(l.events, e); l.mu.Unlock() }
func (l *sinkLog) kinds() []notify.Kind {
	l.mu.Lock()
	defer l.mu.Unlock()
	var k []notify.Kind
	for _, e := range l.events {
		k = append(k, e.Kind)
	}
	return k
}

type testSink struct {
	name string
	fn   func(context.Context, notify.Event) error
}

func (s testSink) Name() string                                   { return s.name }
func (s testSink) Notify(c context.Context, e notify.Event) error { return s.fn(c, e) }

var registerOnce sync.Once
var testSinks = struct {
	sync.Mutex
	fn func(context.Context, notify.Event) error
}{}

// registerTestSink installs the "run-test" sink type once; each test points it at its own function.
func registerTestSink(t *testing.T, fn func(context.Context, notify.Event) error) {
	t.Helper()
	registerOnce.Do(func() {
		if err := notify.Register("run-test", func(spec notify.SinkSpec, _ notify.Deps) (notify.Notifier, error) {
			return testSink{name: "run-test", fn: func(c context.Context, e notify.Event) error {
				testSinks.Lock()
				f := testSinks.fn
				testSinks.Unlock()
				return f(c, e)
			}}, nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	testSinks.Lock()
	testSinks.fn = fn
	testSinks.Unlock()
}

// hookFor builds a hook that reads its config from cfg and keeps state in st.
func hookFor(st *store.Store, cfg notify.Config) *notifyHook {
	return &notifyHook{
		Enabled: func() bool { return true },
		Load: func(context.Context, string) (notify.Config, notify.State, func(), error) {
			return cfg, notify.StoreState{S: st}, nil, nil
		},
		Ref:     func(context.Context) string { return "main" },
		Capture: func(context.Context, ...string) (string, int, error) { return "", 128, nil }, // no git remote
	}
}

func openState(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "notify.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func failRig(t *testing.T) *rig {
	t.Helper()
	return newRig(t, map[string]model.Job{"a": sh("true"), "lint": sh("exit 1", "a")}, gitFake{})
}

var oneSink = notify.Config{Sinks: []notify.SinkSpec{{Type: "run-test"}}}

// The hook turns a finished run into events: a failure once per commit, then a recovery.
func TestNotifyHookMapsRunsToEvents(t *testing.T) {
	log := &sinkLog{}
	registerTestSink(t, func(_ context.Context, e notify.Event) error { log.add(e); return nil })
	h := hookFor(openState(t), oneSink)
	r := failRig(t)
	r.C.Hooks.Auto.Notify = h
	res := r.run(t)
	if res.Exit != 1 || len(log.events) != 1 {
		t.Fatalf("exit %d events %v", res.Exit, log.kinds())
	}
	ev := log.events[0]
	if ev.Kind != notify.KindFailure || ev.Revision != testSHA || ev.Context != "nself-ci" || ev.Ref != "main" || ev.Result != "fail" || ev.RunID != res.RunID ||
		ev.Repo == "" || ev.Counts == nil || ev.Counts.Fail != 1 || len(ev.FailedJobs) != 1 || ev.FailedJobs[0] == "" || ev.Show != "nself ci runs show "+res.RunID {
		t.Fatalf("event: %+v", ev)
	}
	r2 := failRig(t) // the same commit fails again: no second alert
	r2.C.Hooks.Auto.Notify = h
	r2.C.Opts.Root = r.C.Opts.Root // the repo id is a digest of the root
	r2.run(t)
	if len(log.events) != 1 {
		t.Fatalf("one alert per commit: %v", log.kinds())
	}
	r3 := passRig(t) // green on the same ref: recovery (same root, so the same repo id)
	r3.C.Hooks.Auto.Notify = h
	r3.C.Opts.Root = r.C.Opts.Root
	r3.run(t)
	if got := log.kinds(); len(got) != 2 || got[1] != notify.KindRecovery {
		t.Fatalf("recovery: %v", got)
	}
}

// With v1.4 selected, or no sink configured, the hook does nothing at all.
func TestNotifyHookInert(t *testing.T) {
	loaded := 0
	h := &notifyHook{Enabled: func() bool { return false }, Load: func(context.Context, string) (notify.Config, notify.State, func(), error) {
		loaded++
		return notify.Config{}, nil, nil, nil
	}}
	if err := h.Notify(context.Background(), NotifyRequest{}); err != nil || loaded != 0 {
		t.Fatalf("v1.4: err %v loaded %d", err, loaded)
	}
	h.Enabled = func() bool { return true }
	if err := h.Notify(context.Background(), NotifyRequest{}); err != nil || loaded != 1 {
		t.Fatalf("no sinks: err %v loaded %d", err, loaded)
	}
	h.Load = func(context.Context, string) (notify.Config, notify.State, func(), error) {
		return notify.Config{}, nil, nil, errors.New("db locked\nsecond line")
	}
	if err := h.Notify(context.Background(), NotifyRequest{}); err == nil || !strings.HasPrefix(err.Error(), "notify settings not read") || strings.Contains(err.Error(), "second line") {
		t.Fatalf("a settings failure is one note line: %v", err)
	}
}

// TestSinkNeverAffectsRun: a sink that errors, panics or hangs changes neither the run's
// result, exit code, status line, nor the posted status; the hook returns once the budget ends.
func TestSinkNeverAffectsRun(t *testing.T) {
	base := failRig(t)
	want := base.run(t)
	wantPosted := base.Rep.all()

	cases := map[string]func(context.Context, notify.Event) error{
		"errors": func(context.Context, notify.Event) error { return errors.New("sink down") },
		"panics": func(context.Context, notify.Event) error { panic("sink bug") },
	}
	hang := make(chan time.Time, 1)
	cases["hangs"] = func(ctx context.Context, _ notify.Event) error {
		hang <- time.Time{} // starts: the injected budget fires
		<-ctx.Done()
		return ctx.Err()
	}
	for name, fn := range cases {
		registerTestSink(t, fn)
		h := hookFor(openState(t), oneSink)
		h.After = func(time.Duration) <-chan time.Time {
			if name == "hangs" {
				return hang
			}
			return nil
		}
		r := failRig(t)
		r.C.Hooks.Auto.Notify = h
		got := r.run(t)
		posted := r.Rep.all()
		if got.Exit != want.Exit || got.Summary.Result != want.Summary.Result || stripDigest(got.Summary.Status) != stripDigest(want.Summary.Status) ||
			len(posted) != len(wantPosted) || posted[0].State != wantPosted[0].State || posted[0].Description != wantPosted[0].Description && stripDigest(posted[0].Description) != stripDigest(wantPosted[0].Description) {
			t.Errorf("%s: the run changed:\n got %d %q %+v\nwant %d %q %+v", name, got.Exit, got.Summary.Status, posted, want.Exit, want.Summary.Status, wantPosted)
		}
		found := false
		for _, n := range got.Summary.Notes {
			found = found || (strings.HasPrefix(n, "notify hook:") && strings.Contains(n, "E813") && !strings.Contains(n, "sink down") && !strings.Contains(n, "sink bug"))
		}
		if !found {
			t.Errorf("%s: a failed delivery must be one safe note, got %v", name, got.Summary.Notes)
		}
	}
}

// A served run is a clone in a fresh directory: the repository is named by its origin URL and the
// branch is read from the run's own root, so recovery works across clones and across refs.
func TestNotifyHookNamesRepoAndRefFromTheRunRoot(t *testing.T) {
	var asked []string
	h := &notifyHook{Capture: func(_ context.Context, argv ...string) (string, int, error) {
		asked = append(asked, strings.Join(argv, " "))
		switch {
		case strings.Contains(strings.Join(argv, " "), "remote.origin.url"):
			return "https://example.org/acme/app.git\n", 0, nil
		case strings.Contains(strings.Join(argv, " "), "symbolic-ref"):
			return "feature/x\n", 0, nil
		}
		return "", 1, nil
	}}
	a := h.runOf(context.Background(), NotifyRequest{Root: "/clone/one", Run: RunInfo{Revision: testSHA, Result: "fail"}})
	b := h.runOf(context.Background(), NotifyRequest{Root: "/clone/two", Run: RunInfo{Revision: testSHA, Result: "pass"}})
	if a.Repo == "" || a.Repo != b.Repo || a.Ref != "feature/x" || strings.Contains(a.Repo, "example.org") {
		t.Fatalf("repo %q/%q ref %q", a.Repo, b.Repo, a.Ref)
	}
	if len(asked) != 4 || !strings.Contains(asked[0], "-C /clone/one") || !strings.Contains(asked[2], "-C /clone/two") {
		t.Fatalf("git must run in the run root: %v", asked)
	}
	// no origin: the evidence's repo id; detached head: no ref
	h.Capture = func(context.Context, ...string) (string, int, error) { return "", 1, nil }
	c := h.runOf(context.Background(), NotifyRequest{Root: "/clone/one", Run: RunInfo{Revision: testSHA, Evidence: model.Evidence{RepoID: "abc123"}}})
	if c.Repo != "abc123" || c.Ref != "" {
		t.Fatalf("fallbacks: %+v", c)
	}
	// the completed evidence's root-commit id wins over the origin URL
	h.Capture = func(context.Context, ...string) (string, int, error) { return "https://example.org/a.git\n", 0, nil }
	d := h.runOf(context.Background(), NotifyRequest{Root: "/clone/one", Run: RunInfo{Revision: testSHA, Evidence: model.Evidence{RepoID: "local:" + strings.Repeat("f", 64)}}})
	if d.Repo != "local:"+strings.Repeat("f", 64) {
		t.Fatalf("repo %q", d.Repo)
	}
}

// A dirty tree or a run with no commit has no commit of record: nothing is sent and no key is claimed.
func TestNotifyHookSkipsRunsWithoutACommitOfRecord(t *testing.T) {
	log := &sinkLog{}
	registerTestSink(t, func(_ context.Context, e notify.Event) error { log.add(e); return nil })
	h := hookFor(openState(t), oneSink)
	for _, info := range []RunInfo{{Revision: testSHA, Result: "fail", Dirty: true}, {Result: "fail"}} {
		if err := h.Notify(context.Background(), NotifyRequest{Run: info}); err != nil {
			t.Fatal(err)
		}
	}
	if len(log.events) != 0 {
		t.Fatalf("sent for a run without a commit of record: %v", log.kinds())
	}
}

// The production path: notify.sinks stored as an operator row is found through the real resolver.
func TestNotifyHookLoadPolicyReadsOperatorRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{notify.KeySinks: `[{"type":"stdout","name":"console"}]`, notify.KeyDigest: `"daily"`} {
		if err = st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: k, Value: []byte(v), Actor: "test"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	h := &notifyHook{} // real git: the test runs inside the repository checkout
	cfg, state, release, err := h.loadPolicy(context.Background(), repoRoot(t))
	if release != nil {
		defer release()
	}
	if err != nil || state == nil || len(cfg.Sinks) != 1 || cfg.Sinks[0].Name != "console" || !cfg.Digest {
		t.Fatalf("config %+v err %v", cfg, err)
	}
}

// repoRoot is the git checkout this test file lives in.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
