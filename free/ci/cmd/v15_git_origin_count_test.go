//go:build !windows

package main

// Purpose: bound the real V15 tiny-run git spawns when a local origin exists.
// Inputs: the shared counting-PATH fixture with a fetched main branch.
// Outputs: a total budget and a bound on protected-branch fetches.
// Constraints: fixture setup is excluded; every counted git execs the real git.
// SPORT: local origin keeps the regression test independent of network access.

import (
	"strings"
	"testing"
)

func TestTinyV15GitSpawnBudgetWithOrigin(t *testing.T) {
	calls, stdout, exit := tinyGitRun(t, true)
	if exit != 0 {
		t.Fatalf("tiny run with origin exited %d: %s", exit, stdout)
	}
	// Code accounts for 44 direct spawns: planning 7, revision 4, source trust
	// 10, project identity 2, policy 14, evidence 7. Four extra slots allow
	// version-dependent maintenance children from the two local fetches.
	// The helper disables status posting and stubs Go, so neither adds calls.
	const maxGitSpawns = 48
	if len(calls) == 0 || len(calls) > maxGitSpawns {
		t.Fatalf("git spawns = %d; want 1..%d:\n%s", len(calls), maxGitSpawns, strings.Join(calls, "\n"))
	}
	fetches := 0
	for _, call := range calls {
		for _, arg := range strings.Split(call, "\t") {
			if arg == "fetch" {
				fetches++
				break
			}
		}
	}
	// Option resolution precedes the coordinator snapshot; all coordinator
	// admissions share their first successful branch read.
	if fetches == 0 || fetches > 2 {
		t.Fatalf("origin fetches = %d; want 1..2:\n%s", fetches, strings.Join(calls, "\n"))
	}
}
