package android

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

// installSyft places the placeholder where `nself ci artifacts tools install` would.
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

// workspace is a job workspace with APKs (names) built under workdir.
func workspace(t *testing.T, workdir string, apks ...string) *os.Root {
	t.Helper()
	dir := t.TempDir()
	for _, a := range apks {
		p := filepath.Join(dir, workdir, "app/build/outputs/apk/release", a)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("apk "+a), 0o644); err != nil {
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

func TestSBOMStepWritesDocumentsForEveryAPK(t *testing.T) {
	home := t.TempDir()
	installSyft(t, home)
	ws := workspace(t, "mobile", "app-arm64-release.apk", "app-x86-release.apk")
	fs := &fakeSyft{t: t}
	step := SBOMStep{Home: home, Pin: testPin(), NewRunner: func(dir string) sbom.Runner {
		if !strings.HasSuffix(dir, filepath.Join("mobile", "app", "build", "outputs", "apk", "release")) {
			t.Errorf("syft source dir %s", dir)
		}
		return fs
	}}
	if err := step.Run(context.Background(), Job("mobile", nil, 0, nil), ws); err != nil {
		t.Fatal(err)
	}
	if len(fs.scans) != 2 {
		t.Fatalf("%d scans", len(fs.scans))
	}
	for _, apk := range []string{"app-arm64-release.apk", "app-x86-release.apk"} {
		for _, ext := range []string{".cdx.json", ".spdx.json"} {
			name := filepath.Join("mobile", SBOMDir, ArtifactName, apk+ext)
			b, err := ws.ReadFile(name)
			if err != nil || len(b) == 0 {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}

func TestSBOMStepFailures(t *testing.T) {
	home := t.TempDir()
	job := Job(".", nil, 0, nil)
	step := SBOMStep{Home: home, Pin: testPin(), NewRunner: func(string) sbom.Runner { return &fakeSyft{t: t} }}
	if err := step.Run(context.Background(), job, workspace(t, ".")); err == nil {
		t.Error("no APK directory was accepted")
	}
	if err := step.Run(context.Background(), job, workspace(t, ".", "notes.txt")); err == nil {
		t.Error("a directory with no APK was accepted")
	}
	ws := workspace(t, ".", "a.apk")
	err := step.Run(context.Background(), job, ws)                                           // syft is not installed
	if r := sbom.ReasonOf(err); r != sbom.ReasonToolMissing && r != sbom.ReasonToolVersion { // version when an unpinned syft is on PATH
		t.Fatalf("a missing pinned syft: %v", err)
	}
	if _, serr := ws.Stat(SBOMDir); serr == nil {
		t.Error("documents were written without a trusted syft")
	}
}

func TestApplies(t *testing.T) {
	rel, test := Job(".", nil, 0, nil), Job(".", nil, 0, nil)
	test.Kind = "test"
	if !Applies(JobName, rel) || Applies("other", rel) || Applies(JobName, test) {
		t.Fatal("Applies is wrong")
	}
}
