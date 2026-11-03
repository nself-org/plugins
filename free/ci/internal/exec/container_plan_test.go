//go:build !windows

package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// fakeRuntime puts a `docker` script first on PATH that records its calls and
// replays the configured output and exit code.
func fakeRuntime(t *testing.T, out string, code string) (calls func() []string) {
	calls, _ = fakeRuntimeStdin(t, out, code)
	return calls
}

// fakeRuntimeStdin is fakeRuntime that also returns what the run command read from stdin.
func fakeRuntimeStdin(t *testing.T, out string, code string) (calls func() []string, stdin func() string) {
	t.Helper()
	dir := t.TempDir()
	fakeRuntimeDir = dir
	script := `#!/bin/sh
d=$(dirname "$0")
case "$1" in inspect) cat "$d/inspect" 2>/dev/null; exit 0;; esac
echo "$*" >> "$d/calls"
case "$*" in *--entrypoint*) exit 0;; esac
case "$1" in
events) cat "$d/events" 2>/dev/null; exit 0;;
rm)
  if [ -e "$d/cgroup" ]; then
    scope=$(cat "$d/cgroup")
    rm "$scope/pids.events" "$scope/pids.max" "$scope/memory.events"
    rmdir "$scope"
  fi
  exit 0;;
run) ;;
*) exit 0;;
esac
case "$*" in *"--env-file /dev/fd/3"*) cat /dev/fd/3 > "$d/env3";; esac
cat > "$d/stdin"
cat "$d/out"
exit $(cat "$d/code")
`
	for name, body := range map[string]string{"docker": script, "out": out, "code": code} {
		mode := os.FileMode(0600)
		if name == "docker" {
			mode = 0700
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
			data, _ := os.ReadFile(filepath.Join(dir, "calls"))
			return strings.Split(strings.TrimSpace(string(data)), "\n")
		}, func() string {
			data, _ := os.ReadFile(filepath.Join(dir, "stdin"))
			return string(data)
		}
}

// fakeRuntimeFD3 is the directory of the last fakeRuntimeStdin, so a test can read what arrived on fd 3.
var fakeRuntimeDir string

func fd3Seen() string {
	data, _ := os.ReadFile(filepath.Join(fakeRuntimeDir, "env3"))
	return string(data)
}

type hardPlanner struct{ unhardened bool }

func (p hardPlanner) Plan(r ContainerRun) (ContainerCommand, error) {
	args := append([]string{"run", "--rm"}, limitFlags(r)...)
	args = append(args, "--user", fixtureUID(), "--network=none", "--env-file", r.EnvFile, "--mount", "type=bind,src="+r.Workspace+",dst=/repo")
	if r.Interactive {
		args = append(args, "--interactive")
	}
	if !p.unhardened {
		args = append(args, "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only")
	}
	args = append(args, r.Image)
	return ContainerCommand{Binary: "docker", Args: append(args, r.Command...)}, nil
}

// limitFlags are the flags the backstop requires on every planned container.
func limitFlags(r ContainerRun) []string {
	return []string{"--name", r.Name, "--label", "nself.ci.attempt=" + r.AttemptID, "--pids-limit", "256", "--memory", "512m", "--cpus", "1"}
}

func (hardPlanner) Cleanup(r ContainerRun) (*ContainerCommand, error) {
	r.Name += "-clean"
	args := append([]string{"run", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only", "--user", fixtureUID(), "--network=none", "--entrypoint", "sh"}, limitFlags(r)...)
	return &ContainerCommand{Binary: "docker", Args: append(args, r.Image, "-c", "true")}, nil
}

func plannedSpec(t *testing.T, id string, p ContainerPlanner) JobSpec {
	return JobSpec{AttemptID: id, CoordinatorID: "coord", Home: t.TempDir(), Plan: p, Command: []string{"go", "test", "-v"},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}}
}

func TestPlannedContainerRunsCommandAndCleanup(t *testing.T) {
	calls := fakeRuntime(t, "ok\n", "0")
	res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), plannedSpec(t, "plan-ok", hardPlanner{}))
	if res.Err != nil || res.Attempt.State != "passed" || res.Reason != "" {
		t.Fatalf("result %+v", res)
	}
	got := calls()
	if len(got) != 3 || !strings.HasPrefix(got[0], "run --name nself-ci-plan-ok") || !strings.HasSuffix(got[0], "alpine:3.19 go test -v") || got[1] != "rm -f nself-ci-plan-ok" || !strings.Contains(got[2], "--entrypoint sh") {
		t.Fatalf("calls %q", got)
	}
}

func TestPlannedContainerGetsEnvOnStdinNeverOnDiskOrArgv(t *testing.T) {
	calls, stdin := fakeRuntimeStdin(t, "ok\n", "0")
	spec := plannedSpec(t, "plan-env", hardPlanner{})
	spec.Job.Env = map[string]string{"FIXTURE_TOKEN": "synthetic-value-1"}
	res := (&Executor{Isolation: BaselineIsolator{}, Env: nil}).Run(context.Background(), spec)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if !strings.Contains(stdin(), "FIXTURE_TOKEN=synthetic-value-1\n") {
		t.Fatalf("env not on stdin: %q", stdin())
	}
	for _, call := range calls() {
		if strings.Contains(call, "synthetic-value-1") || strings.Contains(call, "ci-env-") {
			t.Fatalf("env value or env file on argv: %q", call)
		}
		if strings.Contains(call, "--entrypoint") || strings.HasPrefix(call, "rm -f ") {
			continue
		}
		if !strings.Contains(call, "--env-file /dev/stdin") {
			t.Fatalf("run call without the stdin env file: %q", call)
		}
	}
	files, _ := filepath.Glob(filepath.Join(spec.Home, "ci-env-*"))
	if len(files) != 0 {
		t.Fatalf("env file written: %v", files)
	}
}

func TestPlannedContainerReasons(t *testing.T) {
	for name, tc := range map[string]struct{ out, code, reason string }{
		"killed":      {"", "137", ""},               // no oom event, no cgroup counter: nothing the host saw
		"forkfail":    {"sh: can't fork\n", "2", ""}, // printed by the job, no host evidence: not a limit (D-1580)
		"plainfail":   {"FAIL\n", "1", ""},
		"killedclean": {"", "0", ""},
	} {
		t.Run(name, func(t *testing.T) {
			fakeRuntime(t, tc.out, tc.code)
			res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), plannedSpec(t, "plan-"+name, hardPlanner{}))
			if res.Reason != tc.reason {
				t.Fatalf("reason %q want %q (%+v)", res.Reason, tc.reason, res)
			}
		})
	}
}

func TestPlannedContainerRuntimeFailureIsNotTheJobs(t *testing.T) {
	fakeRuntime(t, "", "125")
	res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), plannedSpec(t, "plan-125", hardPlanner{}))
	var e *Error
	if !errors.As(res.Err, &e) || e.Code != "E613" {
		t.Fatalf("exit 125 must be an E613 runtime failure: %+v", res)
	}
}

func TestPlannedContainerRefusesUnhardenedPlan(t *testing.T) {
	calls := fakeRuntime(t, "", "0")
	res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), plannedSpec(t, "plan-bad", hardPlanner{unhardened: true}))
	var e *Error
	if !asError(res.Err, &e) || e.Code != "E613" {
		t.Fatalf("err %v", res.Err)
	}
	if got := calls(); len(got) != 1 || got[0] != "" {
		t.Fatalf("docker was called: %q", got)
	}
}

func TestPlannedRunTightensWorkspaceParent(t *testing.T) {
	fakeRuntime(t, "", "0")
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "work"), 0755); err != nil { // a pre-existing, too open parent
		t.Fatal(err)
	}
	res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), JobSpec{AttemptID: "plan-parent", CoordinatorID: "coord", Home: home, Plan: hardPlanner{}, Command: []string{"true"},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: "alpine:3.19"}}})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if info, _ := os.Stat(filepath.Join(home, "work")); info.Mode().Perm() != 0700 {
		t.Fatalf("work dir mode %v", info.Mode().Perm())
	}
}

func TestPlannedWorkspaceOpenedToJobUID(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "d")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	file, script := filepath.Join(sub, "f"), filepath.Join(sub, "run.sh")
	for path, mode := range map[string]os.FileMode{file: 0600, script: 0700} {
		if err := os.WriteFile(path, []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := openWorkspaceToJobUID(root); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{root: 0777, sub: 0777, file: 0666, script: 0777} {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != want {
			t.Fatalf("%s mode %v want %v", path, info.Mode().Perm(), want)
		}
	}
}
