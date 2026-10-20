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

// TestAgentHookAddsTheOperatorsForgeHost (P7-HOST-07): a self-hosted GitLab's host travels in the job
// (EnvHosts, set by the coordinator's Synthesize from policy) and joins the restricted allowlist for a runner
// attempt; a name that is not a plain lower-case host is dropped, and the list is bounded.
func TestAgentHookAddsTheOperatorsForgeHost(t *testing.T) {
	s := runnerSpec("33")
	s.Provider, s.Hosts = "gitlab", []string{"gitlab.example.com", "git.lan"}
	p, err := runnerjob.Synthesize(s)
	if err != nil {
		t.Fatal(err)
	}
	if p.Jobs[0].Env[runnerjob.EnvHosts] != "gitlab.example.com,git.lan" {
		t.Fatalf("env %v", p.Jobs[0].Env)
	}
	l := protocol.Lease{AttemptID: "att-gl", Job: p.Jobs[0], RunnerCredential: &protocol.RunnerCredential{Kind: "gitlab-runner-token", Value: "x"}}
	l.Job.Env[runnerjob.EnvHosts] += ",Evil.Example,a b,*.example.com,,x.y,e1.example,e2.example,e3.example"
	hook := runnerjob.Hook()
	if _, err = hook.Before(context.Background(), l, jobRoot()); err != nil {
		t.Fatal(err)
	}
	got := runnerjob.AttemptHosts(isolation.Attempt{ID: "att-gl"})
	want := []string{"gitlab.example.com", "git.lan", "x.y", "e1.example"}
	if !slices.Equal(got, want) {
		t.Fatalf("hosts %v, want %v", got, want)
	}
	_ = hook.After(context.Background(), l, jobRoot(), outcome(), nil)
}

// TestSpecHostsAreValidated (P7-HOST-07): extra hosts must be plain lower-case names, at most MaxHosts.
func TestSpecHostsAreValidated(t *testing.T) {
	for _, bad := range [][]string{{"Gitlab.com"}, {"a b"}, {"*.gitlab.com"}, {"-x.com"}, {""}, {"a", "b", "c", "d", "e"}, {"gitlab.com,evil.com"}} {
		s := runnerSpec("34")
		s.Hosts = bad
		if err := s.Validate(); err == nil {
			t.Errorf("hosts %v must be refused", bad)
		}
	}
	s := runnerSpec("34")
	s.Hosts = []string{"gitlab.com", "git.lan", "10.0.0.5"}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}
