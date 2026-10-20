package container

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

func sampleParams() Params {
	return Params{
		Binary: "docker", ForbiddenUIDs: []int{501},
		Network: isolation.NetworkPlan{Network: "none"},
		Run: exec.ContainerRun{Name: "nself-ci-a1", Image: "alpine@sha256:" + strings.Repeat("a", 64), Workspace: "/home/agent/.nself/ci/work/a1",
			Workdir: "/repo/sub", EnvFile: "/home/agent/.nself/ci/ci-env-1", Command: []string{"go", "test", "./..."},
			AttemptID: "a1", CoordinatorID: "c1", Slot: 2, CPU: 1.5, MemoryMB: 512},
	}
}

func flagRegion(c exec.ContainerCommand, image string) []string {
	for i, a := range c.Args {
		if a == image {
			return c.Args[:i]
		}
	}
	return c.Args
}

func has(args []string, want ...string) bool {
	for i := range args {
		if i+len(want) <= len(args) && strings.Join(args[i:i+len(want)], "\x00") == strings.Join(want, "\x00") {
			return true
		}
	}
	return false
}

func TestBuildGolden(t *testing.T) {
	p := sampleParams()
	c, err := Build(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"run", "--rm", "--name", "nself-ci-a1",
		"--label", "nself.ci.attempt=a1", "--label", "role=ci-ephemeral", "--label", "coordinator_id=c1", "--label", "nself.ci.coordinator=c1",
		"--network=none", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only",
		"--tmpfs", "/tmp:rw,nosuid,size=256m", "--pids-limit", "512", "--cpus=1.5", "--memory=512m", "--memory-swap=512m",
		"--user", "65534:65534", "--env", "HOME=/tmp", "--env", "TMPDIR=/tmp",
		"--mount", "type=bind,src=/home/agent/.nself/ci/work/a1,dst=/repo", "--workdir", "/repo/sub", "--env-file", "/home/agent/.nself/ci/ci-env-1",
		p.Run.Image, "go", "test", "./..."}
	if c.Binary != "docker" || strings.Join(c.Args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("argv\n got %q\nwant %q", c.Args, want)
	}
}

// TestBuildNeverEmitsForbiddenFlags walks a product of inputs and checks the invariants of Epic D8.
func TestBuildNeverEmitsForbiddenFlags(t *testing.T) {
	for _, binary := range []string{"docker", "podman"} {
		for _, net := range []isolation.NetworkPlan{{Network: "none"}, {Network: "bridge"}, {Network: "none", DNS: []string{"127.0.0.1"}}} {
			for slot := 0; slot < 5; slot++ {
				for _, runtime := range []string{"", "runsc"} {
					p := sampleParams()
					p.Binary, p.Network, p.Run.Slot, p.Runtime = binary, net, slot, runtime
					c, err := Build(p)
					if err != nil {
						t.Fatal(err)
					}
					flags := flagRegion(c, p.Run.Image)
					for _, bad := range []string{"--privileged", "--network=host", "--pid=host", "--ipc=host", "--userns=host", "--cap-add"} {
						if has(flags, bad) {
							t.Errorf("%s emitted", bad)
						}
					}
					joined := strings.Join(flags, " ")
					for _, bad := range []string{"docker.sock", "podman.sock", "/var/run", "--volume", " -v "} {
						if strings.Contains(joined, bad) {
							t.Errorf("%q in %s", bad, joined)
						}
					}
					uid, err := JobUID(slot, p.ForbiddenUIDs)
					if err != nil {
						t.Fatal(err)
					}
					for _, must := range [][]string{{"--cap-drop=ALL"}, {"--security-opt", "no-new-privileges"}, {"--read-only"}, {"--tmpfs", "/tmp:rw,nosuid,size=256m"},
						{"--pids-limit", "512"}, {"--label", "role=ci-ephemeral"}, {"--label", "coordinator_id=c1"}, {"--label", "nself.ci.attempt=a1"},
						{"--user", itoa(uid) + ":" + itoa(uid)}} {
						if !has(flags, must...) {
							t.Errorf("missing %v in %s", must, joined)
						}
					}
					if uid == 501 || uid == 0 {
						t.Errorf("job uid %d is the agent or root", uid)
					}
					mounts := 0
					for i, a := range flags {
						if a == "--mount" {
							mounts++
							if !strings.HasPrefix(flags[i+1], "type=bind,src="+p.Run.Workspace+",dst=/repo") {
								t.Errorf("mount %s", flags[i+1])
							}
						}
					}
					if mounts != 1 {
						t.Errorf("%d mounts", mounts)
					}
				}
			}
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for ; n > 0; n /= 10 {
		d = append([]byte{byte('0' + n%10)}, d...)
	}
	return string(d)
}

func mustUID(t *testing.T, slot int, forbidden []int) int {
	t.Helper()
	uid, err := JobUID(slot, forbidden)
	if err != nil {
		t.Fatal(err)
	}
	return uid
}

func TestJobUIDSkipsForbiddenAndNeverRepeats(t *testing.T) {
	if got := mustUID(t, 0, nil); got != BaseUID {
		t.Fatalf("slot 0 = %d", got)
	}
	if got := mustUID(t, 1, []int{BaseUID}); got != BaseUID+2 {
		t.Fatalf("agent uid in range: slot 1 = %d", got)
	}
	if _, err := JobUID(-1, nil); err == nil {
		t.Fatal("negative slot accepted")
	}
	forbidden := []int{BaseUID + 3, BaseUID, 0}
	seen := map[int]int{}
	for slot := 0; slot < 500; slot++ {
		uid := mustUID(t, slot, forbidden)
		for _, f := range forbidden {
			if uid == f {
				t.Fatalf("slot %d got forbidden %d", slot, uid)
			}
		}
		if prev, dup := seen[uid]; dup {
			t.Fatalf("slots %d and %d share uid %d", prev, slot, uid)
		}
		seen[uid] = slot
	}
}

func TestBuildRejectsUnsafeInput(t *testing.T) {
	for name, mut := range map[string]func(*Params){
		"binary":    func(p *Params) { p.Binary = "sh" },
		"slot":      func(p *Params) { p.Run.Slot = -1 },
		"command":   func(p *Params) { p.Run.Command = nil },
		"relative":  func(p *Params) { p.Run.Workspace = "ws" },
		"comma":     func(p *Params) { p.Run.Workspace = "/a,b" },
		"socket":    func(p *Params) { p.Run.Workspace = "/run/docker.sock" },
		"nameempty": func(p *Params) { p.Run.Name = "" },
	} {
		p := sampleParams()
		mut(&p)
		if _, err := Build(p); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestSlotUIDCeilingOnRootless(t *testing.T) {
	if _, err := SlotUID(3, nil, MaxRootlessUID); err != nil {
		t.Fatalf("slot 3 = 65535 must map: %v", err)
	}
	_, err := SlotUID(4, nil, MaxRootlessUID)
	var e *isolation.Error
	if !errors.As(err, &e) || e.Reason != isolation.ReasonSlotUID {
		t.Fatalf("slot 4 on rootless: %v", err)
	}
	if _, err := SlotUID(4, nil, 0); err != nil {
		t.Fatalf("no ceiling on rootful: %v", err)
	}
}

func TestBuildOptionalFlags(t *testing.T) {
	p := sampleParams()
	p.Runtime, p.UsernsAuto, p.Entrypoint, p.PidsLimit, p.Run.EnvFile = "runsc", true, "sh", 64, ""
	c, _ := Build(p)
	for _, want := range [][]string{{"--runtime", "runsc"}, {"--userns=auto"}, {"--entrypoint", "sh"}, {"--pids-limit", "64"}} {
		if !has(c.Args, want...) {
			t.Errorf("missing %v", want)
		}
	}
	if has(c.Args, "--env-file") {
		t.Error("env file emitted when empty")
	}
}

// TestBuildAddsInteractiveOnlyForAJobStdin: a runner credential on the job's stdin needs the run to be
// interactive; every other run stays without the flag (D-1001 for planned containers).
func TestBuildAddsInteractiveOnlyForAJobStdin(t *testing.T) {
	p := sampleParams()
	cmd, err := Build(p)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(cmd.Args, "--interactive") {
		t.Fatalf("no job stdin but interactive: %v", cmd.Args)
	}
	p.Run.Interactive, p.Run.EnvFile = true, "/dev/fd/3"
	if cmd, err = Build(p); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cmd.Args, "--interactive") || !slices.Contains(cmd.Args, "/dev/fd/3") {
		t.Fatalf("job stdin but not interactive or no fd 3 env file: %v", cmd.Args)
	}
}

// TestBuildAddsTheRunnerRootOnlyWhenAsked: the exec-capable tmpfs is one more --tmpfs, only for a run that
// asked for it, and /tmp stays as it was (noexec by default).
func TestBuildAddsTheRunnerRootOnlyWhenAsked(t *testing.T) {
	p := sampleParams()
	cmd, err := Build(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(cmd.Args, " "), "/runner") {
		t.Fatalf("no runner root asked but present: %v", cmd.Args)
	}
	p.Run.RunnerRoot = exec.RunnerRootPath
	if cmd, err = Build(p); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--tmpfs "+exec.RunnerRootTmpfs()) || strings.Count(joined, "--tmpfs") != 2 || !strings.Contains(joined, "--tmpfs /tmp:rw,nosuid,size=256m") {
		t.Fatalf("runner root tmpfs missing or /tmp changed: %v", cmd.Args)
	}
}
