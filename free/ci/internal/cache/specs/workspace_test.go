package specs

import (
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// D-1032: a container job restores read-only but could never populate a cache,
// because the hardened profile refuses a writable host mount. A container miss
// now lives in the job workspace: BeforeJob creates the dirs there, the job
// fills them, AfterJob packs them and Finalize commits.
func TestContainerMissPopulatesTheCacheThroughTheWorkspace(t *testing.T) {
	e := newEnv(t)
	e.running("c1", ownerFacts)
	s := e.begin(e.containerInput("c1", ownerFacts))
	if got := resultsOf(e.uses("c1")); strings.Join(got, ",") != "go:miss" {
		t.Fatalf("uses %v", got)
	}
	env, mounts := s.Setup()
	if len(mounts) != 0 {
		t.Fatalf("a container miss needs no host mount (the profile refuses a writable one): %+v", mounts)
	}
	for _, name := range []string{"GOMODCACHE", "GOCACHE"} {
		if !strings.HasPrefix(env[name], "/repo/.nself-ci-cache/go/") {
			t.Fatalf("%s=%q must point into the workspace as the container sees it", name, env[name])
		}
	}
	ws, dir := e.workspace("c1")
	if err := s.BeforeJob(e.ctx, ws); err != nil {
		t.Fatal(err)
	}
	// the job runs in the container and fills what it sees at /repo/.nself-ci-cache
	local := func(v string) string { return filepath.Join(dir, strings.TrimPrefix(v, "/repo/")) }
	putFile(t, filepath.Join(local(env["GOMODCACHE"]), "example.com", "m@v1", "m.go"), "package m")
	putFile(t, filepath.Join(local(env["GOCACHE"]), "ab", "cd-d"), "compiled")
	if err := s.AfterJob(e.ctx, ws); err != nil {
		t.Fatal(err)
	}
	if !s.Wants() {
		t.Fatal("a container miss with files must be saved")
	}
	s.Finalize(e.ctx, e.finalizing("c1"))
	e.settlePassed("c1")
	if e.entryCount() != 1 {
		t.Fatalf("entries %d: the container job's cache was not saved", e.entryCount())
	}
	s.Cleanup()

	e.running("c2", ownerFacts)
	s2 := e.begin(e.containerInput("c2", ownerFacts))
	if got := resultsOf(e.uses("c2")); strings.Join(got, ",") != "go:readonly" {
		t.Fatalf("the next container run must restore it: %v", got)
	}
	if getFile(t, filepath.Join(s2.dir("mod"), "example.com", "m@v1", "m.go")) != "package m" {
		t.Fatal("restored content differs")
	}
	if s2.Wants() {
		t.Fatal("a restored entry is never saved again")
	}
}

func TestContainerWorkspaceCacheSavesNothingWhenTheJobWroteNothingOrDidNotPass(t *testing.T) {
	e := newEnv(t)
	e.running("c1", ownerFacts)
	s := e.begin(e.containerInput("c1", ownerFacts))
	ws, _ := e.workspace("c1")
	if err := s.BeforeJob(e.ctx, ws); err != nil {
		t.Fatal(err)
	}
	if err := s.AfterJob(e.ctx, ws); err != nil {
		t.Fatal(err)
	}
	if s.Wants() {
		t.Fatal("an empty workspace cache has nothing to save")
	}
	s.Finalize(e.ctx, e.finalizing("c1"))
	if e.entryCount() != 0 {
		t.Fatal("an empty cache must not be committed")
	}

	e.running("c3", ownerFacts)
	s = e.begin(e.containerInput("c3", ownerFacts))
	ws, dir := e.workspace("c3")
	_ = s.BeforeJob(e.ctx, ws)
	env, _ := s.Setup()
	putFile(t, filepath.Join(dir, strings.TrimPrefix(path.Join(env["GOMODCACHE"], "x"), "/repo/")), "x")
	// the job failed: AfterJob never runs, and a session that is not finalizing saves nothing
	s.Finalize(e.ctx, 2)
	if e.entryCount() != 0 {
		t.Fatal("a failed attempt saved a cache")
	}
}

func TestContainerWorkspaceCacheRefusesARegisteredValue(t *testing.T) {
	e := newEnv(t)
	if err := e.scan.Register([]byte(fakeSecret)); err != nil {
		t.Fatal(err)
	}
	e.running("c1", ownerFacts)
	s := e.begin(e.containerInput("c1", ownerFacts))
	ws, dir := e.workspace("c1")
	_ = s.BeforeJob(e.ctx, ws)
	env, _ := s.Setup()
	putFile(t, filepath.Join(dir, strings.TrimPrefix(env["GOMODCACHE"], "/repo/"), "leak.txt"), "token="+fakeSecret)
	if err := s.AfterJob(e.ctx, ws); err != nil {
		t.Fatalf("a cache refusal must never fail the job: %v", err)
	}
	s.Finalize(e.ctx, e.finalizing("c1"))
	if e.entryCount() != 0 || e.auditActions("cache.save.refused") != 1 {
		t.Fatalf("entries %d, refusals %d", e.entryCount(), e.auditActions("cache.save.refused"))
	}
	if got := resultsOf(e.uses("c1")); strings.Join(got, ",") != "go:miss,go:refused" {
		t.Fatalf("uses %v", got)
	}
}
