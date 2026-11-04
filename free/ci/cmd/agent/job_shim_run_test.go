package main

// Purpose: the system-node step chain (D-1230): a process step of a system agent starts as exactly
// `sudo -n -u nself-ci-job-<s+1> /opt/nself-ci-agent/bin/nself-ci-agent job-shim` with the payload in the
// request line, a failed sudo fails the step with nothing run as the agent, the system binary outside
// the system user refuses process jobs, and every other node behaves as before.
// Inputs:  the system-mode seams and a fake sudo script. Constraints: no real sudo, no real node.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

// asNode sets the system-mode seams for one test.
func asNode(t *testing.T, home, user, exe string) {
	t.Helper()
	h, u, e := shimAgentHome, shimUserName, shimExecutable
	shimAgentHome = func() string { return home }
	shimUserName = func() string { return user }
	shimExecutable = func() string { return exe }
	t.Cleanup(func() { shimAgentHome, shimUserName, shimExecutable = h, u, e })
}

func TestNodesWithoutTheSystemInstallBehaveAsToday(t *testing.T) {
	asNode(t, "", "dev", "/home/dev/.nself/ci-agent/bin/nself-ci-agent")
	if systemShim(0) != nil {
		t.Fatal("a user-mode node got the job shim")
	}
	// a cloud VM runs as root with the same state dir and its own binary: unchanged too
	asNode(t, "/var/lib/nself-ci-agent", "root", "/usr/local/bin/nself-ci-agent")
	if systemShim(0) != nil {
		t.Fatal("a cloud VM agent got the job shim")
	}
}

func TestSystemAgentPutsTheExactChainInFrontOfAProcessStep(t *testing.T) {
	asNode(t, "/var/lib/nself-ci-agent", "nself-ci", "/opt/nself-ci-agent/bin/nself-ci-agent")
	wrap := systemShim(1)
	if wrap == nil {
		t.Fatal("the system agent did not use the job shim")
	}
	s := &ciexec.JobSpec{Command: []string{"make", "test"}}
	if err := wrap(ciexec.BaselineIsolator{}).Prepare(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/bin/sudo", "-n", "-u", "nself-ci-job-2", "/opt/nself-ci-agent/bin/nself-ci-agent", "job-shim"}
	if !reflect.DeepEqual(s.Command, want) {
		t.Fatalf("command = %q, want %q", s.Command, want)
	}
	asNode(t, "/var/db/nself-ci-agent", "nself-ci", "/opt/nself-ci-agent/bin/nself-ci-agent")
	if systemShim(0) == nil {
		t.Fatal("the macOS system agent did not use the job shim")
	}
	if s.Shim == nil || !reflect.DeepEqual(s.Shim.Payload, []string{"make", "test"}) {
		t.Fatalf("payload = %+v", s.Shim)
	}
	c := &ciexec.JobSpec{Command: []string{"make"}}
	c.Job.Isolation = "container"
	if err := wrap(ciexec.BaselineIsolator{}).Prepare(context.Background(), c); err != nil || c.Shim != nil || c.Command[0] != "make" {
		t.Fatalf("a container job was touched: %q %+v %v", c.Command, c.Shim, err)
	}
}

func TestSystemBinaryOutsideTheSystemUserRefusesProcessJobs(t *testing.T) {
	asNode(t, "", "admin", "/opt/nself-ci-agent/bin/nself-ci-agent")
	wrap := systemShim(0)
	if wrap == nil {
		t.Fatal("push mode on a system node was not fenced")
	}
	s := &ciexec.JobSpec{Command: []string{"make"}}
	if err := wrap(ciexec.BaselineIsolator{}).Prepare(context.Background(), s); err == nil || !strings.Contains(err.Error(), "E682") {
		t.Fatalf("err = %v", err)
	}
}

func TestFailedSudoFailsTheStepAndRunsNothingAsTheAgent(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran-as-agent")
	argvFile, stdinFile := filepath.Join(dir, "argv"), filepath.Join(dir, "stdin")
	fake := filepath.Join(dir, "sudo")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argvFile + "'\ncat > '" + stdinFile + "'\necho 'sudo: a password is required' >&2\nexit 1\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := shimSudo
	shimSudo = fake
	t.Cleanup(func() { shimSudo = old })
	asNode(t, "/var/lib/nself-ci-agent", "nself-ci", "/opt/nself-ci-agent/bin/nself-ci-agent")
	ex := &ciexec.Executor{Isolation: systemShim(0)(ciexec.BaselineIsolator{})}
	res := ex.Run(context.Background(), ciexec.JobSpec{AttemptID: "shim1", CoordinatorID: "coord", Home: t.TempDir(),
		Command: []string{"sh", "-c", "touch '" + marker + "'"}})
	if res.Attempt.State == "passed" || res.ExitCode != 1 {
		t.Fatalf("a failed sudo did not fail the step: state=%s exit=%d err=%v", res.Attempt.State, res.ExitCode, res.Err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("the payload ran without the shim: %v", err)
	}
	argv, _ := os.ReadFile(argvFile)
	if got := strings.Fields(string(argv)); !reflect.DeepEqual(got, []string{"-n", "-u", "nself-ci-job-1", "/opt/nself-ci-agent/bin/nself-ci-agent", "job-shim"}) {
		t.Fatalf("sudo argv = %q", got)
	}
	raw, _ := os.ReadFile(stdinFile)
	line, _, _ := strings.Cut(string(raw), "\n")
	req, err := parseShimRequest([]byte(line))
	if err != nil {
		t.Fatalf("the shim could not read the request %q: %v", line, err)
	}
	if len(req.Argv) != 3 || filepath.Base(req.Argv[0]) != "sh" || !filepath.IsAbs(req.Argv[0]) || req.Argv[2] != "touch '"+marker+"'" || !filepath.IsAbs(req.Dir) {
		t.Fatalf("request = %+v", req)
	}
	found := false
	for _, e := range req.Env {
		found = found || e == "NSELF_CI_EXEC_ATTEMPT=shim1"
	}
	if !found {
		t.Fatalf("the step's environment is not in the request: %q", req.Env)
	}
}
