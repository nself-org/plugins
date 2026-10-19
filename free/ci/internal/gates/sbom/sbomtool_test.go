//go:build sbomtool

package sbom_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	gsbom "github.com/nself-org/plugins/free/ci/internal/gates/sbom"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
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
	runner := sbom.ExecRunner{Executor: &exec.Executor{}, Home: t.TempDir(), SourceDir: src}
	targets := []sbom.Target{{Name: "dist", Path: "dist", Kind: sbom.SourceDir}}
	in := gsbom.Input{Runner: runner, Home: t.TempDir(), Pin: pin, Targets: targets}

	in.Job = model.Job{Kind: "release", Isolation: "container", Run: []string{"make dist"}}
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

	in.Pin = sbom.Pin{Version: "0.0.1"}
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
