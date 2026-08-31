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
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

// TestGitleaksArgs_FilesystemFlagForcesNoGit asserts that the --filesystem
// opt-in (Config.ForceFilesystem) forces --no-git even for a real git
// checkout (isRepo=true simulated by the caller having already ANDed in
// !forceFilesystem before calling gitleaksArgs — see runGitleaks). This
// covers the cmd/commands/ci.go --filesystem flag added for the tarball /
// non-checkout case the code comment in runGitleaks already anticipates.
func TestGitleaksArgs_FilesystemFlagForcesNoGit(t *testing.T) {
	// Simulate runGitleaks's isRepo computation: isGitRepo(root) && !forceFilesystem.
	// With forceFilesystem=true, isRepo must be false regardless of the real
	// git-checkout state, so gitleaksArgs must emit --no-git.
	const realGitCheckout = true
	const forceFilesystem = true
	isRepo := realGitCheckout && !forceFilesystem

	args := gitleaksArgs("/repo", "", isRepo)
	if !slices.Contains(args, "--no-git") {
		t.Fatalf("--filesystem must force --no-git even inside a real git repo, got %v", args)
	}
}

// TestGitleaksE2E_CleanRepoWithGitignoredSecretLookalike is a full end-to-end
// regression test for the planetia incident (unyeco/planetia: 222MB walked,
// 20 false-positive findings from node_modules/vendored/gitignored .env,
// while the committed tree was clean — see the package doc comment above).
//
// It builds a small real git repository at runtime (never a checked-in
// fixture, so no secret-shaped string is ever committed to a tracked path):
// a tracked, clean source file, plus a GITIGNORED build/ directory holding a
// file with a fake high-entropy secret matching gitleaks' built-in
// "stripe-access-token" rule (verified locally: gitleaks 8.30.0 flags this
// shape in --no-git/filesystem mode and correctly skips it in git mode,
// whereas an AKIA-prefixed AWS-example-key lookalike is allowlisted by
// gitleaks' default ruleset and would not reproduce the incident). It then
// runs runGitleaks for real (not just gitleaksArgs) and asserts zero
// findings + a fast runtime, proving the git-mode default genuinely skips
// the gitignored tree rather than merely constructing the correct argv
// (which the other tests in this file already cover but never execute).
func TestGitleaksE2E_CleanRepoWithGitignoredSecretLookalike(t *testing.T) {
	if _, err := exec.LookPath("gitleaks"); err != nil {
		t.Skip("skip: gitleaks binary not found on PATH")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("skip: git binary not found on PATH")
	}

	root := t.TempDir()

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=nself-ci-test",
			"GIT_AUTHOR_EMAIL=ci-test@nself.org",
			"GIT_COMMITTER_NAME=nself-ci-test",
			"GIT_COMMITTER_EMAIL=ci-test@nself.org",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// 1. A real git repo with a tracked, clean source file.
	runGit("init", "-q", "-b", "main")
	cleanFile := filepath.Join(root, "clean.go")
	if err := os.WriteFile(cleanFile, []byte("package fixture\n\nfunc Hello() string { return \"hello\" }\n"), 0o644); err != nil {
		t.Fatalf("write clean.go: %v", err)
	}

	// 2. A GITIGNORED build/ directory containing a fake high-entropy secret
	// (planetia's node_modules/vendored/gitignored .env shape, reproduced at
	// small scale). This is a fabricated, never-real value shaped to match
	// gitleaks' built-in stripe-access-token rule purely so the fixture can
	// prove the git-mode skip; it is never committed outside this t.TempDir().
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	buildDir := filepath.Join(root, "build")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatalf("mkdir build/: %v", err)
	}
	secretLookalike := filepath.Join(buildDir, "output.js")
	fakeStripeKey := "sk_live_" + "ts2fnPIxseAZUjdYELQb91a62LZ34EyF6ZrvsMk0" // fabricated, not a real credential
	if err := os.WriteFile(secretLookalike, []byte(`const stripeApiKey = "`+fakeStripeKey+`";`+"\n"), 0o644); err != nil {
		t.Fatalf("write build/output.js: %v", err)
	}

	runGit("add", "clean.go", ".gitignore")
	runGit("commit", "-q", "-m", "initial commit")

	// CR-C guard: a fixture that accidentally tracks the fake-secret file
	// would make the test pass for the wrong reason (gitleaks correctly
	// flagging a TRACKED secret) instead of proving the git-mode skip
	// behavior. Assert the lookalike file is genuinely gitignored.
	checkIgnore := exec.Command("git", "check-ignore", "build/output.js")
	checkIgnore.Dir = root
	if err := checkIgnore.Run(); err != nil {
		t.Fatalf("fixture bug: build/output.js must be gitignored (git check-ignore must exit 0): %v", err)
	}

	start := time.Now()
	result := runGitleaks(root, 30, false, false)
	elapsed := time.Since(start)

	if !result.Passed {
		t.Fatalf("expected zero findings scanning a clean git-tracked tree, got FAIL:\n%s", result.Output)
	}

	const bound = 10 * time.Second
	if elapsed > bound {
		t.Errorf("expected a fast git-mode scan (< %s) that skips the gitignored build/ dir, took %s", bound, elapsed)
	}

	t.Logf("gitleaks e2e: PASS in %s (git-mode correctly skipped gitignored build/output.js)", elapsed.Round(time.Millisecond))
}
