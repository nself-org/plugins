package exec

import (
	"context"
	"fmt"
	osexec "os/exec"
	"strconv"
	"strings"
)

func (e *Executor) runContainer(ctx context.Context, s JobSpec, workspace string, env []string) (int, error) {
	if s.Job.Container == nil || s.Job.Container.Image == "" {
		return -1, coded("E613", "container image is required")
	}
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
	args = append(args, "--mount", "type=bind,src="+workspace+",dst=/repo", "--workdir=/repo")
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if ok && safeID.MatchString(key) && !strings.Contains(key, ".") {
			args = append(args, "--env", key)
		}
	}
	args = append(args, s.Job.Container.Image)
	args = append(args, s.Command...)
	stop := func() {
		_ = osexec.Command("docker", "stop", "-t", "10", name).Run()
		_ = osexec.Command("docker", "rm", "-f", name).Run()
	}
	code, err := e.runCommand(ctx, append([]string{"docker"}, args...), workspace, env, s.Output, stop, nil)
	if err != nil {
		stop()
		return code, fmt.Errorf("%w: %v", coded("E613", "docker run"), err)
	}
	return code, nil
}
