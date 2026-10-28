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
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	// Interactive says the job has a stdin (a one-job runner credential, D-1001): the planner adds
	// --interactive and the environment reaches the CLI on fd 3 (envFileFD) instead of its stdin.
	Interactive bool
	// RunnerRoot, when set, is the path of an exec-capable tmpfs the container gets besides /tmp (a forge
	// runner's root; RunnerRootPath is the only value the backstop accepts).
	RunnerRoot string
	// Entrypoint replaces the image entrypoint of the job container (the cleanup helper never gets it).
	Entrypoint string
	// JobImageCleanup asks a planner to run the cleanup helper in the job's own image. The executor sets it for the
	// second try when the planner's pinned helper image could not start (not pulled and no registry reachable).
	JobImageCleanup bool
}

// ContainerCommand is one runtime CLI invocation: Binary is docker or podman and
// Args follow it. Image is the image the command runs; empty means ContainerRun.Image
// (the cleanup helper runs a pinned helper image, never the job's).
type ContainerCommand struct {
	Binary string
	Args   []string
	Image  string
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
	// The CLI reads the environment from its stdin. A job with a stdin of its own needs that stream, so
	// the environment moves to fd 3 (a pipe, never a file) and the CLI's stdin is the job's.
	interactive := s.Stdin != nil
	envFile := envFileStdin
	if interactive {
		envFile = envFileFD
	}
	run := ContainerRun{Name: "nself-ci-" + s.AttemptID, Image: s.Job.Container.Image, Workspace: workspace, Workdir: workdir, EnvFile: envFile, Interactive: interactive,
		RunnerRoot: map[bool]string{true: RunnerRootPath}[s.RunnerRoot], Entrypoint: s.Entrypoint,
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
	var cliStdin io.Reader = bytes.NewReader(envData)
	var extra []*os.File
	if interactive {
		pr, pw, perr := os.Pipe()
		if perr != nil {
			return -1, "", coded("E612", "environment pipe: "+perr.Error())
		}
		go func() { _, _ = pw.Write(envData); _ = pw.Close() }()
		cliStdin, extra = s.Stdin, []*os.File{pr}
	}
	began := time.Now()
	code, _, err := e.runCommandFiles(ctx, cliStdin, extra, append([]string{cmd.Binary}, cmd.Args...), workspace, dockerHostEnv(), sink, stop, nil, "")
	if err != nil {
		stop()
		err = coded("E613", "container run: "+err.Error())
	}
	if cleanErr := e.cleanupPlanned(s, run); cleanErr != nil && s.Output != nil {
		// not a failure by itself: the host removes what it owns, and a workspace it cannot empty fails a clean run (E610)
		s.Output([]byte("nself-ci: workspace cleanup helper did not finish: " + cleanErr.Error() + "\n"))
	}
	var facts containerFacts
	if err == nil && (code == 125 || code == 137) {
		facts = askRuntime(cmd.Binary, run.Name, began) // exit codes alone cannot tell the runtime's failure or an OOM kill from the job's own
	}
	if err == nil && code == 125 && !facts.jobRan() { // the runtime itself failed (daemon, image pull, option), not the job
		err = coded("E613", "container runtime failed to start the container (exit 125)")
	}
	return code, resourceReason(ctx, code, tail.String(), facts), err
}

// cleanupPlanned runs the profile's cleanup command, best effort and time boxed; the
// helper container is stopped and removed when the time box ends, so it never outlives the attempt.
// The profile's helper image is tried first. If it does not start (an air-gapped node that never pulled it), the
// cleanup is tried once in the job's own image, as it ran before the helper image existed.
// It returns why the cleanup did not run or did not finish (nil when none is needed or it ran clean).
func (e *Executor) cleanupPlanned(s JobSpec, run ContainerRun) error {
	run.Interactive, run.RunnerRoot, run.EnvFile, run.Entrypoint = false, "", "", "" // the helper has no stdin, runner root, environment or entrypoint of the job
	helper, err := e.cleanupOnce(s, run)
	if err == nil || !helper {
		return err
	}
	run.JobImageCleanup = true
	_, err2 := e.cleanupOnce(s, run)
	if err2 != nil {
		return coded("E613", "helper image: "+err.Error()+"; job image: "+err2.Error())
	}
	return nil
}

// cleanupOnce runs one cleanup command. helper reports that the command ran an image other than the job's.
func (e *Executor) cleanupOnce(s JobSpec, run ContainerRun) (helper bool, err error) {
	cmd, err := s.Plan.Cleanup(run)
	if err != nil || cmd == nil {
		return false, err
	}
	if cmd.Image != "" && cmd.Image != run.Image { // the helper runs its own pinned image; the backstop checks that one
		helper, run.Image = true, cmd.Image
	}
	if err := checkContainerCommand(*cmd, run); err != nil {
		return helper, err
	}
	if err := checkSandboxRuntime(s.Job.Isolation, *cmd, run.Image); err != nil {
		return helper, err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 30*time.Second)
	defer cancel()
	stop := containerStopper(cmd.Binary, flagValueOf(cmd.Args, "--name"), e.Grace)
	local := Executor{Redactor: passThroughRedactor{}, Grace: e.Grace}
	tail := &outputTail{max: 512}
	code, err := local.runCommand(ctx, append([]string{cmd.Binary}, cmd.Args...), run.Workspace, dockerHostEnv(), tail.write, stop, nil, "")
	if err != nil || ctx.Err() != nil {
		stop()
		if err == nil {
			err = ctx.Err()
		}
		return helper, err
	}
	if code != 0 {
		return helper, coded("E613", "cleanup helper exited "+strconv.Itoa(code)+": "+strings.TrimSpace(tail.String()))
	}
	return helper, nil
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
