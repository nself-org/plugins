package specs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// fill writes the files a Go job leaves in its two cache dirs.
func fill(t *testing.T, s *Session) {
	t.Helper()
	dirs := s
	putFile(t, filepath.Join(dirs.dir("mod"), "example.com", "m@v1", "m.go"), "package m")
	putFile(t, filepath.Join(dirs.dir("build"), "ab", "cd-d"), "compiled")
}

// saved runs a whole passing owner attempt and returns the finished session.
func saved(t *testing.T, e *env, id string, f entries.Facts) *Session {
	t.Helper()
	e.running(id, f)
	s := e.begin(e.input(id, f))
	fill(t, s)
	s.Finalize(e.ctx, e.finalizing(id))
	return s
}

func TestSessionSavesAfterAPassAndRestoresTheNextRun(t *testing.T) {
	e := newEnv(t)
	s1 := saved(t, e, "a1", ownerFacts)
	if got := resultsOf(e.uses("a1")); strings.Join(got, ",") != "go:miss" || e.entryCount() != 1 {
		t.Fatalf("first run: uses %v, entries %d", got, e.entryCount())
	}
	env1, mounts := s1.Setup()
	if env1["GOMODCACHE"] != filepath.Join(e.home, "cache-work", "a1", "go", "mod") || env1["GOCACHE"] != filepath.Join(e.home, "cache-work", "a1", "go", "build") || len(mounts) != 2 {
		t.Fatalf("setup env %v mounts %v", env1, mounts)
	}
	for _, m := range mounts {
		if m.ReadOnly || m.Host != m.Target {
			t.Fatalf("a process job's own cache mounts are writable at the host path: %+v", m)
		}
	}
	s1.Cleanup()
	if _, err := os.Stat(filepath.Join(e.home, "cache-work", "a1")); !os.IsNotExist(err) {
		t.Fatal("the attempt's cache dirs were not removed")
	}

	e.running("a2", ownerFacts)
	s2 := e.begin(e.input("a2", ownerFacts))
	if got := resultsOf(e.uses("a2")); strings.Join(got, ",") != "go:hit" {
		t.Fatalf("second run: %v", got)
	}
	if getFile(t, filepath.Join(s2.dir("mod"), "example.com", "m@v1", "m.go")) != "package m" || getFile(t, filepath.Join(s2.dir("build"), "ab", "cd-d")) != "compiled" {
		t.Fatal("restored content differs")
	}
	if s2.Wants() {
		t.Fatal("a restored own entry is never saved again")
	}
	s2.Finalize(e.ctx, e.finalizing("a2"))
	if e.entryCount() != 1 {
		t.Fatalf("entries %d", e.entryCount())
	}

	// a changed lockfile and a changed GOFLAGS each miss
	e.running("a3", ownerFacts)
	in := e.input("a3", ownerFacts)
	in.Detected = []detect.Fact{goFacts[0], {Kind: "lockfile", Name: "go", Path: "go.sum", SHA256: strings.Repeat("e5", 32), Confidence: detect.Certain}}
	e.begin(in)
	e.running("a4", ownerFacts)
	in = e.input("a4", ownerFacts)
	in.Environ = []string{"GOFLAGS=-race"}
	e.begin(in)
	for _, id := range []string{"a3", "a4"} {
		if got := resultsOf(e.uses(id)); strings.Join(got, ",") != "go:miss" {
			t.Fatalf("%s: %v", id, got)
		}
	}
}

func TestSessionFailedOrNotFinalizingAttemptSavesNothing(t *testing.T) {
	e := newEnv(t)
	e.running("f1", ownerFacts)
	s := e.begin(e.input("f1", ownerFacts))
	fill(t, s)
	s.Finalize(e.ctx, 2) // the attempt is still running: not a save point
	if e.entryCount() != 0 {
		t.Fatal("saved while running")
	}
	if got := resultsOf(e.uses("f1")); strings.Join(got, ",") != "go:miss,go:refused" {
		t.Fatalf("uses %v", got)
	}
	if err := e.st.Transition(e.ctx, "f1", "running", "failed", storeDetail(2)); err != nil {
		t.Fatal(err)
	}
	s.Finalize(e.ctx, 3) // a failed attempt
	s.Cleanup()
	if e.entryCount() != 0 {
		t.Fatal("a failed attempt saved a cache")
	}
}

func TestSessionRefusesToSaveARegisteredValue(t *testing.T) {
	b64 := "ZmFrZS1yZWdpc3RlcmVkLXZhbHVlLTAxMjM0NTY3ODlhYmNkZWY=" // base64 of fakeSecret
	for name, content := range map[string]string{"raw": "token=" + fakeSecret + "\n", "base64": "blob:" + b64 + "\n"} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			if err := e.scan.Register([]byte(fakeSecret)); err != nil {
				t.Fatal(err)
			}
			e.running("s1", ownerFacts)
			s := e.begin(e.input("s1", ownerFacts))
			fill(t, s)
			putFile(t, filepath.Join(s.dir("build"), "planted.txt"), content)
			s.Finalize(e.ctx, e.finalizing("s1"))
			if e.entryCount() != 0 || e.auditActions("cache.save.refused") != 1 {
				t.Fatalf("entries %d, refusal audit rows %d", e.entryCount(), e.auditActions("cache.save.refused"))
			}
			if got := resultsOf(e.uses("s1")); strings.Join(got, ",") != "go:miss,go:refused" {
				t.Fatalf("uses %v", got)
			}
			if blobs := countBlobs(t, e.home); blobs != 0 {
				t.Fatalf("%d blobs left behind by a refused save", blobs)
			}
		})
	}
}

func TestSessionNeverSavesAnEmptyCache(t *testing.T) {
	e := newEnv(t)
	e.running("e1", ownerFacts)
	s := e.begin(e.input("e1", ownerFacts))
	s.Finalize(e.ctx, e.finalizing("e1"))
	if e.entryCount() != 0 {
		t.Fatal("an empty cache was saved")
	}
}

func TestSessionSnapshotsAreReadOnlyForLowerTrust(t *testing.T) {
	setup := func(t *testing.T, writer entries.Facts) *env {
		e := newEnv(t)
		saved(t, e, "w1", writer)
		return e
	}
	t.Run("a container job with another toolchain identity misses the process-level snapshot", func(t *testing.T) {
		e := setup(t, ownerFacts)
		e.running("u1", untrustFacts)
		in := e.input("u1", untrustFacts)
		in.Container, in.Isolation, in.Image = true, "container", "sha256:"+strings.Repeat("9", 64)
		e.begin(in)
		if got := resultsOf(e.uses("u1")); strings.Join(got, ",") != "go:miss" {
			t.Fatalf("uses %v", got)
		}
	})
	t.Run("untrusted process job is denied a snapshot", func(t *testing.T) {
		e := setup(t, ownerFacts)
		e.running("u2", untrustFacts)
		s := e.begin(e.input("u2", untrustFacts))
		if got := resultsOf(e.uses("u2")); strings.Join(got, ",") != "go:denied" {
			t.Fatalf("uses %v", got)
		}
		env, mounts := s.Setup()
		if len(env) != 0 || len(mounts) != 0 || len(tree(t, s.dir("mod"))) != 0 {
			t.Fatalf("a denied restore must leave nothing: %v %v", env, mounts)
		}
	})
	for name, writer := range map[string]entries.Facts{
		"a writer that declared a secret": func() entries.Facts { f := ownerFacts; f.SecretsDeclared = true; return f }(),
		"a writer with private deps":      func() entries.Facts { f := ownerFacts; f.PrivateDeps = true; return f }(),
	} {
		t.Run("no snapshot from "+name, func(t *testing.T) {
			e := setup(t, writer)
			e.running("u3", untrustFacts)
			e.begin(e.input("u3", untrustFacts))
			if got := resultsOf(e.uses("u3")); strings.Join(got, ",") != "go:miss" {
				t.Fatalf("uses %v", got)
			}
		})
	}
	t.Run("an unprotected trusted writer is no snapshot source", func(t *testing.T) {
		f := ownerFacts
		f.Protection = "unprotected"
		e := setup(t, f)
		e.running("u4", untrustFacts)
		e.begin(e.input("u4", untrustFacts))
		if got := resultsOf(e.uses("u4")); strings.Join(got, ",") != "go:miss" {
			t.Fatalf("uses %v", got)
		}
	})
}

// TestSessionReadOnlySnapshotHit: the same toolchain identity on both sides, so
// the untrusted container job hits the owner's snapshot and gets read-only mounts.
func TestSessionReadOnlySnapshotHit(t *testing.T) {
	e := newEnv(t)
	e.seedSnapshot("w1", map[string]string{"mod": "snapshot-mod", "build": "snapshot-build"})
	s := e.beginUntrustedContainer("u1")
	if got := resultsOf(e.uses("u1")); strings.Join(got, ",") != "go:readonly" {
		t.Fatalf("uses %v", got)
	}
	env, mounts := s.Setup()
	if len(env) != 2 || len(mounts) != 2 {
		t.Fatalf("env %v mounts %v", env, mounts)
	}
	for _, m := range mounts {
		if !m.ReadOnly {
			t.Fatalf("an untrusted restore must mount read-only: %+v", m)
		}
	}
	if getFile(t, filepath.Join(s.dir("mod"), "f.txt")) != "snapshot-mod" {
		t.Fatal("restored content differs")
	}
	if s.Wants() {
		t.Fatal("a read-only restore is never saved")
	}
}

func TestSessionDisabledAndDeniedOutcomes(t *testing.T) {
	e := newEnv(t)
	e.running("d1", ownerFacts)
	in := e.input("d1", ownerFacts)
	in.JobEnv = map[string]string{"GOMODCACHE": "/job/pinned"}
	s := e.begin(in)
	if env, mounts := s.Setup(); len(env)+len(mounts) != 0 || strings.Join(resultsOf(e.uses("d1")), ",") != "go:disabled" {
		t.Fatalf("a job that pins its own cache dir keeps it: %v %v", env, mounts)
	}
	e.running("d2", ownerFacts)
	in = e.input("d2", ownerFacts)
	in.Probe = func(_ context.Context, _ []string) (string, error) { return "", os.ErrNotExist }
	e.begin(in)
	bad := untrustFacts
	bad.HeadRepo, bad.PR = "", ""
	e.running("d4", bad)
	e.begin(e.input("d4", bad))
	for id, want := range map[string]string{"d2": "go:disabled", "d4": "go:denied"} {
		if got := strings.Join(resultsOf(e.uses(id)), ","); got != want {
			t.Errorf("%s: %s, want %s", id, got, want)
		}
	}
}

func TestSessionDropsAndRefusesACraftedEntry(t *testing.T) {
	e := newEnv(t)
	ns, _ := entries.NamespaceOf(ownerFacts)
	in := e.input("c1", ownerFacts)
	e.running("c1", ownerFacts)
	item, _ := (&Session{in: in, own: ns}).plan(e.ctx, mustGet(t, "go"))
	if err := e.es.RecordPlan(e.ctx, "c1", 2, ownerFacts, []string{item.Key}); err != nil {
		t.Fatal(err)
	}
	fin := e.finalizing("c1")
	if err := e.es.RecordResult(e.ctx, "c1", fin, true); err != nil {
		t.Fatal(err)
	}
	evil := tarGz(t, tent{name: "ok.txt", body: "fine"}, tent{name: "../escape", body: "evil"})
	m := entries.Manifest{Namespace: ns.Canonical(), Key: item.Key, Spec: "go"}
	for _, name := range []string{"build", "mod"} {
		d, n, err := e.blobs.Put(e.ctx, casRealm(ns), bytes.NewReader(evil))
		if err != nil {
			t.Fatal(err)
		}
		m.Blobs = append(m.Blobs, entries.Blob{Path: name, SHA256: d, Size: n})
	}
	if err := e.es.Commit(e.ctx, "c1", fin, ns, m); err != nil {
		t.Fatal(err)
	}
	e.running("c2", ownerFacts)
	s := e.begin(e.input("c2", ownerFacts))
	if got := strings.Join(resultsOf(e.uses("c2")), ","); got != "go:refused,go:miss" {
		t.Fatalf("uses %s", got)
	}
	if e.entryCount() != 0 || e.auditActions("cache.restore.refused") != 1 {
		t.Fatalf("the crafted entry must be dropped with an audit row (entries %d)", e.entryCount())
	}
	if _, err := os.Stat(filepath.Join(e.home, "cache-work", "escape")); err == nil {
		t.Fatal("escape file written")
	}
	if len(tree(t, s.dir("mod"))) != 0 {
		t.Fatal("a refused restore left files behind")
	}
}

func TestSessionCargoBindsHomeAndSeedsConfig(t *testing.T) {
	e := newEnv(t)
	userHome := t.TempDir()
	putFile(t, userHome+"/.cargo/config.toml", "[build]\njobs = 2\n")
	putFile(t, userHome+"/.cargo/credentials.toml", "[registry]\ntoken = \"x\"\n")
	e.running("r0", ownerFacts)
	in := e.input("r0", ownerFacts)
	in.Detected = []detect.Fact{fact("stack", "rust", "Cargo.toml"), {Kind: "lockfile", Name: "cargo", Path: "Cargo.lock", SHA256: strings.Repeat("7", 64), Confidence: detect.Certain}}
	in.Home = userHome
	in.Probe = func(context.Context, []string) (string, error) { return "tool 1.0", nil }
	if s := e.begin(in); len(s.items) != 0 || strings.Join(resultsOf(e.uses("r0")), ",") != "cargo:disabled" {
		t.Fatalf("a tool home that holds credentials must not be hidden by a rebind: %v", resultsOf(e.uses("r0")))
	}
	if err := os.Remove(userHome + "/.cargo/credentials.toml"); err != nil {
		t.Fatal(err)
	}
	e.running("r1", ownerFacts)
	in.Attempt, in.Root = "r1", filepath.Join(e.home, "cache-work", "r1")
	s := e.begin(in)
	env, mounts := s.Setup()
	root := filepath.Join(e.home, "cache-work", "r1", "cargo")
	if env["CARGO_HOME"] != root || len(env) != 1 || len(mounts) != 1 || mounts[0].Host != root {
		t.Fatalf("env %v mounts %v", env, mounts)
	}
	if getFile(t, root+"/config.toml") != "[build]\njobs = 2\n" {
		t.Fatal("the user's cargo config was not seeded")
	}
	if _, err := os.Stat(root + "/credentials.toml"); err == nil {
		t.Fatal("credentials were copied into the cache dir")
	}
	putFile(t, root+"/registry/index/x", "idx")
	putFile(t, root+"/git/db/y", "db")
	putFile(t, root+"/target/debug/app", "binary") // never cached: target/ is no spec dir
	s.Finalize(e.ctx, e.finalizing("r1"))
	e.running("r2", ownerFacts)
	in.Attempt, in.Root = "r2", filepath.Join(e.home, "cache-work", "r2")
	s2 := e.begin(in)
	r2 := filepath.Join(e.home, "cache-work", "r2", "cargo")
	if getFile(t, r2+"/registry/index/x") != "idx" || getFile(t, r2+"/git/db/y") != "db" {
		t.Fatal("registry and git dirs were not restored")
	}
	if _, err := os.Stat(r2 + "/target"); err == nil {
		t.Fatal("cargo target/ was cached")
	}
	_ = s2
}

func countBlobs(t *testing.T, home string) (n int) {
	t.Helper()
	_ = filepath.WalkDir(filepath.Join(home, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && strings.Contains(p, "/sha256/") {
			n++
		}
		return nil
	})
	return n
}
