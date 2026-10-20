package main

// Purpose: `nself-ci-agent uninstall --system`: registration, usage, and the refusals that need no root
// (no family for this OS, not running as root). The removal itself is proved by the macOS leg
// (test/macos) and by the darwin family's script tests.
// Constraints: nothing is removed here; the test never runs as root on purpose.

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestUninstallRegisteredAndNeedsSystemFlag(t *testing.T) {
	if _, ok := lookup("uninstall"); !ok {
		t.Fatal("uninstall is not in the agent's subcommand table")
	}
	for _, args := range [][]string{{"uninstall"}, {"uninstall", "--system=false"}, {"uninstall", "--system", "extra"}, {"uninstall", "--nope"}} {
		if code, _, stderr := runCmd(t, args...); code != 2 || !strings.Contains(stderr, "usage:") {
			t.Errorf("%v: code %d stderr %q", args, code, stderr)
		}
	}
}

func TestUninstallRefusesWithoutAFamilyOrWithoutRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("run the tests as an ordinary user: this test must not remove a real install")
	}
	code, _, stderr := runCmd(t, "uninstall", "--system")
	if code != 1 {
		t.Fatalf("code %d stderr %q", code, stderr)
	}
	if runtime.GOOS == "darwin" {
		if !strings.Contains(stderr, "must run as root") {
			t.Fatalf("stderr %q", stderr)
		}
		return
	}
	if !strings.Contains(stderr, "E669") || !strings.Contains(stderr, "nothing to remove") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestLocalRunUsesFixedProgramsOnly(t *testing.T) {
	ctx := context.Background()
	out, err := localRun(ctx, "sh", "-c", "echo hi")
	if err != nil || strings.TrimSpace(out) != "hi" {
		t.Fatalf("sh -c: %q %v", out, err)
	}
	if _, err = localRun(ctx, "sh", "-c", "echo bad >&2; exit 3"); err == nil {
		t.Fatal("a failing command must fail")
	}
	if _, err = localRun(ctx, "rm", "-rf", "/"); err == nil || !strings.Contains(err.Error(), "no program named rm") {
		t.Fatalf("an unlisted program ran: %v", err)
	}
}
