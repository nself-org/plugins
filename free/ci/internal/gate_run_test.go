// Regression tests for Run()'s pass/fail aggregation — specifically the
// near-empty-run case at the heart of G-015.
//
// WHY these exist: wiring this gate into nself-org/plugins produced exactly
// this real output:
//
//	Stacks: node
//	  secrets:gitleaks   PASS  (2-4s)
//	Overall: PASSED
//
// One gate ran (gitleaks), so the pre-existing "zero gates" guard below never
// fired, and a required merge-gate status check reported PASSED having
// verified zero lines of code. These tests pin the fix: Run() must count only
// gates that actually verified something (Substantive && !Skipped) and must
// refuse to report an unqualified pass when that count is zero, however many
// non-substantive gates ran alongside it.
package internal

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRun_GitleaksOnlyIsNotASilentPass reproduces the exact G-015 shape: a
// node stack whose only script is unrelated to lint/typecheck/test/build (no
// workspace members either), with gitleaks left enabled. Before this fix,
// Run() would report Passed=true off the strength of the secrets scan alone.
func TestRun_GitleaksOnlyIsNotASilentPass(t *testing.T) {
	if _, err := exec.LookPath("gitleaks"); err != nil {
		t.Skip("skip: gitleaks binary not found on PATH")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("skip: git binary not found on PATH")
	}

	root := t.TempDir()
	// Mirrors nself-org/plugins' root package.json: a real script, but not
	// one of the four the node gate checks for, and no nested member
	// packages at all.
	writePackageJSON(t, root, `{"version":"1.1.7","scripts":{"ci:local":"echo ok"}}`)

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=nself-ci-test", "GIT_AUTHOR_EMAIL=ci-test@nself.org",
			"GIT_COMMITTER_NAME=nself-ci-test", "GIT_COMMITTER_EMAIL=ci-test@nself.org",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q", "-b", "main")
	runGit("add", "package.json")
	runGit("commit", "-q", "-m", "initial commit")

	result, err := Run(Config{RepoRoot: root, StepTimeout: 30})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Passed {
		t.Fatalf("G-015 regression: a run where only gitleaks executed must not pass, got:\n%s", dumpGates(result))
	}

	foundGitleaks, gitleaksPassed := false, false
	for _, g := range result.Gates {
		if g.Name == "secrets:gitleaks" {
			foundGitleaks, gitleaksPassed = true, g.Passed
		}
	}
	if !foundGitleaks {
		t.Fatalf("expected a secrets:gitleaks gate to have run, got:\n%s", dumpGates(result))
	}
	if !gitleaksPassed {
		t.Fatalf("expected the clean fixture repo to pass gitleaks (so the FAILED overall comes from the substantive-check guard, not a secrets false-positive):\n%s", dumpGates(result))
	}

	foundGuard := false
	for _, g := range result.Gates {
		if g.Name == "gate:no-substantive-checks" {
			foundGuard = true
			if g.Passed {
				t.Errorf("gate:no-substantive-checks must itself be Passed=false")
			}
		}
	}
	if !foundGuard {
		t.Fatalf("expected a gate:no-substantive-checks entry explaining the failure, got:\n%s", dumpGates(result))
	}
}

// TestRun_RealScriptCountsAsSubstantive is the positive-path counterpart: once
// a real check runs (here, a trivial passing "test" script), the substantive
// guard must not block a genuine pass.
func TestRun_RealScriptCountsAsSubstantive(t *testing.T) {
	pm := "pnpm"
	if _, err := exec.LookPath(pm); err != nil {
		pm = "npm"
		if _, err := exec.LookPath(pm); err != nil {
			t.Skip("skip: neither pnpm nor npm found on PATH")
		}
	}

	root := t.TempDir()
	writePackageJSON(t, root, `{"name":"solo","version":"1.0.0","scripts":{"test":"node -e \"process.exit(0)\""}}`)

	result, err := Run(Config{RepoRoot: root, SkipGitleaks: true, StepTimeout: 30})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !result.Passed {
		t.Fatalf("expected a genuine passing script to pass the gate, got:\n%s", dumpGates(result))
	}

	substantiveRan := false
	for _, g := range result.Gates {
		if g.Name == "node:test" && g.Substantive && !g.Skipped {
			substantiveRan = true
		}
	}
	if !substantiveRan {
		t.Fatalf("expected node:test to be marked Substantive and not Skipped, got:\n%s", dumpGates(result))
	}
}

// TestRun_NodeStackWithNoScriptsAnywhereFailsExplicitly covers a bare
// package.json with no scripts, no workspace, and no tsconfig.json: every one
// of the 4 checked scripts (lint/typecheck/test/build) resolves to an
// explicit Skipped gate with its own reason (never a bare silent gap), and
// the run as a whole fails via gate:no-substantive-checks rather than
// reporting PASSED.
//
// This also documents that the pre-existing "zero gates" branch in Run() is
// no longer reachable through the node runner specifically: runNodeGates now
// always emits a Skipped entry per unmatched script instead of an empty
// slice, which is the whole point — a silent gap must announce itself. The
// zero-gates branch still exists as defense-in-depth for a future stack with
// no runner wired in the switch.
func TestRun_NodeStackWithNoScriptsAnywhereFailsExplicitly(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"name":"empty","version":"1.0.0"}`)

	result, err := Run(Config{RepoRoot: root, SkipGitleaks: true, StepTimeout: 5})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Passed {
		t.Fatalf("a repo with no substantive checks anywhere must not pass, got:\n%s", dumpGates(result))
	}

	wantSkipped := map[string]bool{"node:lint": false, "node:typecheck": false, "node:test": false, "node:build": false}
	foundGuard := false
	for _, g := range result.Gates {
		if _, ok := wantSkipped[g.Name]; ok {
			if !g.Skipped || g.Output == "" {
				t.Errorf("%s: expected Skipped=true with a non-empty reason, got skipped=%v output=%q", g.Name, g.Skipped, g.Output)
			}
			wantSkipped[g.Name] = true
		}
		if g.Name == "gate:no-substantive-checks" {
			foundGuard = true
		}
	}
	for name, seen := range wantSkipped {
		if !seen {
			t.Errorf("expected a %s gate entry, got:\n%s", name, dumpGates(result))
		}
	}
	if !foundGuard {
		t.Fatalf("expected gate:no-substantive-checks, got:\n%s", dumpGates(result))
	}
}

func dumpGates(r *Result) string {
	var b strings.Builder
	for _, g := range r.Gates {
		b.WriteString("  " + g.Name + " passed=")
		if g.Passed {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString(" substantive=")
		if g.Substantive {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString(" skipped=")
		if g.Skipped {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString("\n")
	}
	return b.String()
}
