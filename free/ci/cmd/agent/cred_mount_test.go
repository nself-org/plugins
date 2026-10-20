//go:build !windows

package main

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// TestCredentialWrapperIsMountedForEveryContainerLevel proves P7-HOST-19: a runner job under
// sandboxed-container (gVisor) runs the wrapper from the read-only agent mount in the container, as the
// container level does, and a process job runs the host binary.
func TestCredentialWrapperIsMountedForEveryContainerLevel(t *testing.T) {
	for _, tc := range []struct {
		isolation model.Isolation
		mounted   bool
	}{{"container", true}, {"sandboxed-container", true}, {"process", false}, {"", false}} {
		spec := execSpec{Job: model.Job{Isolation: tc.isolation, Run: []string{"/home/runner/run.sh"}}}
		var in jobInputs
		j := session.JobRun{Credential: []byte("JIT-SYNTHETIC-1"), Lease: protocol.Lease{RunnerCredential: &protocol.RunnerCredential{Kind: "github-jitconfig", Value: "JIT-SYNTHETIC-1"}}}
		if err := wrapCredential(&spec, &in, j); err != nil {
			t.Fatal(err)
		}
		inContainer := spec.Job.Run[0] == containerAgentPath
		if inContainer != tc.mounted || (len(in.Mounts) == 1) != tc.mounted {
			t.Fatalf("isolation %q: wrapper %q, mounts %v", tc.isolation, spec.Job.Run[0], in.Mounts)
		}
		if in.RunnerRoot != tc.mounted {
			t.Fatalf("isolation %q: the runner tmpfs is asked for exactly when the job runs in a container (got %v)", tc.isolation, in.RunnerRoot)
		}
		if tc.mounted && !in.Mounts[0].ReadOnly {
			t.Fatalf("isolation %q: the agent mount must be read-only", tc.isolation)
		}
		if spec.Job.Run[1] != "cred-wrapper" || spec.Job.Run[2] != "github-jitconfig" || spec.Job.Run[3] != "--" || spec.Job.Run[4] != "/home/runner/run.sh" {
			t.Fatalf("isolation %q: argv %v", tc.isolation, spec.Job.Run)
		}
	}
}
