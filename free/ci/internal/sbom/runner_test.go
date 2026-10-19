package sbom_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

func execRunner(t *testing.T, source string) sbom.ExecRunner {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh fixtures")
	}
	return sbom.ExecRunner{Executor: &exec.Executor{}, Home: t.TempDir(), SourceDir: source}
}

func TestExecRunnerRunsInTheWorkspaceAndCollectsFiles(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "in.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := execRunner(t, src)
	res, err := r.Run(context.Background(), sbom.Cmd{
		Argv:    []string{"/bin/sh", "-c", "mkdir -p .nself-ci/sbom && cat in.txt > .nself-ci/sbom/out.txt && echo \"$SYFT_CHECK_FOR_APP_UPDATE\""},
		Env:     map[string]string{"SYFT_CHECK_FOR_APP_UPDATE": "false"},
		Collect: []string{".nself-ci/sbom/out.txt"},
	})
	if err != nil || res.Exit != 0 {
		t.Fatalf("run: %+v %v", res, err)
	}
	if string(res.Files[".nself-ci/sbom/out.txt"]) != "payload" || strings.TrimSpace(res.Output) != "false" {
		t.Fatalf("res = %+v", res)
	}
	if _, statErr := os.Stat(filepath.Join(src, ".nself-ci")); !os.IsNotExist(statErr) {
		t.Fatal("the command wrote into the source directory instead of its workspace")
	}
}

func TestExecRunnerReportsExitAndMissingFile(t *testing.T) {
	r := execRunner(t, t.TempDir())
	res, err := r.Run(context.Background(), sbom.Cmd{Argv: []string{"/bin/sh", "-c", "echo nope; exit 3"}})
	if err != nil || res.Exit != 3 || !strings.Contains(res.Output, "nope") {
		t.Fatalf("exit: %+v %v", res, err)
	}
	if _, err := r.Run(context.Background(), sbom.Cmd{Argv: []string{"/bin/sh", "-c", "true"}, Collect: []string{"absent.json"}}); err == nil {
		t.Fatal("a missing collected file passed")
	}
	if _, err := (sbom.ExecRunner{}).Run(context.Background(), sbom.Cmd{Argv: []string{"x"}}); err == nil {
		t.Fatal("nil executor passed")
	}
}
