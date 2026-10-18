package exec

// Purpose: run a container whose command line an isolation profile built.
// Inputs:  a JobSpec whose Plan an Isolator set (P7-TRUST-04), the attempt workspace and the job environment.
// Outputs: the exit code, a failure reason and an error; the container is always stopped and removed.
// Constraints: exec stays the only package that starts the runtime CLI. The profile only
// returns an argv; checkContainerCommand refuses any argv that is not hardened, whatever
// the profile says, so a profile bug cannot start a privileged container.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// ContainerRun is what exec knows when it starts a planned container.
type ContainerRun struct {
	Name, Image, Workspace, Workdir, EnvFile string
	Command                                  []string
	AttemptID, CoordinatorID                 string
	Slot                                     int
	CPU                                      float64
	MemoryMB                                 int
	Network                                  model.NetworkScope
}

// ContainerCommand is one runtime CLI invocation: Binary is docker or podman and
// Args follow it.
type ContainerCommand struct {
	Binary string
	Args   []string
}

// ContainerPlanner builds the container command lines; it never starts anything.
type ContainerPlanner interface {
	// Plan returns the `run` command for the job container.
	Plan(ContainerRun) (ContainerCommand, error)
	// Cleanup returns a command run after the job container exited (it returns nil
	// when none is needed), so files the job UID created can be removed.
	Cleanup(ContainerRun) (*ContainerCommand, error)
}

var (
	runtimeBinary = regexp.MustCompile(`^(docker|podman)$`)
	// resourceSignatures are what a job prints when the kernel refuses a fork.
	resourceSignatures = []string{"Resource temporarily unavailable", "can't fork", "Cannot fork", "pthread_create failed"}
	jobUser            = regexp.MustCompile(`^([1-9][0-9]*):([1-9][0-9]*)$`)
	attemptNetwork     = regexp.MustCompile(`^nself-ci-[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	memoryValue        = regexp.MustCompile(`^[1-9][0-9]{0,8}m$`)
)

// plannedFlags is the allowlist of flags a planned container may carry and whether each takes a
// separate value. Anything else (including every short flag) is refused, so a profile cannot
// add --privileged, a host namespace, --cap-add, --device or a volume by accident.
var plannedFlags = map[string]bool{
	"--rm": false, "--read-only": false, "--name": true, "--label": true, "--network": true, "--dns": true,
	"--cap-drop": true, "--security-opt": true, "--tmpfs": true, "--pids-limit": true, "--cpus": true,
	"--memory": true, "--memory-swap": true, "--user": true, "--env": true, "--runtime": true, "--userns": true,
	"--entrypoint": true, "--mount": true, "--workdir": true, "--env-file": true,
}

// checkContainerCommand refuses a planned command that is not a hardened `run` of run.Image.
// Only the flag region (before the image) is checked: the job command may carry any word.
func checkContainerCommand(c ContainerCommand, run ContainerRun) error {
	bad := func(why string) error { return coded("E613", "planned container refused: "+why) }
	if !runtimeBinary.MatchString(c.Binary) {
		return bad("unknown runtime " + c.Binary)
	}
	if len(c.Args) == 0 || c.Args[0] != "run" {
		return bad("not a run command")
	}
	seen := map[string]bool{}
	i := 1
	for ; i < len(c.Args) && c.Args[i] != run.Image; i++ {
		name, value, hasValue := strings.Cut(c.Args[i], "=")
		takesValue, known := plannedFlags[name]
		if !strings.HasPrefix(name, "--") || !known {
			return bad("flag not allowed: " + c.Args[i])
		}
		if takesValue && !hasValue {
			if i+1 >= len(c.Args) {
				return bad("flag without value: " + name)
			}
			i++
			value = c.Args[i]
		} else if !takesValue && hasValue {
			return bad("flag takes no value: " + name)
		}
		if err := checkFlagValue(name, value, run); err != nil {
			return err
		}
		seen[name+"="+value] = true
	}
	if i >= len(c.Args) {
		return bad("image not on the command line")
	}
	for _, want := range []string{"--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only="} {
		if !seen[want] {
			return bad("missing " + strings.TrimSuffix(want, "="))
		}
	}
	return nil
}

// checkFlagValue constrains the flags whose value decides the isolation.
func checkFlagValue(name, value string, run ContainerRun) error {
	bad := func(why string) error { return coded("E613", "planned container refused: "+why) }
	switch name {
	case "--cap-drop":
		if value != "ALL" {
			return bad("cap-drop must be ALL")
		}
	case "--security-opt":
		if value != "no-new-privileges" && value != "no-new-privileges=true" {
			return bad("security-opt " + value)
		}
	case "--network":
		if value != "none" && value != "bridge" && !attemptNetwork.MatchString(value) {
			return bad("network " + value)
		}
	case "--user":
		m := jobUser.FindStringSubmatch(value)
		if m == nil || m[1] != m[2] {
			return bad("user must be a numeric uid:uid, not root")
		}
		if uid := strconv.Itoa(os.Getuid()); m[1] == uid || m[1] == strconv.Itoa(os.Geteuid()) {
			return bad("job user equals the agent user")
		}
	case "--runtime":
		if value != "runsc" {
			return bad("runtime " + value)
		}
	case "--userns":
		if value != "auto" {
			return bad("userns " + value)
		}
	case "--mount":
		return checkMount(value, run.Workspace)
	case "--tmpfs":
		if !strings.HasPrefix(value, "/tmp:") && value != "/tmp" {
			return bad("tmpfs outside /tmp")
		}
	case "--env-file":
		if value != run.EnvFile {
			return bad("env file is not the attempt's")
		}
	case "--pids-limit":
		if n, err := strconv.Atoi(value); err != nil || n < 1 || n > 1<<20 {
			return bad("pids-limit must be a positive number")
		}
	case "--memory", "--memory-swap":
		if !memoryValue.MatchString(value) {
			return bad(name + " must be a positive size in megabytes")
		}
	case "--cpus":
		if f, err := strconv.ParseFloat(value, 64); err != nil || f <= 0 || f > 1024 {
			return bad("cpus must be positive")
		}
	}
	return nil
}

// checkMount allows exactly one bind of the attempt workspace onto /repo: the keys are
// type, src and dst (no alias such as source or target, no duplicate, nothing else).
func checkMount(spec, workspace string) error {
	bad := func(why string) error { return coded("E613", "planned container refused: "+why) }
	fields := map[string]string{}
	for _, part := range strings.Split(spec, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return bad("mount option without a value: " + part)
		}
		if _, dup := fields[k]; dup {
			return bad("duplicate mount key " + k)
		}
		fields[k] = v
	}
	if len(fields) != 3 || fields["type"] != "bind" || fields["src"] != workspace || fields["dst"] != "/repo" {
		return bad("mount is not the job root bound at /repo")
	}
	return nil
}

func (e *Executor) runPlanned(ctx context.Context, s JobSpec, workspace string, env []string) (int, string, error) {
	if s.Job.Container == nil || !imageRef.MatchString(s.Job.Container.Image) || strings.Contains(s.Job.Container.Image, "..") {
		return -1, "", coded("E613", "invalid container image")
	}
	workdir, err := containerWorkdir(s)
	if err != nil {
		return -1, "", err
	}
	// the workspace is opened to the job UID, so its parent must keep every other host user out
	if err := os.Chmod(filepath.Dir(workspace), 0700); err != nil {
		return -1, "", coded("E612", "workspace parent is not private: "+err.Error())
	}
	if err := openWorkspaceToJobUID(workspace); err != nil {
		return -1, "", coded("E612", "workspace preparation failed: "+err.Error())
	}
	envFile, removeEnv, err := writeEnvFile(s.Home, env)
	if err != nil {
		return -1, "", err
	}
	defer removeEnv()
	run := ContainerRun{Name: "nself-ci-" + s.AttemptID, Image: s.Job.Container.Image, Workspace: workspace, Workdir: workdir, EnvFile: envFile,
		Command: s.Command, AttemptID: s.AttemptID, CoordinatorID: s.CoordinatorID, Slot: s.Slot, CPU: s.CPU, MemoryMB: s.MemoryMB, Network: s.Network}
	if run.CPU <= 0 {
		run.CPU = 1.5
	}
	if run.MemoryMB <= 0 {
		run.MemoryMB = 1024
	}
	cmd, err := s.Plan.Plan(run)
	if err != nil {
		return -1, "", err
	}
	if err := checkContainerCommand(cmd, run); err != nil {
		return -1, "", err
	}
	stop := containerStopper(cmd.Binary, run.Name, e.Grace)
	tail := &outputTail{max: 4096}
	sink := func(b []byte) {
		tail.write(b)
		if s.Output != nil {
			s.Output(b)
		}
	}
	code, err := e.runCommand(ctx, append([]string{cmd.Binary}, cmd.Args...), workspace, dockerHostEnv(), sink, stop, nil, "")
	if err != nil {
		stop()
		err = coded("E613", "container run: "+err.Error())
	}
	e.cleanupPlanned(s, run)
	return code, resourceReason(ctx, code, tail.String()), err
}

// cleanupPlanned runs the profile's cleanup command, best effort and time boxed.
func (e *Executor) cleanupPlanned(s JobSpec, run ContainerRun) {
	cmd, err := s.Plan.Cleanup(run)
	if err != nil || cmd == nil || checkContainerCommand(*cmd, run) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 30*time.Second)
	defer cancel()
	local := Executor{Redactor: passThroughRedactor{}, Grace: e.Grace}
	_, _ = local.runCommand(ctx, append([]string{cmd.Binary}, cmd.Args...), run.Workspace, dockerHostEnv(), nil, nil, nil, "")
}

// resourceReason reports "resource_limit" for a SIGKILLed container (probably the
// memory limit: an operator kill or a sweep looks the same from outside) or output
// showing a refused fork (the pids limit); a deadline or cancel is never a resource
// reason by itself.
func resourceReason(ctx context.Context, code int, tail string) string {
	if code == 0 {
		return ""
	}
	if code == 137 && ctx.Err() == nil {
		return "resource_limit"
	}
	for _, sig := range resourceSignatures {
		if strings.Contains(tail, sig) {
			return "resource_limit"
		}
	}
	return ""
}

// outputTail keeps the last max bytes of output.
type outputTail struct {
	max int
	buf []byte
}

func (t *outputTail) write(b []byte) {
	t.buf = append(t.buf, b...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
}

func (t *outputTail) String() string { return string(t.buf) }

// openWorkspaceToJobUID lets the job UID (never the agent's) read and write the
// attempt workspace: directories 0777, files group/other readable and writable.
// The parent of every workspace is 0700, so no other host user reaches it.
func openWorkspaceToJobUID(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			return os.Chmod(path, 0777)
		case info.Mode().IsRegular():
			perm := info.Mode().Perm() | 0066
			if perm&0100 != 0 {
				perm |= 0011
			}
			return os.Chmod(path, perm)
		}
		return nil
	})
}
