package sbomhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

const pinVersion = "1.54.1"

const placeholder = "#!/bin/sh\n"

func testPin() sbom.Pin {
	sum := sha256.Sum256([]byte(placeholder))
	return sbom.Pin{Version: pinVersion, Sources: []toolchain.Source{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "https://example.invalid/syft.tgz",
		SHA256: strings.Repeat("a", 64), BinarySHA256: hex.EncodeToString(sum[:])}}}
}

func installSyft(t *testing.T, home string) {
	t.Helper()
	p := sbom.ToolPath(home, pinVersion)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(placeholder), 0o755); err != nil {
		t.Fatal(err)
	}
}

// fakeSyft answers `version` and returns the golden documents for any scan, named for the target.
type fakeSyft struct {
	t     *testing.T
	scans []sbom.Cmd
}

func (f *fakeSyft) IsolationName() string { return "process" }

func (f *fakeSyft) Run(_ context.Context, c sbom.Cmd) (sbom.Result, error) {
	if len(c.Argv) == 2 && c.Argv[1] == "version" {
		return sbom.Result{Output: "Application:   syft\nVersion:       " + pinVersion + "\nBuildDate:     2026-10-01T19:00:50Z\n"}, nil
	}
	f.scans = append(f.scans, c)
	name := ""
	for i, a := range c.Argv {
		if a == "--source-name" {
			name = c.Argv[i+1]
		}
	}
	read := func(n string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "sbom", "testdata", "golden", n))
		if err != nil {
			f.t.Fatal(err)
		}
		return strings.ReplaceAll(string(b), `"name": "fx"`, `"name": "`+name+`"`)
	}
	return sbom.Result{Files: map[string][]byte{c.Collect[0]: []byte(read("dir.cdx.json")), c.Collect[1]: []byte(read("dir.spdx.json"))}}, nil
}

func releaseJob(runArgv ...string) model.Job {
	return model.Job{Kind: "release", Run: runArgv, Workdir: "pkg", Isolation: "process", Artifacts: []model.ArtifactDecl{
		{Path: "dist/*.tar.gz", Name: "tool", Kind: "archive"},
		{Path: ".nself-ci/sbom/", Kind: "sbom"},
	}}
}

func workspace(t *testing.T, files ...string) *os.Root {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		p := filepath.Join(dir, "pkg", f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("bytes of "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func newHook(t *testing.T, fs *fakeSyft) (Hook, string) {
	home := t.TempDir()
	installSyft(t, home)
	return Hook{Pin: func() (sbom.Pin, error) { return testPin(), nil }, NewRunner: func(model.Job, string, string) sbom.Runner { return fs }}, home
}

// A release job that ends with release.sbom gets both documents of every
// artifact it declares, named for the record so the publication finds them.
func TestReleaseJobGetsTwoSBOMsPerArtifact(t *testing.T) {
	fs := &fakeSyft{t: t}
	h, home := newHook(t, fs)
	ws := workspace(t, "dist/tool-1.tar.gz", "dist/tool-2.tar.gz")
	info := run.AttemptInfo{JobName: "dist", Home: home, Job: releaseJob("sh", "-c", "make dist", "release.sbom")}
	if err := h.AfterJob(context.Background(), info, ws); err != nil {
		t.Fatal(err)
	}
	if len(fs.scans) != 2 {
		t.Fatalf("%d scans, want 2", len(fs.scans))
	}
	for _, rec := range []string{"tool/tool-1.tar.gz", "tool/tool-2.tar.gz"} {
		for _, ext := range []string{".cdx.json", ".spdx.json"} {
			name := filepath.Join("pkg", sbom.OutDir, rec+ext)
			if b, err := ws.ReadFile(name); err != nil || len(b) == 0 {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}

// Jobs that are not release jobs ending with the check are left alone.
func TestHookIgnoresOtherJobs(t *testing.T) {
	fs := &fakeSyft{t: t}
	h, home := newHook(t, fs)
	ws := workspace(t, "dist/tool-1.tar.gz")
	notLast := releaseJob("release.sbom", "make")
	test := releaseJob("make", "release.sbom")
	test.Kind = "test"
	for name, job := range map[string]model.Job{"no check": releaseJob("make"), "check not last": notLast, "not a release job": test} {
		if err := h.AfterJob(context.Background(), run.AttemptInfo{JobName: "dist", Home: home, Job: job}, ws); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(fs.scans) != 0 {
		t.Errorf("%d scans for jobs that do not run the check", len(fs.scans))
	}
}

// A required check that cannot run fails the attempt: no SBOM, no release.
func TestMissingSyftFailsTheAttempt(t *testing.T) {
	h := Hook{Pin: func() (sbom.Pin, error) { return testPin(), nil }, NewRunner: func(model.Job, string, string) sbom.Runner { return &fakeSyft{t: t} }}
	ws := workspace(t, "dist/tool-1.tar.gz")
	info := run.AttemptInfo{JobName: "dist", Home: t.TempDir(), Job: releaseJob("make", "release.sbom")}
	err := h.AfterJob(context.Background(), info, ws)
	if err == nil || !strings.Contains(err.Error(), "E718") {
		t.Fatalf("got %v, want an E718 error", err)
	}
}
