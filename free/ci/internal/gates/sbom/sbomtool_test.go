//go:build sbomtool

package sbom_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	gsbom "github.com/nself-org/plugins/free/ci/internal/gates/sbom"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// TestReleaseSBOMRequired runs the check against the pinned syft on PATH (a
// prerequisite: the test fails, never skips, without it). A release run without
// the sbom check is refused with E718; with it, a real artifact is inventoried
// and the documents pass the structural check; a pin that differs from the
// syft on PATH fails the required check with E718 tool_version.
func TestReleaseSBOMRequired(t *testing.T) {
	ctx := context.Background()
	pin, err := sbom.EmbeddedPin()
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "dist", "go.mod"), []byte("module example.com/fx\n\ngo 1.22\n\nrequire github.com/google/uuid v1.6.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a process-level job: syft comes only from $NSELF_CI_HOME/tools, so place a
	// copy of the pinned PATH syft there (what `artifacts tools install` does)
	onPath, err := osexec.LookPath("syft")
	if err != nil {
		t.Fatalf("syft %s must be on PATH for the sbomtool tests: %v", pin.Version, err)
	}
	home := t.TempDir()
	installed := sbom.ToolPath(home, pin.Version)
	bin, err := os.ReadFile(onPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// the PATH syft is a copy of the pinned release, possibly built for another platform, so
	// the test pins the digest of the copy it placed (the digest check has its own tests)
	sum := sha256.Sum256(bin)
	withDigest := func(p sbom.Pin) sbom.Pin {
		var srcs []toolchain.Source
		for _, x := range p.Sources {
			if x.OS != runtime.GOOS || x.Arch != runtime.GOARCH { // the placed copy replaces the pinned host row
				srcs = append(srcs, x)
			}
		}
		p.Sources = append(srcs, toolchain.Source{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "https://example.invalid/syft.tgz", SHA256: strings.Repeat("a", 64), BinarySHA256: hex.EncodeToString(sum[:])})
		return p
	}
	pin = withDigest(pin)
	runner := sbom.ExecRunner{Executor: &exec.Executor{Isolation: exec.BaselineIsolator{}}, Home: t.TempDir(), SourceDir: src}
	targets := []sbom.Target{{Name: "dist", Path: "dist", Kind: sbom.SourceDir}}
	in := gsbom.Input{Runner: runner, Home: home, Pin: pin, Targets: targets}

	in.Job = model.Job{Kind: "release", Run: []string{"make dist"}}
	refused := gsbom.Run(ctx, in).Check
	if refused.Result != "error" || !refused.Required || !strings.HasPrefix(refused.Excerpt, "E718 invalid:") {
		t.Fatalf("a release run without the sbom check: %+v", refused)
	}

	in.Job.Run = []string{"make dist", gsbom.CheckID}
	out := gsbom.Run(ctx, in)
	if out.Check.Result != "pass" || len(out.Docs) != 1 {
		t.Fatalf("release run with the check (syft %s must be on PATH): %+v", pin.Version, out.Check)
	}
	if names := componentNames(t, out.Docs[0].CDX); !names["github.com/google/uuid"] {
		t.Fatalf("components %v", names)
	}

	// a pin that differs from the installed syft: the probe reports the drift
	in.Pin = withDigest(sbom.Pin{Version: "0.0.1"})
	if err := os.MkdirAll(filepath.Dir(sbom.ToolPath(home, "0.0.1")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sbom.ToolPath(home, "0.0.1"), bin, 0o755); err != nil {
		t.Fatal(err)
	}
	drift := gsbom.Run(ctx, in).Check
	if drift.Result != "error" || !strings.HasPrefix(drift.Excerpt, "E718 tool_version:") || !strings.Contains(drift.Excerpt, sbom.InstallCommand) {
		t.Fatalf("version drift in a release job: %+v", drift)
	}
}

func componentNames(t *testing.T, doc []byte) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, name := range []string{"github.com/google/uuid"} {
		if strings.Contains(string(doc), `"name": "`+name+`"`) || strings.Contains(string(doc), `"name":"`+name+`"`) {
			out[name] = true
		}
	}
	return out
}
