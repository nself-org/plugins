package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
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
	cacheOn.probe = func(context.Context, []string) (string, error) { return "go version go1.27.2 test/test\n", nil }
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
	if o.State != StatePassed || r1.uses(t, o) != "go:miss" || r1.entryCount(t) != 1 {
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
	if o = outcome(r3.run(t), "build"); o.State != StatePassed || r3.uses(t, o) != "go:miss" || r3.entryCount(t) != 2 {
		t.Fatalf("lockfile edit: %+v uses %q entries %d\n%s", o, r3.uses(t, o), r3.entryCount(t), r3.Out)
	}

	job := sh(expectEmpty + " && " + populate)
	job.Env = map[string]string{"GOFLAGS": "-mod=mod"} // a changed GOFLAGS is another key
	r4 := r1.again(t, map[string]model.Job{"build": job})
	if o = outcome(r4.run(t), "build"); o.State != StatePassed || r4.uses(t, o) != "go:miss" || r4.entryCount(t) != 3 {
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
		{"container block", AttemptInfo{Job: model.Job{Container: &model.ContainerConfig{Image: "x"}}}, "container", true},
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

func TestCacheHookEvictsTheSessionOfAnEndedAttempt(t *testing.T) {
	// a run that asks for a service no hook provides fails in prepare, after Before
	r := cacheRig(t, map[string]model.Job{"build": sh("true")})
	r.C.Opts.Services = []string{"db"}
	o := outcome(r.run(t), "build")
	if o.State != StateFailed {
		t.Fatalf("%+v\n%s", o, r.Out)
	}
	if _, ok := cacheOn.sessions.Load(o.AttemptID); !ok {
		t.Fatal("the job never reached the cache hook")
	}
	cacheOn.evict(context.Background(), r.store(t), time.Now())
	if _, ok := cacheOn.sessions.Load(o.AttemptID); ok {
		t.Fatal("the session of an ended attempt was kept")
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
