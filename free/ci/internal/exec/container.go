package exec

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var imageRef = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(?:@sha256:[a-fA-F0-9]{64})?$`)

func dockerHostEnv() []string {
	var env []string
	for _, key := range []string{"PATH", "HOME", "USER", "TMPDIR", "XDG_RUNTIME_DIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

// containerWorkdir maps the job workdir to its path under the /repo mount.
func containerWorkdir(s JobSpec) (string, error) {
	if s.Job.Workdir == "" || s.Job.Workdir == "." {
		return "/repo", nil
	}
	rel := filepath.Clean(s.Job.Workdir)
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", coded("E612", "workdir escapes workspace")
	}
	return "/repo/" + filepath.ToSlash(rel), nil
}

func (e *Executor) runContainer(ctx context.Context, s JobSpec, workspace string, env []string) (int, error) {
	if s.Job.Container == nil || !imageRef.MatchString(s.Job.Container.Image) || strings.Contains(s.Job.Container.Image, "..") {
		return -1, coded("E613", "invalid container image")
	}
	workdir, err := containerWorkdir(s)
	if err != nil {
		return -1, err
	}
	envFile, removeEnv, err := writeEnvFile(s.Home, env)
	if err != nil {
		return -1, err
	}
	defer removeEnv()
	name := "nself-ci-" + s.AttemptID
	args := []string{"run", "--rm", "--name", name, "--label", "nself.ci.attempt=" + s.AttemptID, "--label", "nself.ci.coordinator=" + s.CoordinatorID}
	if s.Network == "" || s.Network == "none" {
		args = append(args, "--network=none")
	} else if s.Network != "internet" {
		return -1, coded("E613", "unsupported network scope")
	}
	if s.CPU <= 0 {
		s.CPU = 1.5
	}
	if s.MemoryMB <= 0 {
		s.MemoryMB = 1024
	}
	args = append(args, "--read-only", "--tmpfs=/tmp:size=256m", "--cpus="+strconv.FormatFloat(s.CPU, 'f', -1, 64), "--memory="+strconv.Itoa(s.MemoryMB)+"m", "--memory-swap="+strconv.Itoa(s.MemoryMB)+"m")
	args = append(args, "--mount", "type=bind,src="+workspace+",dst=/repo", "--workdir="+workdir, "--env-file", envFile)
	args = append(args, s.Job.Container.Image)
	args = append(args, s.Command...)
	stop := containerStopper("docker", name, e.Grace)
	code, err := e.runCommand(ctx, append([]string{"docker"}, args...), workspace, dockerHostEnv(), s.Output, stop, nil, "")
	if err != nil {
		stop()
		return code, fmt.Errorf("%w: %v", coded("E613", "docker run"), err)
	}
	return code, nil
}

// writeEnvFile writes the job environment to a 0600 file under home so no value
// reaches an argv, and returns the path and its remover.
func writeEnvFile(home string, env []string) (string, func(), error) {
	file, err := os.CreateTemp(home, "ci-env-*")
	if err != nil {
		return "", nil, err
	}
	remove := func() { _ = file.Close(); _ = os.Remove(file.Name()) }
	if err := file.Chmod(0600); err != nil {
		remove()
		return "", nil, err
	}
	for _, item := range env {
		if strings.ContainsAny(item, "\r\n") {
			remove()
			return "", nil, coded("E612", "newline in environment")
		}
		if _, err := fmt.Fprintln(file, item); err != nil {
			remove()
			return "", nil, err
		}
	}
	if err := file.Close(); err != nil {
		remove()
		return "", nil, err
	}
	return file.Name(), remove, nil
}

// containerStopper returns a once-only stop-then-remove of the named container
// through the given runtime CLI, bounded by the grace period.
func containerStopper(binary, name string, grace time.Duration) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			deadline := time.Now().Add(grace)
			stopBudget := grace * 3 / 4
			stopCtx, cancelStop := context.WithTimeout(context.Background(), stopBudget)
			seconds := int(stopBudget / time.Second)
			_ = osexec.CommandContext(stopCtx, binary, "stop", "-t", strconv.Itoa(seconds), name).Run()
			cancelStop()
			if remain := time.Until(deadline); remain > 0 {
				rmCtx, cancelRM := context.WithTimeout(context.Background(), remain)
				_ = osexec.CommandContext(rmCtx, binary, "rm", "-f", name).Run()
				cancelRM()
			}
		})
	}
}
