package main

// Purpose: end-to-end `runs resolve` as the cli renders a machine request (P7-CI-62 review): booleans as --abandon=true,
//          consent as --yes=true, a "--" before the positional, flags in name order after --json. Also pins that a read
//          command started by a machine caller never runs crash recovery (it kills processes and rewrites run state).
// Constraints: argv is built the way cli internal/invoke/argv.go builds it; the poster-free fixtures come from runs_resolve_test.go.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/release/job"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestRunsResolveAcceptsTheMachineRendering(t *testing.T) {
	home := seedStuckRelease(t, "needs-operator")
	t.Setenv("NSELF_INVOKED_BY", "mcp")
	t.Setenv("NSELF_NONINTERACTIVE", "1")
	key := job.IdempotencyKey(resolveAttempt)
	argv := []string{"runs", "resolve", "--json", "--abandon=true", "--idempotency-key=" + key, "--yes=true", "--", resolveAttempt}
	code, out, errs := capture(t, func() int { return runRuns(argv[1:]) })
	if code != 0 || !strings.Contains(out, `"abandon"`) {
		t.Fatalf("machine-rendered resolve: code %d stdout %q stderr %q", code, out, errs)
	}
	if got := attemptState(t, home); got != "failed" {
		t.Fatalf("state after the machine abandon = %s", got)
	}
}

func TestParseResolveRendering(t *testing.T) {
	a, err := parseResolve([]string{"--json", "--rerun=true", "--idempotency-key=k", "--yes=true", "--force=false", "--", "att"})
	if err != nil || a.attempt != "att" || a.action != job.Rerun || !a.yes || a.force || !a.asJSON {
		t.Fatalf("parse: %+v %v", a, err)
	}
	for _, bad := range [][]string{{"--rerun=true", "--abandon=true", "--idempotency-key=k", "att"}, {"--rerun", "att"}, {"--bogus", "att"}, {"--rerun", "--idempotency-key=k"}} {
		if _, err = parseResolve(bad); err == nil {
			t.Errorf("parseResolve accepted %v", bad)
		}
	}
}

// TestMachineRunsReadNeverRecovers: `runs` (a read row) must not call crash recovery for a machine caller, and must for a human.
func TestMachineRunsReadNeverRecovers(t *testing.T) {
	calls := 0
	saved := recoverRuns
	recoverRuns = func(context.Context, *store.Store, string, string) cirun.RecoverReport {
		calls++
		return cirun.RecoverReport{}
	}
	defer func() { recoverRuns = saved }()
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_INVOKED_BY", "mcp")
	if code, _, errs := capture(t, func() int { return runRuns([]string{"--json"}) }); code != 0 || calls != 0 {
		t.Fatalf("machine runs: code %d, recovery calls %d, stderr %q", code, calls, errs)
	}
	t.Setenv("NSELF_INVOKED_BY", "")
	if code, _, errs := capture(t, func() int { return runRuns([]string{"--json"}) }); code != 0 || calls != 1 {
		t.Fatalf("human runs: code %d, recovery calls %d, stderr %q", code, calls, errs)
	}
}

// TestResolveRecoversOnlyAfterConsent: a refused resolve (no --yes, no terminal) leaves the store untouched, so recovery
// (process kills, workspace and container removal, state rewrites) has not run; a confirmed one recovers once.
func TestResolveRecoversOnlyAfterConsent(t *testing.T) {
	calls := 0
	saved := recoverRuns
	recoverRuns = func(context.Context, *store.Store, string, string) cirun.RecoverReport {
		calls++
		return cirun.RecoverReport{}
	}
	defer func() { recoverRuns = saved }()
	home := seedStuckRelease(t, "needs-operator")
	t.Setenv("NSELF_INVOKED_BY", "")
	key := job.IdempotencyKey(resolveAttempt)
	code, _, errs := capture(t, func() int { return runResolve([]string{resolveAttempt, "--idempotency-key", key, "--abandon"}) })
	if code != 1 || !strings.Contains(errs, "nothing changed") || calls != 0 {
		t.Fatalf("refused resolve: code %d, recovery calls %d, stderr %q", code, calls, errs)
	}
	if got := attemptState(t, home); got != "needs-operator" {
		t.Fatalf("a refused resolve changed the attempt to %s", got)
	}
	code, _, errs = capture(t, func() int {
		return runResolve([]string{resolveAttempt, "--idempotency-key", key, "--abandon", "--yes"})
	})
	if code != 0 || calls != 1 {
		t.Fatalf("confirmed resolve: code %d, recovery calls %d, stderr %q", code, calls, errs)
	}
}
