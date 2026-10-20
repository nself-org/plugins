//go:build !windows

package main

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// recordingIsolator stands in for the TRUST-04/05 registry: it records the spec the executor hands it and
// refuses with E679, so no container starts.
type recordingIsolator struct{ got []ciexec.JobSpec }

func (r *recordingIsolator) Prepare(_ context.Context, s *ciexec.JobSpec) error {
	r.got = append(r.got, *s)
	return &ciexec.Error{Code: "E679", Message: "recorded"}
}

// TestAgentExecutorUsesTheIsolationRegistry (D-1367): the agent's executor must run a lease through the
// registered isolation (not the baseline, which refuses sandboxed-container with E611), and a lease that carries
// a runner credential runs with the restricted network scope, the runner tmpfs and the wrapped stdin.
func TestAgentExecutorUsesTheIsolationRegistry(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_WORK", t.TempDir())
	rec := &recordingIsolator{}
	prev := cirun.Registered.Isolation
	cirun.RegisterIsolation(rec)
	t.Cleanup(func() { cirun.RegisterIsolation(prev) })

	job := model.Job{Isolation: "sandboxed-container", Run: []string{"/home/runner/run.sh"}}
	lease := protocol.Lease{
		AttemptID: "a1", Job: job,
		Checkout:         protocol.Checkout{Revision: "0123456789012345678901234567890123456789"},
		RunnerCredential: &protocol.RunnerCredential{Kind: "github-jitconfig", Value: "JIT-SYNTHETIC-1"},
	}
	res, err := leaseRunner{coordinator: "c1"}.Run(context.Background(), session.JobRun{Lease: lease, Credential: []byte("JIT-SYNTHETIC-1")})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("the registered isolator was called %d times, want 1 (outcome %+v)", len(rec.got), res.Outcome)
	}
	s := rec.got[0]
	if s.Job.Isolation != "sandboxed-container" || s.Network != "restricted" || !s.RunnerRoot || s.Stdin == nil {
		t.Fatalf("spec given to the isolator: isolation %q network %q runner root %v stdin %v", s.Job.Isolation, s.Network, s.RunnerRoot, s.Stdin)
	}
}

// TestOrdinaryLeaseKeepsTheDefaultNetwork: a lease without a runner credential sets no network scope (none).
func TestOrdinaryLeaseKeepsTheDefaultNetwork(t *testing.T) {
	t.Setenv("NSELF_CI_AGENT_WORK", t.TempDir())
	rec := &recordingIsolator{}
	prev := cirun.Registered.Isolation
	cirun.RegisterIsolation(rec)
	t.Cleanup(func() { cirun.RegisterIsolation(prev) })
	lease := protocol.Lease{AttemptID: "a2", Job: model.Job{Isolation: "container", Run: []string{"true"}}, Checkout: protocol.Checkout{Revision: "0123456789012345678901234567890123456789"}}
	if _, err := (leaseRunner{coordinator: "c1"}).Run(context.Background(), session.JobRun{Lease: lease}); err != nil {
		t.Fatal(err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("the registered isolator was called %d times, want 1", len(rec.got))
	}
	if rec.got[0].Network != "" {
		t.Fatalf("network %q, want unset", rec.got[0].Network)
	}
}
