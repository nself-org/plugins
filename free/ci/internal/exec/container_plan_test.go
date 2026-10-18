//go:build !windows

package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// fakeRuntime puts a `docker` script first on PATH that records its calls and
// replays the configured output and exit code.
func fakeRuntime(t *testing.T, out string, code string) (calls func() []string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nd=$(dirname \"$0\")\necho \"$*\" >> \"$d/calls\"\ncase \"$*\" in *--entrypoint*) exit 0;; esac\ncat \"$d/out\"\nexit $(cat \"$d/code\")\n"
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
	}
}

type hardPlanner struct{ unhardened bool }

func (p hardPlanner) Plan(r ContainerRun) (ContainerCommand, error) {
	args := []string{"run", "--rm", "--name", r.Name, "--mount", "type=bind,src=" + r.Workspace + ",dst=/repo"}
	if !p.unhardened {
		args = append(args, "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only")
	}
	args = append(args, r.Image)
	return ContainerCommand{Binary: "docker", Args: append(args, r.Command...)}, nil
}

func (hardPlanner) Cleanup(r ContainerRun) (*ContainerCommand, error) {
	return &ContainerCommand{Binary: "docker", Args: []string{"run", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only", "--entrypoint", "sh", r.Image, "-c", "true"}}, nil
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
	run := ContainerRun{Image: "alpine:3.19", Workspace: "/w/ws"}
	good := []string{"run", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only", "--mount", "type=bind,src=/w/ws,dst=/repo", "alpine:3.19", "go", "test", "-v", "--privileged"}
	if err := checkContainerCommand(ContainerCommand{"docker", good}, run); err != nil {
		t.Fatalf("job words after the image must not matter: %v", err)
	}
	withFlag := func(extra ...string) ContainerCommand {
		return ContainerCommand{"docker", append(append([]string{}, good[:1]...), append(extra, good[1:]...)...)}
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
		"nocapdrop":   {"docker", []string{"run", "--security-opt", "no-new-privileges", "--read-only", "alpine:3.19", "x"}},
		"notrun":      {"docker", []string{"exec", "alpine:3.19"}},
		"binary":      {"sh", good},
		"noimage":     {"docker", good[:7]},
	} {
		if err := checkContainerCommand(c, run); err == nil {
			t.Errorf("%s: accepted %v", name, c.Args)
		}
	}
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

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	*target = e
	return ok
}
