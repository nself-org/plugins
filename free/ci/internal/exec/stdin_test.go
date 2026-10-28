//go:build !windows

package exec

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func runCapture(t *testing.T, e *Executor, s JobSpec) (AttemptResult, string) {
	t.Helper()
	var mu sync.Mutex
	var out strings.Builder
	s.Output = func(p []byte) { mu.Lock(); defer mu.Unlock(); out.Write(p) }
	res := e.Run(context.Background(), s)
	mu.Lock()
	defer mu.Unlock()
	return res, out.String()
}

// TestStdinReachesAProcessJobOnly proves D-1001: a JobSpec Stdin is the job's
// standard input, not argv or the environment, and a job without one reads EOF.
func TestStdinReachesAProcessJobOnly(t *testing.T) {
	script := `read line; printf 'got=%s\n' "$line"; printf 'argv=%s env=%s\n' "$*" "$(env | grep -c STDIN-SECRET-1 || true)"`
	spec := JobSpec{AttemptID: "stdin-ok", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", script, "job"}, Stdin: strings.NewReader("STDIN-SECRET-1\n")}
	res, out := runCapture(t, &Executor{Isolation: BaselineIsolator{}}, spec)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run %+v", res)
	}
	if !strings.Contains(out, "got=STDIN-SECRET-1\n") || !strings.Contains(out, "argv= env=0") {
		t.Fatalf("stdin not delivered, or leaked to argv/env:\n%s", out)
	}
	spec = JobSpec{AttemptID: "stdin-none", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", `read line; echo "got=[$line]"`}}
	if _, out = runCapture(t, &Executor{Isolation: BaselineIsolator{}}, spec); !strings.Contains(out, "got=[]") {
		t.Fatalf("a job without Stdin must read EOF:\n%s", out)
	}
}

// TestMasksRedactTheValueEvenSplitAcrossChunks proves a value listed in Masks never
// reaches the output sink, in one write, split across writes, or left in a final partial line.
func TestMasksRedactTheValueEvenSplitAcrossChunks(t *testing.T) {
	const value = "JITCONFIG-synthetic-0123456789"
	script := `printf 'a ` + value + ` b\n'; printf 'JITCONFIG-syn'; sleep .2; printf 'thetic-0123456789 tail'`
	spec := JobSpec{AttemptID: "mask-split", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"sh", "-c", script}, Masks: []string{value}}
	spec.Job = model.Job{}
	res, out := runCapture(t, &Executor{Isolation: BaselineIsolator{}, NewRedactor: NewOutputRedactor}, spec)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run %+v", res)
	}
	if strings.Contains(out, value) || strings.Contains(out, "JITCONFIG") || strings.Count(out, maskText) != 2 || !strings.Contains(out, "a [redacted] b\n") || !strings.HasSuffix(out, "[redacted] tail") {
		t.Fatalf("value not masked:\n%q", out)
	}
}

// TestPlannedContainerForwardsStdin proves a planned container takes a job stdin (D-1001 for the
// hardened and gVisor profiles): the run is interactive, the credential reaches the CLI's stdin, the
// environment moves to fd 3, and none of it is on argv or on disk.
func TestPlannedContainerForwardsStdin(t *testing.T) {
	calls, stdin := fakeRuntimeStdin(t, "ok\n", "0")
	spec := plannedSpec(t, "plan-stdin", hardPlanner{})
	spec.Job.Env = map[string]string{"FIXTURE_TOKEN": "synthetic-env-1"}
	spec.Stdin = strings.NewReader("STDIN-SECRET-1\n")
	if res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), spec); res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run %+v", res)
	}
	if stdin() != "STDIN-SECRET-1\n" {
		t.Fatalf("job stdin %q", stdin())
	}
	if !strings.Contains(fd3Seen(), "FIXTURE_TOKEN=synthetic-env-1\n") || strings.Contains(fd3Seen(), "STDIN-SECRET-1") {
		t.Fatalf("env not on fd 3, or the credential mixed into it: %q", fd3Seen())
	}
	ran := 0
	for _, c := range calls() {
		if strings.Contains(c, "STDIN-SECRET-1") || strings.Contains(c, "synthetic-env-1") {
			t.Fatalf("a value is on argv: %q", c)
		}
		if strings.HasPrefix(c, "run ") && !strings.Contains(c, "--entrypoint") {
			ran++
			if !strings.Contains(c, " --interactive ") || !strings.Contains(c, "--env-file /dev/fd/3") {
				t.Fatalf("run call without --interactive and the fd 3 env file: %q", c)
			}
		}
	}
	if ran != 1 {
		t.Fatalf("want one job run, got %d", ran)
	}
}

// TestPlannedContainerBackstopOnInteractive: --interactive is refused without a job stdin, and a job
// stdin needs it (else the credential would never arrive and the wrapper would wait on EOF).
func TestPlannedContainerBackstopOnInteractive(t *testing.T) {
	fakeRuntime(t, "ok\n", "0")
	run := ContainerRun{Name: "nself-ci-x", Image: "alpine:3.19", AttemptID: "x"}
	cmd := ContainerCommand{Binary: "docker", Args: []string{"run", "--rm", "--interactive", "--name", "nself-ci-x", "--label", "nself.ci.attempt=x", "--user", fixtureUID(), "--network=none", "--pids-limit", "64", "--memory", "64m", "--cpus", "1", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only", "alpine:3.19", "true"}}
	if err := checkContainerCommand(cmd, run); err == nil || !strings.Contains(err.Error(), "--interactive without a job stdin") {
		t.Fatalf("an interactive run with no job stdin must be refused, got %v", err)
	}
	run.Interactive = true
	if err := checkContainerCommand(cmd, run); err != nil {
		t.Fatalf("an interactive run with a job stdin must pass: %v", err)
	}
	cmd.Args = append(cmd.Args[:2], cmd.Args[3:]...) // drop --interactive
	if err := checkContainerCommand(cmd, run); err == nil || !strings.Contains(err.Error(), "missing --interactive") {
		t.Fatalf("a job stdin without --interactive must be refused, got %v", err)
	}
}

// TestBaselineContainerForwardsStdin: the baseline docker run gets --interactive only
// when a Stdin is given, and the value arrives on the CLI's stdin, never argv.
func TestBaselineContainerForwardsStdin(t *testing.T) {
	calls, stdin := fakeRuntimeStdin(t, "ok\n", "0")
	spec := JobSpec{AttemptID: "base-stdin", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"true"},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}, Stdin: strings.NewReader("STDIN-SECRET-1\n")}
	if res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), spec); res.Err != nil {
		t.Fatal(res.Err)
	}
	if stdin() != "STDIN-SECRET-1\n" {
		t.Fatalf("stdin %q", stdin())
	}
	for _, c := range calls() {
		if strings.Contains(c, "STDIN-SECRET-1") || (strings.HasPrefix(c, "run ") && !strings.Contains(c, " --interactive ")) {
			t.Fatalf("run call %q", c)
		}
	}
	calls, _ = fakeRuntimeStdin(t, "ok\n", "0")
	spec = JobSpec{AttemptID: "base-nostdin", CoordinatorID: "coord", Home: t.TempDir(), Command: []string{"true"},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}}
	if res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), spec); res.Err != nil {
		t.Fatal(res.Err)
	}
	for _, c := range calls() {
		if strings.Contains(c, "--interactive") {
			t.Fatalf("no Stdin but run is interactive: %q", c)
		}
	}
}
