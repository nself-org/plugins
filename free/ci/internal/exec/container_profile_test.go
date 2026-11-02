//go:build !windows

package exec

import (
	"strings"
	"testing"
)

// TestPlannedContainerRunnerRoot: a runner job gets one extra, exec-capable tmpfs at /runner and nothing else
// new; the backstop accepts exactly that value and only for a run that asked for it, and requires it then.
func TestPlannedContainerRunnerRoot(t *testing.T) {
	fakeRuntime(t, "ok\n", "0")
	base := []string{"run", "--rm", "--name", "nself-ci-x", "--label", "nself.ci.attempt=x", "--user", fixtureUID(), "--network=none", "--pids-limit", "64", "--memory", "64m", "--cpus", "1",
		"--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only"}
	with := func(tmpfs ...string) ContainerCommand {
		args := append([]string(nil), base...)
		for _, v := range tmpfs {
			args = append(args, "--tmpfs", v)
		}
		return ContainerCommand{Binary: "docker", Args: append(args, "alpine:3.19", "true")}
	}
	run := ContainerRun{Name: "nself-ci-x", Image: "alpine:3.19", AttemptID: "x"}
	for name, tc := range map[string]struct {
		cmd    ContainerCommand
		root   string
		accept bool
	}{
		"asked and present":            {with("/tmp:rw,nosuid,size=256m", RunnerRootTmpfs()), RunnerRootPath, true},
		"asked and missing":            {with("/tmp:rw,nosuid,size=256m"), RunnerRootPath, false},
		"present but not asked":        {with("/tmp:rw,nosuid,size=256m", RunnerRootTmpfs()), "", false},
		"asked with a wider value":     {with("/runner:rw,exec,size=99999m"), RunnerRootPath, false},
		"asked with another path":      {with("/etc:rw,exec,size=1m"), RunnerRootPath, false},
		"another root path is refused": {with(RunnerRootTmpfs()), "/opt", false},
	} {
		run.RunnerRoot = tc.root
		err := checkContainerCommand(tc.cmd, run)
		if (err == nil) != tc.accept {
			t.Errorf("%s: accepted=%v err=%v", name, err == nil, err)
		}
	}
	if RunnerRootTmpfs() != "/runner:rw,exec,nosuid,nodev,size=384m" {
		t.Fatalf("the runner tmpfs value changed: %q", RunnerRootTmpfs())
	}
}

// TestPlannedContainerBackstopRequiresTheEnvFile (gp-gemini): a run that has an environment but whose command
// leaves out --env-file is refused; with the pipe on fd 3 nobody would read it and the writer would wait.
func TestPlannedContainerBackstopRequiresTheEnvFile(t *testing.T) {
	fakeRuntime(t, "ok\n", "0")
	args := []string{"run", "--rm", "--interactive", "--name", "nself-ci-x", "--label", "nself.ci.attempt=x", "--user", fixtureUID(), "--network=none", "--pids-limit", "64", "--memory", "64m", "--cpus", "1",
		"--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only"}
	cmd := func(extra ...string) ContainerCommand {
		return ContainerCommand{Binary: "docker", Args: append(append(append([]string(nil), args...), extra...), "alpine:3.19", "true")}
	}
	run := ContainerRun{Name: "nself-ci-x", Image: "alpine:3.19", AttemptID: "x", Interactive: true, EnvFile: envFileFD}
	if err := checkContainerCommand(cmd(), run); err == nil || !strings.Contains(err.Error(), "missing --env-file") {
		t.Fatalf("a run with an environment and no --env-file must be refused, got %v", err)
	}
	if err := checkContainerCommand(cmd("--env-file", envFileFD), run); err != nil {
		t.Fatalf("with the fd 3 env file it passes: %v", err)
	}
	run.EnvFile = ""
	run.Interactive = false
	noFlag := ContainerCommand{Binary: "docker", Args: append(append([]string(nil), args[:2]...), args[3:]...)}
	noFlag.Args = append(noFlag.Args, "alpine:3.19", "true")
	if err := checkContainerCommand(noFlag, run); err != nil {
		t.Fatalf("a run with no environment (the cleanup helper) needs no --env-file: %v", err)
	}
}

// TestPlannedContainerEntrypoint (P7-HOST-07): a runner image whose own entrypoint is a launcher (gitlab-runner)
// gets the job's entrypoint, the mounted agent. The backstop accepts exactly the entrypoint the run asked for,
// only for a run that holds a stdin (a runner credential), requires it then, and still allows the cleanup
// helper's `sh`.
func TestPlannedContainerEntrypoint(t *testing.T) {
	fakeRuntime(t, "ok\n", "0")
	const agent = "/usr/local/bin/nself-ci-agent"
	base := []string{"run", "--rm", "--interactive", "--name", "nself-ci-x", "--label", "nself.ci.attempt=x", "--user", fixtureUID(), "--network=none", "--pids-limit", "64", "--memory", "64m", "--cpus", "1",
		"--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only"}
	cmd := func(entry ...string) ContainerCommand {
		args := append([]string(nil), base...)
		for _, v := range entry {
			args = append(args, "--entrypoint", v)
		}
		return ContainerCommand{Binary: "docker", Args: append(args, "alpine:3.19", "true")}
	}
	run := ContainerRun{Name: "nself-ci-x", Image: "alpine:3.19", AttemptID: "x", Interactive: true}
	mounted := []Mount{{Host: "/host/agent", Target: agent, ReadOnly: true}}
	for name, tc := range map[string]struct {
		cmd         ContainerCommand
		entry       string
		interactive bool
		accept      bool
		noMount     bool
	}{
		"asked and present":          {cmd(agent), agent, true, true, false},
		"asked and missing":          {cmd(), agent, true, false, false},
		"present but not asked":      {cmd(agent), "", true, false, false},
		"another program":            {cmd("/bin/sh"), agent, true, false, false},
		"asked path is not a mount":  {cmd("/bin/sh"), "/bin/sh", true, false, false},
		"sh on a credential run":     {cmd("sh"), agent, true, false, false},
		"two entrypoints":            {cmd(agent, "sh"), agent, true, false, false},
		"agent not mounted":          {cmd(agent), agent, true, false, true},
		"cleanup helper sh":          {cmd("sh"), "", false, true, false},
		"asked without a job stdin":  {cmd(agent), agent, false, false, false},
		"no entrypoint and none ask": {cmd(), "", true, true, false},
	} {
		run.Entrypoint, run.Interactive, run.Mounts = tc.entry, tc.interactive, mounted
		if tc.noMount {
			run.Mounts = nil
		}
		c := tc.cmd
		if !tc.interactive { // the same command without --interactive, so only the entrypoint rule can refuse it
			c = ContainerCommand{Binary: "docker", Args: append(append([]string(nil), c.Args[:2]...), c.Args[3:]...)}
		}
		err := checkContainerCommand(c, run)
		if (err == nil) != tc.accept {
			t.Errorf("%s: accepted=%v err=%v", name, err == nil, err)
		}
	}
}
