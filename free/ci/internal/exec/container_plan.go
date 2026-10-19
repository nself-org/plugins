package exec

// Purpose: run a container whose command line an isolation profile built.
// Inputs:  a JobSpec whose Plan an Isolator set (P7-TRUST-04), the attempt workspace and the job environment.
// Outputs: the exit code, a failure reason and an error; the container is always stopped and removed.
// Constraints: exec stays the only package that starts the runtime CLI. The profile only
// returns an argv; checkContainerCommand refuses any argv that is not hardened, whatever
// the profile says, so a profile bug cannot start a privileged container.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// ContainerRun is what exec knows when it starts a planned container.
type ContainerRun struct {
	Name, Image, Workspace, Workdir, EnvFile string
	Command                                  []string
	Mounts                                   []Mount // read-only host binds besides the workspace
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
	for _, m := range s.Mounts { // the job UID cannot own an agent path, so a writable host mount cannot be coherent
		if !m.ReadOnly {
			return -1, "", coded("E611", "a read-write mount is not available under the hardened container profile")
		}
	}
	envData, err := envLines(env)
	if err != nil {
		return -1, "", err
	}
	run := ContainerRun{Name: "nself-ci-" + s.AttemptID, Image: s.Job.Container.Image, Workspace: workspace, Workdir: workdir, EnvFile: envFileStdin,
		Command: s.Command, Mounts: s.Mounts, AttemptID: s.AttemptID, CoordinatorID: s.CoordinatorID, Slot: s.Slot, CPU: s.CPU, MemoryMB: s.MemoryMB, Network: s.Network}
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
	if err := checkSandboxRuntime(s.Job.Isolation, cmd, run.Image); err != nil {
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
	code, err := e.runCommandIn(ctx, bytes.NewReader(envData), append([]string{cmd.Binary}, cmd.Args...), workspace, dockerHostEnv(), sink, stop, nil, "")
	if err != nil {
		stop()
		err = coded("E613", "container run: "+err.Error())
	}
	e.cleanupPlanned(s, run)
	if err == nil && code == 125 { // the runtime itself failed (daemon, image pull, option), not the job
		err = coded("E613", "container runtime failed to start the container (exit 125)")
	}
	return code, resourceReason(ctx, code, tail.String()), err
}

// cleanupPlanned runs the profile's cleanup command, best effort and time boxed; the
// helper container is stopped and removed when the time box ends, so it never outlives the attempt.
func (e *Executor) cleanupPlanned(s JobSpec, run ContainerRun) {
	cmd, err := s.Plan.Cleanup(run)
	if err != nil || cmd == nil || checkContainerCommand(*cmd, run) != nil || checkSandboxRuntime(s.Job.Isolation, *cmd, run.Image) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 30*time.Second)
	defer cancel()
	stop := containerStopper(cmd.Binary, flagValueOf(cmd.Args, "--name"), e.Grace)
	local := Executor{Redactor: passThroughRedactor{}, Grace: e.Grace}
	if _, err := local.runCommand(ctx, append([]string{cmd.Binary}, cmd.Args...), run.Workspace, dockerHostEnv(), nil, stop, nil, ""); err != nil || ctx.Err() != nil {
		stop()
	}
}

// flagValueOf returns the value of `--name <v>` or `--name=<v>` in args, or "".
func flagValueOf(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
	}
	return ""
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
