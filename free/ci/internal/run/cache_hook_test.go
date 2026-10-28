package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

// These tests change process-wide hook state, so none of them runs in parallel.

const fakeCacheSecret = "fake-registered-cache-value-0123456789"

const populate = `mkdir -p "$GOMODCACHE/x" && echo mod > "$GOMODCACHE/x/m.txt" && echo build > "$GOCACHE/b.txt"`
const expectRestored = `test "$(cat "$GOMODCACHE/x/m.txt")" = mod && test "$(cat "$GOCACHE/b.txt")" = build`
const expectEmpty = `test -n "$GOMODCACHE" && test ! -e "$GOMODCACHE/x/m.txt" && test ! -e "$GOCACHE/b.txt"`

// cacheRig is a rig whose root is a Go project (go.mod + go.sum) and whose
// tool probe is fixed, so keys do not depend on the machine's Go.
func cacheRig(t *testing.T, jobs map[string]model.Job) *rig {
	t.Helper()
	t.Setenv("NSELF_CI_STORE_MIN_FREE", "1")
	t.Setenv("NSELF_CI_CACHE_DISABLE", "")
	old := cacheOn.probe
	cacheOn.probe = func(context.Context, model.Job, string, []string) (string, error) {
		return "go version go1.27.2 test/test\n", nil
	}
	t.Cleanup(func() { cacheOn.probe = old })
	r := newRig(t, jobs, gitFake{})
	lock(t, r.C.Opts.Root, "h1:one")
	if err := os.WriteFile(filepath.Join(r.C.Opts.Root, "go.mod"), []byte("module example.invalid/m\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return r
}

func lock(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte(content+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// again is a rig that shares r's project root and state directory.
func (r *rig) again(t *testing.T, jobs map[string]model.Job) *rig {
	t.Helper()
	n := newRig(t, jobs, gitFake{})
	n.C.Opts.Root, n.C.Opts.Home, n.Home = r.C.Opts.Root, r.Home, r.Home
	return n
}

func (r *rig) uses(t *testing.T, o JobOutcome) string {
	t.Helper()
	list, err := r.store(t).CacheUses(context.Background(), o.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, u := range list {
		out = append(out, u.Spec+":"+u.Result)
	}
	return strings.Join(out, ",")
}

func (r *rig) entryCount(t *testing.T) int {
	t.Helper()
	list, err := r.store(t).CacheEntries(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

func (r *rig) noCacheWork(t *testing.T) {
	t.Helper()
	if ents, _ := os.ReadDir(filepath.Join(r.Home, "cache-work")); len(ents) != 0 {
		t.Fatalf("cache dirs left behind: %v", ents)
	}
}

func TestCacheHookRestoreSave(t *testing.T) {
	r1 := cacheRig(t, map[string]model.Job{"build": sh(populate)})
	o := outcome(r1.run(t), "build")
	if o.State != StatePassed || r1.uses(t, o) != "go:miss,go:saved" || r1.entryCount(t) != 1 {
		t.Fatalf("first run: %+v uses %q entries %d\n%s", o, r1.uses(t, o), r1.entryCount(t), r1.Out)
	}
	r1.noCacheWork(t)

	r2 := r1.again(t, map[string]model.Job{"build": sh(expectRestored)})
	if o = outcome(r2.run(t), "build"); o.State != StatePassed || r2.uses(t, o) != "go:hit" || r2.entryCount(t) != 1 {
		t.Fatalf("second run restores: %+v uses %q entries %d\n%s", o, r2.uses(t, o), r2.entryCount(t), r2.Out)
	}
	r2.noCacheWork(t)

	lock(t, r1.C.Opts.Root, "h1:two") // a lockfile edit misses, and the new key is saved
	r3 := r1.again(t, map[string]model.Job{"build": sh(expectEmpty + " && " + populate)})
	if o = outcome(r3.run(t), "build"); o.State != StatePassed || r3.uses(t, o) != "go:miss,go:saved" || r3.entryCount(t) != 2 {
		t.Fatalf("lockfile edit: %+v uses %q entries %d\n%s", o, r3.uses(t, o), r3.entryCount(t), r3.Out)
	}

	job := sh(expectEmpty + " && " + populate)
	job.Env = map[string]string{"GOFLAGS": "-mod=mod"} // a changed GOFLAGS is another key
	r4 := r1.again(t, map[string]model.Job{"build": job})
	if o = outcome(r4.run(t), "build"); o.State != StatePassed || r4.uses(t, o) != "go:miss,go:saved" || r4.entryCount(t) != 3 {
		t.Fatalf("GOFLAGS change: %+v uses %q entries %d\n%s", o, r4.uses(t, o), r4.entryCount(t), r4.Out)
	}
}

func TestCacheHookFailedAttemptSavesNothing(t *testing.T) {
	r := cacheRig(t, map[string]model.Job{"build": sh(populate + "; exit 3")})
	o := outcome(r.run(t), "build")
	if o.State != StateFailed || r.entryCount(t) != 0 || r.uses(t, o) != "go:miss" {
		t.Fatalf("%+v entries %d uses %q", o, r.entryCount(t), r.uses(t, o))
	}
	r.noCacheWork(t)
}

func TestCacheHookOperatorAndJobDisable(t *testing.T) {
	probe := `case "$GOMODCACHE" in *cache-work*) exit 9;; esac`
	off := sh(probe + " && " + populate + "; true")
	off.Cache = &model.JobCacheConfig{Disable: true}
	t.Run("job disable", func(t *testing.T) {
		r := cacheRig(t, map[string]model.Job{"build": off})
		if o := outcome(r.run(t), "build"); o.State != StatePassed || r.entryCount(t) != 0 || r.uses(t, o) != "" {
			t.Fatalf("%+v\n%s", o, r.Out)
		}
	})
	t.Run("operator scope cannot be undone by the revision's keys", func(t *testing.T) {
		j := sh(probe)
		j.Cache = &model.JobCacheConfig{Keys: []string{"force"}}
		r := cacheRig(t, map[string]model.Job{"build": j})
		t.Setenv("NSELF_CI_CACHE_DISABLE", "1")
		if o := outcome(r.run(t), "build"); o.State != StatePassed || r.uses(t, o) != "" || r.entryCount(t) != 0 {
			t.Fatalf("%+v\n%s", o, r.Out)
		}
	})
}

func TestCacheHookTopLevelDisable(t *testing.T) {
	r := cacheRig(t, map[string]model.Job{"build": sh(`case "$GOMODCACHE" in *cache-work*) exit 9;; esac`)})
	if err := os.MkdirAll(filepath.Join(r.C.Opts.Root, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\ncache:\n  disable: true\njobs:\n  build:\n    kind: test\n    run: [\"true\"]\n"
	if err := os.WriteFile(filepath.Join(r.C.Opts.Root, ".nself", "ci.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	// The harness builds a plan before writing this config; mirror the resolved block.
	r.C.Plan.Cache = &model.CacheConfig{Disable: true}
	if o := outcome(r.run(t), "build"); o.State != StatePassed || r.uses(t, o) != "" || r.entryCount(t) != 0 {
		t.Fatalf("%+v\n%s", o, r.Out)
	}
}

func TestCacheHookRefusesPathsOutsideTheWorkspace(t *testing.T) {
	j := sh("true")
	j.Cache = &model.JobCacheConfig{Paths: []string{"../outside"}}
	r := cacheRig(t, map[string]model.Job{"build": j})
	if o := outcome(r.run(t), "build"); o.State != StateFailed || o.Code != "E711" {
		t.Fatalf("%+v\n%s", o, r.Out)
	}
}

func TestCacheHookNarrowedPathsAreAnotherKey(t *testing.T) {
	r1 := cacheRig(t, map[string]model.Job{"build": sh(populate)})
	r1.run(t)
	j := sh(expectEmpty)
	j.Cache = &model.JobCacheConfig{Paths: []string{"."}, Keys: []string{"v2"}}
	r2 := r1.again(t, map[string]model.Job{"build": j})
	if o := outcome(r2.run(t), "build"); o.State != StatePassed || r2.uses(t, o) != "go:miss" {
		t.Fatalf("%+v uses %q\n%s", o, r2.uses(t, o), r2.Out)
	}
}

func TestCacheHookRefusesARegisteredValueInTheCache(t *testing.T) {
	if err := RegisterSecretValue([]byte(fakeCacheSecret)); err != nil {
		t.Fatal(err)
	}
	r := cacheRig(t, map[string]model.Job{"build": sh(populate + `; printf 'k=%s\n' ` + fakeCacheSecret + ` > "$GOCACHE/leak.txt"`)})
	o := outcome(r.run(t), "build")
	if o.State != StatePassed || r.entryCount(t) != 0 || r.uses(t, o) != "go:miss,go:refused" {
		t.Fatalf("%+v entries %d uses %q\n%s", o, r.entryCount(t), r.uses(t, o), r.Out)
	}
	r.noCacheWork(t)
}

func TestCacheFactsRecordDeclaredSecretsAndPrivateDeps(t *testing.T) {
	p := store.PipelineRow{Project: "/work/proj", SourceTrust: "owner"}
	att := CacheAttestation{Protection: "protected"}
	with := model.Job{Secrets: []string{"DEPLOY_TOKEN"}} // declared counts, resolved or not
	if f := cacheFacts(AttemptInfo{Job: with}, p, att, true); !f.SecretsDeclared || !f.PrivateDeps || f.Project != "/work/proj" || f.Trust != "owner" || f.Protection != "protected" {
		t.Fatalf("%+v", f)
	}
	if f := cacheFacts(AttemptInfo{Job: model.Job{}}, p, att, false); f.SecretsDeclared || f.PrivateDeps {
		t.Fatalf("%+v", f)
	}
}

func TestCacheHookEvictsSessionsThatNeverReachedAfter(t *testing.T) {
	h := &cacheHook{}
	h.sessions.Store("old", cacheEntry{s: &specs.Session{}, at: time.Now().Add(-2 * staleAfter), id: "old"})
	h.sessions.Store("new", cacheEntry{s: &specs.Session{}, at: time.Now(), id: "new"})
	h.evict(context.Background(), nil, time.Now())
	if _, ok := h.sessions.Load("old"); ok {
		t.Fatal("a stale session was kept")
	}
	if _, ok := h.sessions.Load("new"); !ok {
		t.Fatal("a live session was evicted")
	}
}

func TestCacheHookJobEnvCannotLiftTheOperatorDisable(t *testing.T) {
	j := sh(`case "$GOMODCACHE" in *cache-work*) exit 9;; esac`)
	j.Env = map[string]string{"NSELF_CI_CACHE_DISABLE": "0"}
	r := cacheRig(t, map[string]model.Job{"build": j})
	t.Setenv("NSELF_CI_CACHE_DISABLE", "1")
	if o := outcome(r.run(t), "build"); o.State != StatePassed || r.uses(t, o) != "" {
		t.Fatalf("%+v\n%s", o, r.Out)
	}
	// and the job env can add a disable of its own
	j2 := sh(`case "$GOMODCACHE" in *cache-work*) exit 9;; esac`)
	j2.Env = map[string]string{"NSELF_CI_CACHE_DISABLE": "1"}
	t.Setenv("NSELF_CI_CACHE_DISABLE", "")
	r2 := cacheRig(t, map[string]model.Job{"build": j2})
	if o := outcome(r2.run(t), "build"); o.State != StatePassed || r2.uses(t, o) != "" {
		t.Fatalf("%+v\n%s", o, r2.Out)
	}
}

func TestCacheLevelFollowsTheEffectiveIsolation(t *testing.T) {
	cases := []struct {
		name  string
		a     AttemptInfo
		level string
		ok    bool
	}{
		{"default", AttemptInfo{}, "process", true},
		{"process", AttemptInfo{Job: model.Job{Isolation: "process"}}, "process", true},
		{"container job", AttemptInfo{Job: model.Job{Isolation: "container"}}, "container", true},
		{"container block alone is still a process job", AttemptInfo{Job: model.Job{Container: &model.ContainerConfig{Image: "x"}}}, "process", true},
		{"process isolation with a container block", AttemptInfo{Job: model.Job{Isolation: "process", Container: &model.ContainerConfig{Image: "x"}}}, "process", true},
		{"--container beats a vm job", AttemptInfo{Container: true, Job: model.Job{Isolation: "vm"}}, "container", true},
		{"--container run", AttemptInfo{Container: true}, "container", true},
		{"vm", AttemptInfo{Job: model.Job{Isolation: "vm"}}, "", false},
		{"hosted", AttemptInfo{Job: model.Job{Isolation: "hosted-disposable"}}, "", false},
	}
	for _, c := range cases {
		if level, ok := cacheLevel(c.a); level != c.level || ok != c.ok {
			t.Errorf("%s: %q %v, want %q %v", c.name, level, ok, c.level, c.ok)
		}
	}
}

func TestCacheHookPrepareFailureReleasesSession(t *testing.T) {
	// a run that asks for a service no hook provides fails in prepare, after Before
	r := cacheRig(t, map[string]model.Job{"build": sh("true")})
	r.C.Opts.Services = []string{"db"}
	o := outcome(r.run(t), "build")
	if o.State != StateFailed {
		t.Fatalf("%+v\n%s", o, r.Out)
	}
	if _, ok := cacheOn.sessions.Load(o.AttemptID); ok {
		t.Fatal("prepare failure kept the cache session until eviction")
	}
	r.noCacheWork(t)
}

func TestCacheHookCachesAPnpmProject(t *testing.T) {
	populateStore := `mkdir -p "$npm_config_store_dir/v3" && echo pkg > "$npm_config_store_dir/v3/p.json"`
	r1 := cacheRig(t, map[string]model.Job{"install": sh(populateStore)})
	for name, body := range map[string]string{"package.json": `{"name":"x","packageManager":"pnpm@9.0.0"}`, "pnpm-lock.yaml": "lockfileVersion: '9.0'\n"} {
		if err := os.WriteFile(filepath.Join(r1.C.Opts.Root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if o := outcome(r1.run(t), "install"); o.State != StatePassed || !strings.Contains(r1.uses(t, o), "pnpm:miss") || r1.entryCount(t) != 1 { // the Go dirs stay empty: only the pnpm store is saved
		t.Fatalf("%+v uses %q entries %d\n%s", o, r1.uses(t, o), r1.entryCount(t), r1.Out)
	}
	r2 := r1.again(t, map[string]model.Job{"install": sh(`test "$(cat "$npm_config_store_dir/v3/p.json")" = pkg`)})
	if o := outcome(r2.run(t), "install"); o.State != StatePassed || !strings.Contains(r2.uses(t, o), "pnpm:hit") {
		t.Fatalf("%+v uses %q\n%s", o, r2.uses(t, o), r2.Out)
	}
}

func TestFactDirsIncludeEveryDetectedProject(t *testing.T) {
	facts := []detect.Fact{{Path: "go.mod"}, {Path: "packages/web/package.json"}, {Path: "svc/api/go.mod"}}
	got := strings.Join(factDirs("/repo", "/repo/sub", facts), ",")
	if got != "/repo,/repo/sub,/repo/sub/packages/web,/repo/sub/svc/api" {
		t.Fatalf("dirs %s", got)
	}
}

func TestCacheHookNeverEvictsALiveAttempt(t *testing.T) {
	r := cacheRig(t, map[string]model.Job{"build": sh("true")})
	st := r.store(t)
	ctx := context.Background()
	p := store.PipelineRow{ID: "pl1", Revision: "rev", Project: r.C.Opts.Root, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for i, e := range []error{st.CreatePipeline(ctx, p), st.CreateJob(ctx, store.JobRow{ID: "jl1", PipelineID: "pl1", Name: "b", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		st.CreateAttempt(ctx, store.AttemptRow{ID: "live1", JobID: "jl1", N: 1}, ""),
		storetest.Admit(ctx, st, "live1"), st.Transition(ctx, "live1", "leased", "running", store.TransitionDetail{Epoch: 1})} {
		if e != nil {
			t.Fatalf("step %d: %v", i, e)
		}
	}
	h := &cacheHook{}
	h.sessions.Store("live1", cacheEntry{s: &specs.Session{}, at: time.Now().Add(-3 * staleAfter), id: "live1"})
	h.evict(ctx, st, time.Now())
	if _, ok := h.sessions.Load("live1"); !ok {
		t.Fatal("a running attempt's session was evicted for its age")
	}
	if err := st.Transition(ctx, "live1", "running", "failed", store.TransitionDetail{Epoch: 2, FailureClass: "infra"}); err != nil {
		t.Fatal(err)
	}
	h.evict(ctx, st, time.Now())
	if _, ok := h.sessions.Load("live1"); ok {
		t.Fatal("the session of an ended attempt was kept")
	}
}

// recordingCache is a CacheHook that notes what the run tells it, and fails the job.
type recordingCache struct{ container chan bool }

func (c recordingCache) Before(_ context.Context, a AttemptInfo) (CacheSetup, error) {
	c.container <- a.Container
	return CacheSetup{}, errors.New("E711 stop here")
}
func (recordingCache) After(context.Context, AttemptInfo, Outcome) error { return nil }

func TestRunTellsTheCacheHookAboutTheContainerOption(t *testing.T) {
	for _, container := range []bool{false, true} {
		r := newRig(t, map[string]model.Job{"build": sh("true")}, gitFake{})
		rec := recordingCache{container: make(chan bool, 4)}
		r.C.Hooks.Cache, r.C.Opts.Container = rec, container
		r.C.Hooks.Image = nil
		_ = outcome(r.run(t), "build")
		select {
		case got := <-rec.container:
			if got != container {
				t.Fatalf("--container=%v reached the hook as %v", container, got)
			}
		default:
			t.Fatalf("--container=%v: the hook was not called", container)
		}
	}
}

// fakeSession records how a session is closed.
type fakeSession struct{ saved, retracted, cleaned bool }

func (f *fakeSession) Finalize(context.Context, int64)           {}
func (f *fakeSession) Wants() bool                               { return false }
func (f *fakeSession) Saved() bool                               { return f.saved }
func (f *fakeSession) Retract(context.Context)                   { f.retracted = true }
func (f *fakeSession) Cleanup()                                  { f.cleaned = true }
func (f *fakeSession) BeforeJob(context.Context, *os.Root) error { return nil }
func (f *fakeSession) AfterJob(context.Context, *os.Root) error  { return nil }

// TestCacheHookEvictionRetractsACancelledAttemptsEntry: an attempt that committed
// an entry and settled cancelled, whose session eviction closes before After
// ran, still has its entry retracted; a passed one keeps it.
func TestCacheHookEvictionRetractsACancelledAttemptsEntry(t *testing.T) {
	r := cacheRig(t, map[string]model.Job{"build": sh("true")})
	st := r.store(t)
	ctx := context.Background()
	p := store.PipelineRow{ID: "plc", Revision: "rev", Project: r.C.Opts.Root, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	steps := []error{st.CreatePipeline(ctx, p)}
	for _, id := range []string{"canc1", "pass1"} {
		steps = append(steps, st.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "plc", Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
			st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""),
			storetest.Admit(ctx, st, id), st.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}),
			st.Transition(ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}))
	}
	steps = append(steps, st.Transition(ctx, "canc1", "finalizing", "cancelled", store.TransitionDetail{Epoch: 3, FailureClass: "cancelled"}),
		st.Transition(ctx, "pass1", "finalizing", "passed", store.TransitionDetail{Epoch: 3}))
	for i, e := range steps {
		if e != nil {
			t.Fatalf("step %d: %v", i, e)
		}
	}
	h := &cacheHook{}
	canc, pass := &fakeSession{saved: true}, &fakeSession{saved: true}
	h.sessions.Store("canc1", cacheEntry{s: canc, at: time.Now(), id: "canc1"})
	h.sessions.Store("pass1", cacheEntry{s: pass, at: time.Now(), id: "pass1"})
	h.evict(ctx, st, time.Now())
	if !canc.retracted || !canc.cleaned {
		t.Fatalf("a cancelled attempt's entry must be retracted by eviction too: %+v", canc)
	}
	if pass.retracted || !pass.cleaned {
		t.Fatalf("a passed attempt keeps its entry: %+v", pass)
	}
}

func TestSweepStaleNeverFollowsASymlink(t *testing.T) {
	victim := t.TempDir()
	if err := os.MkdirAll(filepath.Join(victim, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(victim, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.Symlink(victim, filepath.Join(work, "old-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "old-dir", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	sweepStale(work, time.Now().Add(3*staleAfter), func(string) bool { return false })
	if info, err := os.Stat(filepath.Join(victim, "sub")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("the link target was touched: %v %v", info, err)
	}
	if ents, _ := os.ReadDir(work); len(ents) != 0 {
		t.Fatalf("stale entries left: %v", ents)
	}
	alive := filepath.Join(work, "live")
	if err := os.MkdirAll(alive, 0o755); err != nil {
		t.Fatal(err)
	}
	sweepStale(work, time.Now().Add(3*staleAfter), func(id string) bool { return id == "live" })
	if _, err := os.Stat(alive); err != nil {
		t.Fatal("a live attempt's dir was swept")
	}
}

func TestCloseSessionRetractsOnlyAConfirmedNonPass(t *testing.T) {
	r := cacheRig(t, map[string]model.Job{"build": sh("true")})
	f := &fakeSession{saved: true}
	closeSession(context.Background(), r.store(t), cacheEntry{s: f, id: "no-such-attempt"}) // history pruned: unknown, so not a confirmed non-pass
	if f.retracted || !f.cleaned {
		t.Fatalf("an attempt the store no longer knows must keep its entry: %+v", f)
	}
}

// Two jobs of one run that use the same toolchain cache must not share one key:
// the entry a hit restores is never saved again, so with a shared key the job
// whose save lost the race would restore the other job's partial cache on every
// later run (D-1008: go vet restoring the cache of go test and rebuilding).
func TestCacheHookKeysAreScopedToTheJob(t *testing.T) {
	vet := `echo vet > "$GOCACHE/vet.txt"`
	test := `echo test > "$GOCACHE/test.txt"`
	r1 := cacheRig(t, map[string]model.Job{"vet": sh(vet), "test": sh(test)})
	res := r1.run(t)
	if o := outcome(res, "vet"); o.State != StatePassed {
		t.Fatalf("vet: %+v\n%s", o, r1.Out)
	}
	if o := outcome(res, "test"); o.State != StatePassed {
		t.Fatalf("test: %+v\n%s", o, r1.Out)
	}
	if n := r1.entryCount(t); n != 2 {
		t.Fatalf("entries after the first run = %d, want one per job (2)", n)
	}
	r2 := r1.again(t, map[string]model.Job{
		"vet":  sh(`test "$(cat "$GOCACHE/vet.txt")" = vet && test ! -e "$GOCACHE/test.txt"`),
		"test": sh(`test "$(cat "$GOCACHE/test.txt")" = test && test ! -e "$GOCACHE/vet.txt"`)})
	res = r2.run(t)
	for _, name := range []string{"vet", "test"} {
		if o := outcome(res, name); o.State != StatePassed || r2.uses(t, o) != "go:hit" {
			t.Fatalf("%s on the second run: %+v uses %q\n%s", name, o, r2.uses(t, o), r2.Out)
		}
	}
}

// The tool probe runs in the job's workdir with the job's env (D-1035).
func TestCacheHookProbesInTheJobsWorkdirAndEnv(t *testing.T) {
	r := cacheRig(t, map[string]model.Job{"build": sh(populate)})
	var gotJob model.Job
	var gotRoot string
	old := cacheOn.probe
	cacheOn.probe = func(_ context.Context, job model.Job, root string, _ []string) (string, error) {
		gotJob, gotRoot = job, root
		return "go version go1.27.2 test/test\n", nil
	}
	t.Cleanup(func() { cacheOn.probe = old })
	sub := filepath.Join(r.C.Opts.Root, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "go.mod"), []byte("module example.invalid/s\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	j := sh(populate)
	j.Workdir, j.Env = "sub", map[string]string{"GOTOOLCHAIN": "go1.99.0"}
	r2 := r.again(t, map[string]model.Job{"build": j})
	if o := outcome(r2.run(t), "build"); o.State != StatePassed {
		t.Fatalf("%+v\n%s", o, r2.Out)
	}
	if gotJob.Workdir != "sub" || gotJob.Env["GOTOOLCHAIN"] != "go1.99.0" || gotRoot != r.C.Opts.Root {
		t.Fatalf("probe saw job %+v root %q", gotJob, gotRoot)
	}
}
