package runnerjob_test

import (
	"context"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

// TestAgentHookGivesOnlyARunnerAttemptItsForgeHosts: the restricted scope's extra allowlist is the forge's
// host list for an attempt whose lease carries a runner credential, nothing for any other attempt, and it
// is gone when the lease ends.
func TestAgentHookGivesOnlyARunnerAttemptItsForgeHosts(t *testing.T) {
	hook := runnerjob.Hook()
	runner := protocol.Lease{AttemptID: "att-runner", Job: runnerJob(t, "31"), RunnerCredential: &protocol.RunnerCredential{Kind: "github-jitconfig", Value: "x"}}
	runner.Job.Env["NSELF_RUNNERJOB_PROVIDER"] = "github"
	plain := protocol.Lease{AttemptID: "att-plain", Job: runnerJob(t, "32")} // looks like a runner job but the lease has no credential
	plain.Job.Env["NSELF_RUNNERJOB_PROVIDER"] = "github"
	for _, l := range []protocol.Lease{runner, plain} {
		if _, err := hook.Before(context.Background(), l, jobRoot()); err != nil {
			t.Fatal(err)
		}
	}
	hosts := runnerjob.AttemptHosts(isolation.Attempt{ID: "att-runner"})
	for _, want := range []string{"github.com", "api.github.com", "ghcr.io", "codeload.github.com"} {
		if !slices.Contains(hosts, want) {
			t.Errorf("missing %s in %v", want, hosts)
		}
	}
	for _, bad := range hosts {
		if bad == "" || bad[0] == '*' {
			t.Errorf("the scope allowlist matches exact names; %q cannot work", bad)
		}
	}
	if got := runnerjob.AttemptHosts(isolation.Attempt{ID: "att-plain"}); len(got) != 0 {
		t.Fatalf("a lease with no runner credential gets no extra host: %v", got)
	}
	if got := runnerjob.AttemptHosts(isolation.Attempt{ID: "unknown"}); len(got) != 0 {
		t.Fatalf("unknown attempt: %v", got)
	}
	if err := hook.After(context.Background(), runner, jobRoot(), outcome(), nil); err != nil {
		t.Fatal(err)
	}
	if got := runnerjob.AttemptHosts(isolation.Attempt{ID: "att-runner"}); len(got) != 0 {
		t.Fatalf("hosts must be forgotten when the lease ends: %v", got)
	}
}
