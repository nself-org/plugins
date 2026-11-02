//go:build !windows

package exec

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
)

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
	spec.Mounts = []Mount{{Host: t.TempDir(), Target: "/cache"}}
	res := (&Executor{Isolation: BaselineIsolator{}}).Run(context.Background(), spec)
	var e *Error
	if !errors.As(res.Err, &e) || e.Code != "E611" {
		t.Fatalf("a read-write mount must be refused under the hardened profile: %+v", res)
	}
}
