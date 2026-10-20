//go:build !windows

package main

import (
	"strings"
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

// TestRunnerJobAsksForRunnerSizedResources: a job that carries a runner credential gets 2 CPUs and 2 GB (the
// runner and its memory-only root do not fit the 1 GB default); every other job keeps the default (0 = unset).
func TestRunnerJobAsksForRunnerSizedResources(t *testing.T) {
	if cpu, mem := jobCPU(jobInputs{RunnerRoot: true}), jobMemoryMB(jobInputs{RunnerRoot: true}); cpu != 2 || mem != 2048 {
		t.Fatalf("runner job: %v CPU, %d MB", cpu, mem)
	}
	if cpu, mem := jobCPU(jobInputs{}), jobMemoryMB(jobInputs{}); cpu != 0 || mem != 0 {
		t.Fatalf("ordinary job: %v CPU, %d MB", cpu, mem)
	}
}

// TestGitLabRunnerTakesTheEntrypoint (P7-HOST-07): the gitlab-runner image's own entrypoint is a launcher
// (`dumb-init /entrypoint`, which execs `gitlab-runner "$@"`), so a gitlab-runner-token job in a container
// replaces it with the mounted agent and its command starts at the wrapper; the GitHub kind keeps the
// appended command; a process job needs neither.
func TestGitLabRunnerTakesTheEntrypoint(t *testing.T) {
	for _, tc := range []struct {
		kind       string
		isolation  model.Isolation
		entrypoint string
		argv       []string
	}{
		{"gitlab-runner-token", "sandboxed-container", containerAgentPath, []string{"cred-wrapper", "gitlab-runner-token", "--", "gitlab-runner", "run-single"}},
		{"gitlab-runner-token", "container", containerAgentPath, []string{"cred-wrapper", "gitlab-runner-token", "--", "gitlab-runner", "run-single"}},
		{"github-jitconfig", "sandboxed-container", "", []string{containerAgentPath, "cred-wrapper", "github-jitconfig", "--", "gitlab-runner", "run-single"}},
	} {
		spec := execSpec{Job: model.Job{Isolation: tc.isolation, Run: []string{"gitlab-runner", "run-single"}}}
		var in jobInputs
		j := session.JobRun{Credential: []byte("GLRT-SYNTHETIC-1"), Lease: protocol.Lease{RunnerCredential: &protocol.RunnerCredential{Kind: tc.kind, Value: "GLRT-SYNTHETIC-1"}}}
		if err := wrapCredential(&spec, &in, j); err != nil {
			t.Fatal(err)
		}
		if in.Entrypoint != tc.entrypoint || strings.Join(spec.Job.Run, " ") != strings.Join(tc.argv, " ") {
			t.Fatalf("%s/%s: entrypoint %q argv %v", tc.kind, tc.isolation, in.Entrypoint, spec.Job.Run)
		}
		wantMasks := 1 + len(partialMasks(tc.kind, "GLRT-SYNTHETIC-1"))
		if in.Stdin == nil || len(in.Masks) != wantMasks || in.Masks[0] != "GLRT-SYNTHETIC-1" || !in.RunnerRoot || len(in.Mounts) != 1 {
			t.Fatalf("%s/%s: stdin %v masks %v root %v mounts %v", tc.kind, tc.isolation, in.Stdin, in.Masks, in.RunnerRoot, in.Mounts)
		}
	}
	spec := execSpec{Job: model.Job{Isolation: "process", Run: []string{"gitlab-runner"}}}
	var in jobInputs
	j := session.JobRun{Credential: []byte("X"), Lease: protocol.Lease{RunnerCredential: &protocol.RunnerCredential{Kind: "gitlab-runner-token", Value: "X"}}}
	if err := wrapCredential(&spec, &in, j); err != nil || in.Entrypoint != "" || spec.Job.Run[1] != "cred-wrapper" {
		t.Fatalf("process job: %v entrypoint %q argv %v", err, in.Entrypoint, spec.Job.Run)
	}
}

// TestPartialMasksCoverWhatGitLabRunnerPrints (P7-HOST-07): gitlab-runner prints the first nine characters of
// the token's random part (measured on v19.4.1: "with token AAAAAAAAA ..." and "runner=AAAAAAAAA"), also for a
// routable token (glrt-t2_...), so those nine are masked; other kinds and too-short values add nothing.
func TestPartialMasksCoverWhatGitLabRunnerPrints(t *testing.T) {
	for _, tc := range []struct {
		kind, value string
		want        []string
	}{
		{"gitlab-runner-token", "glrt-AAAAAAAAAAAA", []string{"AAAAAAAAA"}},
		{"gitlab-runner-token", "glrt-t2_AAAAAAAAAAAA", []string{"AAAAAAAAA"}},
		{"gitlab-runner-token", "glrt-short", nil},
		{"github-jitconfig", "glrt-AAAAAAAAAAAA", nil},
	} {
		got := partialMasks(tc.kind, tc.value)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s %q: %v, want %v", tc.kind, tc.value, got, tc.want)
		}
	}
}
