package main

// Purpose: the agent half of the A01 delivery floor (P7-TRUST-18, review item M3). Delivery raises a
// plain `process` job (no `container:` image) to the trust floor (dispatch.TestDispatchRaisesDeliveredIsolationToTheFloor
// proves the lease carries `container` or above); this proves what the agent then does with it.
// Constraints: behaviour under test is FAIL CLOSED. A raised job with no image is refused with E613
// and never runs as a host process. FOLLOW-UP (debt row filed by the lead): remote jobs raised to a
// container level need a default toolchain image, or they fail on every attempt; do not "fix" this test
// by letting the job fall back to the host.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestRaisedJobWithoutImageIsRefusedNotRunOnTheHost(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	dir := t.TempDir()
	hostMarker, dockerMarker := filepath.Join(dir, "ran-on-host"), filepath.Join(dir, "docker-run")
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\n"+
		// `docker info` answers so the container level is available; any other verb (a run) leaves a marker
		"if [ \"$1\" = info ]; then echo '{\"ServerVersion\":\"27.0\",\"MemoryLimit\":true,\"PidsLimit\":true,\"CPUCfsQuota\":true,\"SecurityOptions\":[\"name=rootless\"]}'; exit 0; fi\n"+
		"echo \"$1\" >> "+dockerMarker+"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, level := range []model.Isolation{"container"} { // what delivery writes for a plain collaborator job
		spec := execSpec{AttemptID: "a-" + string(level), CoordinatorID: "c", JobID: "j", Revision: rev40,
			Job: model.Job{Isolation: level, Run: []string{"touch", hostMarker}}} // Container is nil: no image
		res, _, err := execute(context.Background(), spec, jobInputs{})
		if err != nil {
			t.Fatal(err)
		}
		var ce *ciexec.Error
		if !errors.As(res.Err, &ce) || ce.Code != "E613" || res.Attempt.State != "failed" || res.Attempt.FailureClass != "infra" {
			t.Fatalf("%s: err=%v state=%s class=%s, want E613 failed/infra", level, res.Err, res.Attempt.State, res.Attempt.FailureClass)
		}
		if _, err := os.Stat(hostMarker); err == nil {
			t.Fatalf("%s: the job ran as a host process", level)
		}
		if verbs, _ := os.ReadFile(dockerMarker); strings.Contains("\n"+string(verbs), "\nrun\n") {
			t.Fatalf("%s: a container was started", level)
		}
	}
}

// TestAgentRunsAContainerJobThroughTheHardenedChain (P7-TRUST-18, A01, review item M1): the agent builds its
// executor with the Isolator chain. An executor without it runs the same imaged job in the legacy container
// (no cap-drop, no no-new-privileges, no --user), so dropping `Isolation: iso` from execute() fails here.
func TestAgentRunsAContainerJobThroughTheHardenedChain(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	dir := t.TempDir()
	runLog := filepath.Join(dir, "docker-run-args")
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\n"+
		"if [ \"$1\" = info ]; then echo '{\"ServerVersion\":\"27.0\",\"MemoryLimit\":true,\"PidsLimit\":true,\"CPUCfsQuota\":true,\"SecurityOptions\":[\"name=rootless\"]}'; exit 0; fi\n"+
		"if [ \"$1\" = run ]; then echo \"$@\" >> "+runLog+"; fi\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	spec := execSpec{AttemptID: "a-hard", CoordinatorID: "c", JobID: "j", Revision: rev40,
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine"}, Run: []string{"true"}}}
	res, _, err := execute(context.Background(), spec, jobInputs{})
	if err != nil || res.Err != nil {
		t.Fatalf("execute: err=%v result err=%v", err, res.Err)
	}
	raw, _ := os.ReadFile(runLog)
	for _, want := range []string{"--cap-drop=ALL", "no-new-privileges", "--user", "--pids-limit", "--network=none"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the agent's container run lacks %s: %s", want, raw)
		}
	}
}
