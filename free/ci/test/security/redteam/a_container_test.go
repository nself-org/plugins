//go:build security

package redteam

// Purpose: red team attacks on the container command backstop and the resource limits (TM-05, TM-06,
// TM-07, TM-10). A hostile or buggy profile plans a container command; exec must refuse it before any
// runtime CLI starts.
//   - A07: runtime flag deny list in every spelling (--privileged=true, -v, --mount of the socket, host
//     namespaces, --cap-add, --security-opt overrides, root user, second --user, bad limits);
//   - A08: pids, cpu and memory limits are always emitted and a spec value of zero, negative or NaN cannot
//     remove them;
//   - A09: a sandboxed-container job never starts without --runtime=runsc, however the word is smuggled;
//   - A30, A31: host mounts: the socket and system directories, also behind a symlink or a /proc path.
// Inputs:  exec.Executor.Run with a planner that mutates the real container.Build output; a fake docker on PATH.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: no real docker; the fake CLI records only the argv of `run`.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var aAttemptSeq atomic.Int64

// aAllowAll is an Isolator that leaves the spec as the test built it, so the backstop is what is under attack.
type aAllowAll struct{}

func (aAllowAll) Prepare(context.Context, *exec.JobSpec) error { return nil }

// aMutPlanner builds the genuine hardened command, then lets the attack rewrite its flag region.
type aMutPlanner struct {
	runtime string
	mutate  func([]string) []string
}

func (m aMutPlanner) Plan(r exec.ContainerRun) (exec.ContainerCommand, error) {
	cmd, err := container.Build(container.Params{Run: r, Binary: "docker", ForbiddenUIDs: []int{os.Getuid(), os.Geteuid()},
		Network: isolation.NetworkPlan{Network: "none"}, Runtime: m.runtime})
	if err != nil {
		return cmd, err
	}
	if m.mutate != nil {
		cmd.Args = m.mutate(cmd.Args)
	}
	return cmd, nil
}
func (m aMutPlanner) Cleanup(exec.ContainerRun) (*exec.ContainerCommand, error) { return nil, nil }

// aWithFlags inserts extra right after "run".
func aWithFlags(extra ...string) func([]string) []string {
	return func(a []string) []string { return append(append([]string{a[0]}, extra...), a[1:]...) }
}

type aRunOpts struct {
	level   model.Isolation
	plan    exec.ContainerPlanner
	cpu     float64
	mem     int
	command []string
	mounts  []exec.Mount
}

func aRunJob(t *testing.T, o aRunOpts) (exec.AttemptResult, [][]string) {
	t.Helper()
	runs := aFakeDocker(t)
	if o.command == nil {
		o.command = []string{"true"}
	}
	id := fmt.Sprintf("rt%d", aAttemptSeq.Add(1))
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	res := (&exec.Executor{Grace: time.Second, Isolation: aAllowAll{}}).Run(ctx, exec.JobSpec{AttemptID: id, CoordinatorID: "c1", Home: t.TempDir(),
		Job:     model.Job{Isolation: o.level, Container: &model.ContainerConfig{Image: "alpine"}},
		Command: o.command, Plan: o.plan, CPU: o.cpu, MemoryMB: o.mem, Mounts: o.mounts, Network: "none"})
	return res, runs()
}

func aFlagValue(args []string, name string) (string, bool) {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1], true
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"="), true
		}
	}
	return "", false
}

// A07: the flag allowlist refuses every hostile spelling; the unmodified command is accepted (control).
func TestA07_RuntimeFlagDenyList(t *testing.T) {
	if res, runs := aRunJob(t, aRunOpts{level: "container", plan: aMutPlanner{}}); res.Err != nil || len(runs) != 1 {
		t.Fatalf("control: the genuine hardened command must run once: err=%v runs=%d", res.Err, len(runs))
	}
	hostile := [][]string{
		{"--privileged"}, {"--privileged=true"}, {"--privileged=false"}, {"--Privileged"}, {"--privileged "}, {"--priv​ileged"},
		{"--pid=host"}, {"--pid", "host"}, {"--ipc=host"}, {"--uts=host"}, {"--cgroupns=host"}, {"--userns=host"}, {"--network=host"},
		{"--net=host"}, {"--network", "container:other"}, {"--cap-add=SYS_ADMIN"}, {"--cap-add", "ALL"}, {"--cap-drop=NET_RAW"},
		{"-v", "/var/run/docker.sock:/var/run/docker.sock"}, {"-v=/:/h"}, {"--volume=/:/h"}, {"--volumes-from=other"},
		{"--mount", "type=bind,src=/var/run/docker.sock,dst=/s"}, {"--mount=type=bind,src=/,dst=/h"},
		{"--mount", "type=bind,source=/etc,target=/repo"}, {"--mount", "type=bind,src=/etc,dst=/repo"},
		{"--security-opt", "seccomp=unconfined"}, {"--security-opt=apparmor=unconfined"}, {"--security-opt", "label=disable"},
		{"--security-opt", "no-new-privileges=false"}, {"--security-opt", "systempaths=unconfined"},
		{"--device=/dev/mem"}, {"--device-cgroup-rule=a *:* rwm"}, {"--user=0:0"}, {"--user", "root"}, {"--user", "0"},
		{"--user=65532:0"}, {"--user=1:1"}, {"--user=" + strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getuid())},
		{"--sysctl", "net.ipv4.ip_forward=1"}, {"--ulimit", "nproc=-1"}, {"--oom-kill-disable"}, {"--init=false"},
		{"--pids-limit=-1"}, {"--pids-limit=0"}, {"--pids-limit=99999999"}, {"--memory=0"}, {"--memory=0m"}, {"--memory=-1m"},
		{"--memory=1234567890m"}, {"--memory-swap=-1"}, {"--memory-swap=0m"}, {"--cpus=0"}, {"--cpus=-1"}, {"--cpus=+Inf"}, {"--cpus=1e9"},
		{"--runtime=runc"}, {"--runtime", "kata"}, {"--env=AWS_SECRET=x"}, {"--env", "LD_PRELOAD=/x"}, {"--env-file=/etc/passwd"},
		{"--env-file", "/proc/self/environ"}, {"--tmpfs=/etc"}, {"--tmpfs", "/proc"}, {"--name=other"}, {"--read-only=false"},
		{"--read-only", "false"}, {"--rm=false"}, {"--interactive"}, {"--restart=always"}, {"--detach"}, {"-d"}, {"-it"},
		{"--add-host=a:10.0.0.1"}, {"--dns-search=evil"}, {"--link=a:b"}, {"--publish=22:22"}, {"-p", "22:22"}, {"--expose=22"},
		{"--ip=10.0.0.9"}, {"--mac-address=00:00:00:00:00:01"}, {"--hostname=x"}, {"--cgroup-parent=/"}, {"--storage-opt=size=1g"},
	}
	for _, extra := range hostile {
		res, runs := aRunJob(t, aRunOpts{level: "container", plan: aMutPlanner{mutate: aWithFlags(extra...)}})
		var ce *exec.Error
		if len(runs) != 0 || res.Err == nil || !errors.As(res.Err, &ce) || ce.Code != "E613" {
			t.Errorf("hostile flags %q: docker run started=%d err=%v", extra, len(runs), res.Err)
		}
	}
	// the same flags after the image are the job's own command words, never docker flags
	res, runs := aRunJob(t, aRunOpts{level: "container", plan: aMutPlanner{}, command: []string{"--privileged", "-v", "/:/h"}})
	if res.Err != nil || len(runs) != 1 {
		t.Fatalf("job words after the image: err=%v runs=%d", res.Err, len(runs))
	}
	args := runs[0]
	if idx := aIndexOf(args, "alpine"); idx < 0 || strings.Join(args[idx+1:], " ") != "--privileged -v /:/h" {
		t.Errorf("job command words are not exactly the words after the image: %q", args)
	}
}

func aIndexOf(a []string, s string) int {
	for i, v := range a {
		if v == s {
			return i
		}
	}
	return -1
}

// A08: limits are always on the command, whatever the spec says.
func TestA08_LimitsAlwaysEmitted(t *testing.T) {
	cases := []struct {
		name string
		cpu  float64
		mem  int
	}{{"zero", 0, 0}, {"negative", -3, -9}, {"neg-inf", math.Inf(-1), -1}, {"pos-inf", math.Inf(1), 1 << 30},
		{"huge", 1e12, math.MaxInt32}, {"tiny", 1e-300, 1}}
	for _, c := range cases {
		res, runs := aRunJob(t, aRunOpts{level: "container", plan: aMutPlanner{}, cpu: c.cpu, mem: c.mem})
		if len(runs) == 0 {
			continue // refused: fail closed is fine
		}
		if res.Err != nil {
			t.Errorf("%s: err %v", c.name, res.Err)
		}
		args := runs[0]
		cpus, _ := aFlagValue(args, "--cpus")
		f, err := strconv.ParseFloat(cpus, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 || f > 64 {
			t.Errorf("%s: --cpus=%q is not a finite sane limit", c.name, cpus)
		}
		mem, _ := aFlagValue(args, "--memory")
		mb, err := strconv.Atoi(strings.TrimSuffix(mem, "m"))
		if err != nil || mb <= 0 || mb > 1<<20 {
			t.Errorf("%s: --memory=%q is not a sane limit (more than 1 TiB or missing)", c.name, mem)
		}
		if swap, _ := aFlagValue(args, "--memory-swap"); swap != mem {
			t.Errorf("%s: swap %q differs from memory %q (swap would lift the memory cap)", c.name, swap, mem)
		}
		if p, _ := aFlagValue(args, "--pids-limit"); p == "" || p == "0" || strings.HasPrefix(p, "-") {
			t.Errorf("%s: pids-limit %q", c.name, p)
		}
	}
	// the profile itself, called directly with zero values: the backstop is the only guard
	for _, bad := range []container.Params{{PidsLimit: -5}, {PidsLimit: 0}} {
		bad.Run = exec.ContainerRun{Name: "n", Image: "alpine", Workspace: "/x/ws", Command: []string{"true"}, MemoryMB: 64, CPU: 1}
		bad.Binary = "docker"
		bad.Network = isolation.NetworkPlan{Network: "none"}
		cmd, err := container.Build(bad)
		if err != nil {
			continue
		}
		if p, _ := aFlagValue(cmd.Args, "--pids-limit"); p == "" || strings.HasPrefix(p, "-") || p == "0" {
			t.Errorf("Build(PidsLimit=%d) emitted --pids-limit=%q", bad.PidsLimit, p)
		}
	}
}

// A09: --runtime=runsc must be a real runtime flag of the flag region, not a label value, not a job word.
func TestA09_SandboxNeedsRunsc(t *testing.T) {
	good, runs := aRunJob(t, aRunOpts{level: "sandboxed-container", plan: aMutPlanner{runtime: "runsc"}})
	if good.Err != nil || len(runs) != 1 {
		t.Fatalf("control: runsc plan must run: err=%v runs=%d", good.Err, len(runs))
	}
	attacks := map[string]aRunOpts{
		"no runtime flag":      {plan: aMutPlanner{}},
		"label value":          {plan: aMutPlanner{mutate: aWithFlags("--label", "--runtime=runsc")}},
		"job command word":     {plan: aMutPlanner{}, command: []string{"--runtime=runsc"}},
		"job words before img": {plan: aMutPlanner{}, command: []string{"x", "--runtime", "runsc"}},
		"runsc then runc":      {plan: aMutPlanner{runtime: "runsc", mutate: aWithFlags("--runtime=runc")}},
		"runc then runsc":      {plan: aMutPlanner{mutate: aWithFlags("--runtime=runc", "--runtime=runsc")}},
		"runtime with space":   {plan: aMutPlanner{mutate: aWithFlags("--runtime=runsc ")}},
		"upper case":           {plan: aMutPlanner{mutate: aWithFlags("--runtime=RUNSC")}},
		"entrypoint value":     {plan: aMutPlanner{mutate: aWithFlags("--entrypoint", "--runtime=runsc")}},
	}
	for name, o := range attacks {
		o.level = "sandboxed-container"
		res, runs := aRunJob(t, o)
		if len(runs) != 0 || res.Err == nil {
			t.Errorf("%s: sandboxed job started without a real runsc runtime (runs=%d err=%v)", name, len(runs), res.Err)
		}
	}
}

// A30/A31: host mounts. The lexical deny list holds; anything it does not name by string is a finding.
func TestA30_MountsLexical(t *testing.T) {
	for _, host := range []string{"/var/run/docker.sock", "/run/docker.sock", "/var/run", "/run", "/var/run/user/1000/podman/podman.sock",
		"/proc", "/etc", "/", "/home", "/Users", "/root", "/tmp/x/docker.sock", "/var/run/docker", "/run/containerd/containerd.sock"} {
		res, runs := aRunJob(t, aRunOpts{level: "container", mounts: []exec.Mount{{Host: host, Target: "/cache", ReadOnly: true}}})
		if len(runs) != 0 || res.Err == nil {
			t.Errorf("mount of %q reached docker (runs=%d err=%v)", host, len(runs), res.Err)
		}
	}
}

func TestA31_MountsBehindALinkOrProcPath(t *testing.T) {
	if !openFinding(t, "A03") {
		return
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "real.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "cache")
	if err := os.Symlink(sock, link); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{link, "/proc/1/root", "/proc/self/root", "/etc/ssh", "/root/.ssh", "/var/lib/docker", "/private/var/run"} {
		res, runs := aRunJob(t, aRunOpts{level: "container", mounts: []exec.Mount{{Host: host, Target: "/cache", ReadOnly: true}}})
		if len(runs) != 0 || res.Err == nil {
			t.Errorf("mount of %q reached docker (runs=%d err=%v)", host, len(runs), res.Err)
		}
	}
}

// A08b: a NaN cpu count slips through "f <= 0" in the backstop and reaches the runtime CLI as --cpus=NaN. The
// runtime rejects it today, so the job does not run unlimited; the backstop is still not the fail-closed
// filter its header promises.
func TestA08b_NaNCPUReachesTheRuntime(t *testing.T) {
	if !openFinding(t, "A02") {
		return
	}
	res, runs := aRunJob(t, aRunOpts{level: "container", plan: aMutPlanner{}, cpu: math.NaN()})
	if len(runs) != 0 {
		v, _ := aFlagValue(runs[0], "--cpus")
		t.Errorf("a NaN cpu spec started docker with --cpus=%s (err=%v)", v, res.Err)
	}
}
