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
	// plannedForbidden lists flags no job container may carry (Epic D8).
	plannedForbidden = []string{"--privileged", "--pid", "--ipc", "--uts", "--cgroupns", "--device", "--cap-add", "--volumes-from", "--userns=host"}
	// socketPath matches any container runtime socket in a flag value.
	socketPath = regexp.MustCompile(`(?i)(docker|podman|containerd|crio)[^,]*\.sock`)
	// resourceSignatures are what a job prints when the kernel refuses a fork.
	resourceSignatures = []string{"Resource temporarily unavailable", "can't fork", "Cannot fork", "pthread_create failed"}
)

// checkContainerCommand refuses a planned command that is not a hardened `run`.
func checkContainerCommand(c ContainerCommand, run ContainerRun) error {
	bad := func(why string) error { return coded("E613", "planned container refused: "+why) }
	if !runtimeBinary.MatchString(c.Binary) {
		return bad("unknown runtime " + c.Binary)
	}
	if len(c.Args) == 0 || c.Args[0] != "run" {
		return bad("not a run command")
	}
	// only the flag region (before the image) is checked: the job command may carry any word
	flags := c.Args
	for i, a := range c.Args {
		if a == run.Image {
			flags = c.Args[:i]
			break
		}
	}
	if len(flags) == len(c.Args) {
		return bad("image not on the command line")
	}
	joined := " " + strings.Join(flags, " ") + " "
	for _, flag := range plannedForbidden {
		for _, a := range flags {
			if a == flag || strings.HasPrefix(a, flag+"=") {
				return bad("forbidden flag " + flag)
			}
		}
	}
	for i, a := range flags {
		switch {
		case a == "--network=host" || a == "--net=host" || (a == "--network" || a == "--net") && i+1 < len(flags) && flags[i+1] == "host":
			return bad("host network")
		case strings.HasPrefix(a, "-v") && !strings.HasPrefix(a, "--") || a == "--volume" || strings.HasPrefix(a, "--volume="):
			return bad("volume flag")
		case socketPath.MatchString(a):
			return bad("socket path")
		case a == "--mount" && i+1 < len(flags):
			if err := checkMount(flags[i+1], run.Workspace); err != nil {
				return err
			}
		}
	}
	for _, want := range []string{" --cap-drop=ALL ", " --security-opt no-new-privileges ", " --read-only "} {
		if !strings.Contains(joined, want) {
			return bad("missing" + strings.TrimRight(want, " "))
		}
	}
	return nil
}

// checkMount allows only a bind of the attempt workspace.
func checkMount(spec, workspace string) error {
	bad := func(why string) error { return coded("E613", "planned container refused: "+why) }
	fields := map[string]string{}
	for _, part := range strings.Split(spec, ",") {
		k, v, _ := strings.Cut(part, "=")
		fields[k] = v
	}
	if fields["type"] != "bind" || fields["src"] != workspace {
		return bad("mount outside the job root")
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

// resourceReason reports "resource_limit" for a SIGKILLed container (the memory
// limit) or output showing a refused fork (the pids limit); a deadline or cancel
// is never a resource reason by itself.
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
