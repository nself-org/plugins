// Regression tests for the gitleaks gate argv.
//
// WHY these exist: nself 1.2.0 shipped gitleaks with an UNCONDITIONAL --no-git, which
// made it scan the raw filesystem instead of git-tracked content. On unyeco/planetia
// that meant walking 222 MB (node_modules is 1.4 GB there), reporting 20 findings --
// every one of them an untracked build bundle, a vendored CocoaPod, or a GITIGNORED
// local .env -- while the committed tree was clean. A gate that cannot pass on a clean
// repo cannot be a required status check, which is exactly what nself ci exists to be.
//
// This package had no tests, so nothing caught it.
package internal

import (
	"slices"
	"strings"
	"testing"
)

func TestGitleaksArgs_GitRepoDoesNotDisableGitMode(t *testing.T) {
	args := gitleaksArgs("/repo", "", true)
	if slices.Contains(args, "--no-git") {
		t.Fatalf("a git checkout must be scanned in git mode; --no-git present: %v", args)
	}
	if !slices.Contains(args, "detect") {
		t.Errorf("expected a detect scan, got %v", args)
	}
	if !slices.Contains(args, "--exit-code") {
		t.Errorf("gate must request a nonzero exit on findings, got %v", args)
	}
}

func TestGitleaksArgs_NonRepoFallsBackToFilesystem(t *testing.T) {
	args := gitleaksArgs("/tarball", "", false)
	if !slices.Contains(args, "--no-git") {
		t.Fatalf("a non-checkout must still be scannable via --no-git, got %v", args)
	}
}

func TestGitleaksArgs_UsesRepoConfigWhenPresent(t *testing.T) {
	args := gitleaksArgs("/repo", "/repo/.gitleaks.toml", true)
	i := slices.Index(args, "--config")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("repo allowlist must be passed through, got %v", args)
	}
	if got := args[i+1]; got != "/repo/.gitleaks.toml" {
		t.Errorf("wrong config path: %q", got)
	}
}

func TestGitleaksArgs_OmitsConfigFlagWhenAbsent(t *testing.T) {
	args := gitleaksArgs("/repo", "", true)
	if slices.Contains(args, "--config") {
		t.Errorf("must not pass an empty --config, got %v", args)
	}
}

func TestGitleaksArgs_SourceIsTheRepoRoot(t *testing.T) {
	args := gitleaksArgs("/some/root", "", true)
	i := slices.Index(args, "--source")
	if i < 0 || i+1 >= len(args) || args[i+1] != "/some/root" {
		t.Errorf("--source must be the repo root, got %v", args)
	}
	if strings.TrimSpace(args[i+1]) == "" {
		t.Error("--source must not be empty")
	}
}
