//go:build !windows

package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
	script := "#!/bin/sh\nd=$(dirname \"$0\")\necho \"$*\" >> \"$d/calls\"\ncase \"$*\" in *--entrypoint*) exit 0;; esac\ncase \"$1\" in run) ;; *) exit 0;; esac\ncase \"$*\" in *\"--env-file /dev/fd/3\"*) cat /dev/fd/3 > \"$d/env3\";; esac\ncat > \"$d/stdin\"\ncat \"$d/out\"\nexit $(cat \"$d/code\")\n"
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
	res := (&Executor{}).Run(context.Background(), plannedSpec(t, "plan-ok", hardPlanner{}))
	if res.Err != nil || res.Attempt.State != "passed" || res.Reason != "" {
		t.Fatalf("result %+v", res)
	}
	got := calls()
	if len(got) != 2 || !strings.HasPrefix(got[0], "run --rm --name nself-ci-plan-ok") || !strings.HasSuffix(got[0], "alpine:3.19 go test -v") || !strings.Contains(got[1], "--entrypoint sh") {
		t.Fatalf("calls %q", got)
	}
}

func TestPlannedContainerGetsEnvOnStdinNeverOnDiskOrArgv(t *testing.T) {
	calls, stdin := fakeRuntimeStdin(t, "ok\n", "0")
	spec := plannedSpec(t, "plan-env", hardPlanner{})
	spec.Job.Env = map[string]string{"FIXTURE_TOKEN": "synthetic-value-1"}
	res := (&Executor{Env: nil}).Run(context.Background(), spec)
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
		if strings.Contains(call, "--entrypoint") {
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
		"killed":      {"", "137", "resource_limit"},
		"forkfail":    {"sh: can't fork\n", "2", "resource_limit"},
		"plainfail":   {"FAIL\n", "1", ""},
		"killedclean": {"", "0", ""},
	} {
		t.Run(name, func(t *testing.T) {
			fakeRuntime(t, tc.out, tc.code)
			res := (&Executor{}).Run(context.Background(), plannedSpec(t, "plan-"+name, hardPlanner{}))
			if res.Reason != tc.reason {
				t.Fatalf("reason %q want %q (%+v)", res.Reason, tc.reason, res)
			}
		})
	}
}

func TestPlannedContainerRuntimeFailureIsNotTheJobs(t *testing.T) {
	fakeRuntime(t, "", "125")
	res := (&Executor{}).Run(context.Background(), plannedSpec(t, "plan-125", hardPlanner{}))
	var e *Error
	if !errors.As(res.Err, &e) || e.Code != "E613" {
		t.Fatalf("exit 125 must be an E613 runtime failure: %+v", res)
	}
}

func TestPlannedContainerRefusesUnhardenedPlan(t *testing.T) {
	calls := fakeRuntime(t, "", "0")
	res := (&Executor{}).Run(context.Background(), plannedSpec(t, "plan-bad", hardPlanner{unhardened: true}))
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
	res := (&Executor{}).Run(context.Background(), JobSpec{AttemptID: "plan-parent", CoordinatorID: "coord", Home: home, Plan: hardPlanner{}, Command: []string{"true"},
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

func TestCheckContainerCommand(t *testing.T) {
	run := ContainerRun{Image: "alpine:3.19", Workspace: "/w/ws", Name: "nself-ci-a1", AttemptID: "a1"}
	good := append(append([]string{"run"}, limitFlags(run)...), "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only", "--mount", "type=bind,src=/w/ws,dst=/repo", "--user", fixtureUID(), "--network=none", "--runtime", "runsc", "--userns=auto", "alpine:3.19", "go", "test", "-v", "--privileged")

	if err := checkContainerCommand(ContainerCommand{Binary: "docker", Args: good}, run); err != nil {
		t.Fatalf("job words after the image must not matter: %v", err)
	}
	withFlag := func(extra ...string) ContainerCommand {
		return ContainerCommand{Binary: "docker", Args: append(append([]string{}, good[:1]...), append(extra, good[1:]...)...)}
	}
	for name, c := range map[string]ContainerCommand{
		"privileged":  withFlag("--privileged"),
		"pidhost":     withFlag("--pid=host"),
		"ipchost":     withFlag("--ipc", "host"),
		"nethost":     withFlag("--network=host"),
		"netsplit":    withFlag("--network", "host"),
		"capadd":      withFlag("--cap-add", "SYS_ADMIN"),
		"device":      withFlag("--device=/dev/kvm"),
		"volume":      withFlag("-v", "/:/host"),
		"volumeglue":  withFlag("-v/:/host"),
		"volumelong":  withFlag("--volume=/:/host"),
		"socketmount": withFlag("--mount", "type=bind,src=/var/run/docker.sock,dst=/s"),
		"hostpath":    withFlag("--mount", "type=bind,src=/etc,dst=/e"),
		"usernshost":  withFlag("--userns=host"),
		"nocapdrop":   {Binary: "docker", Args: []string{"run", "--security-opt", "no-new-privileges", "--read-only", "alpine:3.19", "x"}},
		"notrun":      {Binary: "docker", Args: []string{"exec", "alpine:3.19"}},
		"binary":      {Binary: "sh", Args: good},
		"noimage":     {Binary: "docker", Args: good[:13]},
		"nouser":      {Binary: "docker", Args: []string{"run", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only", "--network=none", "alpine:3.19", "x"}},
		"nonetwork":   {Binary: "docker", Args: []string{"run", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only", "--user", fixtureUID(), "alpine:3.19", "x"}},
		"envfiledisk": withFlag("--env-file", "/tmp/secrets"),
		"seccomp":     withFlag("--security-opt", "seccomp=unconfined"),
		"apparmor":    withFlag("--security-opt=apparmor=unconfined"),
		"capdrop-net": {Binary: "docker", Args: []string{"run", "--cap-drop=NET_ADMIN", "--security-opt", "no-new-privileges", "--read-only", "alpine:3.19", "x"}},
		"userroot":    withFlag("--user", "0:0"),
		"userword":    withFlag("--user", "root"),
		"useragent":   withFlag("--user", itoa(os.Getuid())+":"+itoa(os.Getuid())),
		"runtimekata": withFlag("--runtime", "kata"),
		"tmpfsother":  withFlag("--tmpfs", "/etc:rw"),
		"envfile":     withFlag("--env-file", "/etc/shadow"),
		"sysctl":      withFlag("--sysctl", "net.ipv4.ip_forward=1"),
		"gpus":        withFlag("--gpus=all"),
		"addhost":     withFlag("--add-host", "x:1.1.1.1"),
		"positional":  withFlag("stray"),
		"valueonbool": withFlag("--rm=true"),
		"net other":   withFlag("--network", "container:x"),
		"mounteq":     withFlag("--mount=type=bind,src=/,dst=/h"),
		"mountsource": withFlag("--mount", "type=bind,src=/w/ws,source=/,dst=/repo"),
		"mountdup":    withFlag("--mount", "type=bind,src=/w/ws,src=/,dst=/repo"),
		"mountupper":  withFlag("--mount", "TYPE=bind,SOURCE=/,src=/w/ws,dst=/repo"),
		"mountdst":    withFlag("--mount", "type=bind,src=/w/ws,dst=/etc"),
		"mountro":     withFlag("--mount", "type=bind,src=/w/ws,dst=/repo,readonly"),
		"roffalse":    withFlag("--read-only=false"),
		"pids0":       withFlag("--pids-limit", "0"),
		"pidsneg":     withFlag("--pids-limit=-1"),
		"mem0":        withFlag("--memory=0m"),
		"memg":        withFlag("--memory=1g"),
		"cpus0":       withFlag("--cpus=0"),
		"nopids":      without(good, "--pids-limit", 2),
		"nomemory":    without(good, "--memory", 2),
		"nocpus":      without(good, "--cpus", 2),
		"nolabel":     without(good, "--label", 2),
		"noname":      without(good, "--name", 2),
		"othername":   withFlag("--name", "someone-else"),
		"lowuid":      withFlag("--user", "1:1"),
		"envsecret":   withFlag("--env", "TOKEN=hunter2"),
	} {
		if err := checkContainerCommand(c, run); err == nil {
			t.Errorf("%s: accepted %v", name, c.Args)
		}
	}
}

// without returns a command whose argv lacks flag and its width-1 value(s).
func without(args []string, flag string, width int) ContainerCommand {
	out := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			i += width - 1
			continue
		}
		out = append(out, args[i])
	}
	return ContainerCommand{Binary: "docker", Args: out}
}

func TestRuntimeQueryAndRemoveRefuseOtherVerbs(t *testing.T) {
	ctx := context.Background()
	if _, err := RuntimeQuery(ctx, "docker", "run", "x"); err == nil {
		t.Fatal("run accepted")
	}
	if _, err := RuntimeQuery(ctx, "sh", "info"); err == nil {
		t.Fatal("sh accepted")
	}
	if err := RuntimeRemove(ctx, "docker", "other-container"); err == nil {
		t.Fatal("non-attempt name accepted")
	}
}

// fixtureUID is a job user that is never the test's own uid.
func fixtureUID() string {
	uid := 65533
	if os.Getuid() == uid || os.Geteuid() == uid {
		uid++
	}
	return itoa(uid) + ":" + itoa(uid)
}

func itoa(n int) string { return strconv.Itoa(n) }

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	*target = e
	return ok
}

func TestPlannedContainerMounts(t *testing.T) {
	ro := Mount{Host: "/cache/a", Target: "/cache", ReadOnly: true}
	run := ContainerRun{Image: "alpine:3.19", Workspace: "/w/ws", Name: "nself-ci-a1", AttemptID: "a1", Mounts: []Mount{ro}}
	args := append(append([]string{"run"}, limitFlags(run)...), "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only",
		"--mount", ro.Spec(), "--mount", "type=bind,src=/w/ws,dst=/repo", "--user", fixtureUID(), "--network=none", "alpine:3.19", "x")
	if err := checkContainerCommand(ContainerCommand{Binary: "docker", Args: args}, run); err != nil {
		t.Fatalf("a declared read-only mount must pass: %v", err)
	}
	run.Mounts = nil
	if err := checkContainerCommand(ContainerCommand{Binary: "docker", Args: args}, run); err == nil {
		t.Fatal("an undeclared mount passed")
	}
	spec := plannedSpec(t, "plan-rw", hardPlanner{})
	spec.Mounts = []Mount{{Host: "/cache/a", Target: "/cache"}}
	res := (&Executor{}).Run(context.Background(), spec)
	var e *Error
	if !errors.As(res.Err, &e) || e.Code != "E611" {
		t.Fatalf("a read-write mount must be refused under the hardened profile: %+v", res)
	}
}

// helperPlanner runs its cleanup in its own image, as the container profile does (D-0963).
type helperPlanner struct {
	hardPlanner
	declared bool // the command carries Image; false hides it so the backstop cannot find the helper image
}

func (p helperPlanner) Cleanup(r ContainerRun) (*ContainerCommand, error) {
	if !r.JobImageCleanup {
		r.Image = "docker.io/library/busybox@sha256:helper"
	}
	cmd, err := p.hardPlanner.Cleanup(r)
	if err == nil && p.declared {
		cmd.Image = r.Image
	}
	return cmd, err
}

func TestPlannedCleanupRunsItsOwnImage(t *testing.T) {
	for name, tc := range map[string]struct {
		declared  bool
		wantNote  bool
		wantClean bool
	}{
		"helper image declared":     {declared: true, wantClean: true},
		"helper image not declared": {declared: false, wantNote: true},
	} {
		t.Run(name, func(t *testing.T) {
			calls := fakeRuntime(t, "ok\n", "0")
			var out strings.Builder
			spec := plannedSpec(t, "plan-helper", helperPlanner{declared: tc.declared})
			spec.Output = func(b []byte) { out.Write(b) }
			res := (&Executor{}).Run(context.Background(), spec)
			if res.Err != nil || res.Attempt.State != "passed" {
				t.Fatalf("result %+v", res)
			}
			got := calls()
			cleaned := len(got) == 2 && strings.Contains(got[1], "docker.io/library/busybox@sha256:helper -c true") && !strings.Contains(got[1], "alpine:3.19")
			if cleaned != tc.wantClean {
				t.Errorf("cleanup ran in the helper image = %v, want %v (calls %q)", cleaned, tc.wantClean, got)
			}
			if note := strings.Contains(out.String(), "cleanup helper did not finish"); note != tc.wantNote {
				t.Errorf("failure note = %v, want %v (output %q)", note, tc.wantNote, out.String())
			}
		})
	}
}

// A node that cannot start the helper image (never pulled, no registry) still cleans the workspace in the job image,
// as it did before the helper existed; a node where both fail reports it in the job log.
func TestPlannedCleanupFallsBackToTheJobImage(t *testing.T) {
	for name, tc := range map[string]struct {
		helperFails, jobFails bool
		wantCalls             int
		wantNote              bool
	}{
		"helper fine":                   {wantCalls: 2},
		"helper missing then job image": {helperFails: true, wantCalls: 3},
		"both fail":                     {helperFails: true, jobFails: true, wantCalls: 3, wantNote: true},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "calls")
			body := "echo \"$*\" >> " + log + "\ncase \"$*\" in\n"
			if tc.helperFails {
				body += "*sha256:helper*) echo 'Unable to find image' >&2; exit 125;;\n"
			}
			if tc.jobFails {
				body += "*--entrypoint*) exit 1;;\n"
			}
			body += "esac\nexit 0\n"
			fakeDocker(t, body)
			var out strings.Builder
			spec := plannedSpec(t, "plan-fallback", helperPlanner{declared: true})
			spec.Output = func(b []byte) { out.Write(b) }
			res := (&Executor{}).Run(context.Background(), spec)
			if res.Err != nil || res.Attempt.State != "passed" {
				t.Fatalf("result %+v", res)
			}
			data, _ := os.ReadFile(log)
			calls := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(calls) != tc.wantCalls {
				t.Fatalf("calls %q, want %d", calls, tc.wantCalls)
			}
			if tc.helperFails && (!strings.Contains(calls[1], "sha256:helper") || !strings.Contains(calls[2], "alpine:3.19") || strings.Contains(calls[2], "sha256:helper")) {
				t.Errorf("second try must run the job image: %q", calls[1:])
			}
			if note := strings.Contains(out.String(), "cleanup helper did not finish"); note != tc.wantNote {
				t.Errorf("note = %v, want %v (output %q)", note, tc.wantNote, out.String())
			}
		})
	}
}

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
