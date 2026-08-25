// Purpose: a minimal Docker CLI shell-exec adapter for the watchdog plugin.
//
// The core CLI's watchdog package used internal/health.DockerClient,
// internal/health.NewShellDockerClient, and internal/health.RestartContainer
// — but those types live in internal/health/restarter.go, which cannot move:
// cli/cmd/commands/start.go (a golden-path command) constructs its own
// Restarter from the same file. Forking internal/health itself would risk
// the two copies drifting on a security- and reliability-relevant path.
//
// What this plugin actually needs from that file, though, is not
// health-check business logic — it is three `docker` CLI subprocess calls
// (ps, inspect, restart) with no shared state file and no core-only
// dependency. That is boilerplate, not the thing CLI-R11 is protecting by
// keeping internal/health in core, so it is reimplemented here rather than
// left behind. Behavior matches internal/health/restarter.go's
// ShellDockerClient exactly: same docker subcommands, same flags, same
// output parsing.
//
// Inputs: a Docker daemon reachable via the `docker` CLI on PATH.
//
// Outputs: RestartContainer / RestartContainerInfo values consumed by the
// Watchdog in watchdog.go.
//
// Constraints: no dependency beyond the standard library and the `docker`
// binary — this package must stay buildable offline, like the rest of the
// plugin.
package watchdog

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DockerClient abstracts the Docker operations the Watchdog needs, for
// testability. Mirrors internal/health.DockerClient in the core CLI.
type DockerClient interface {
	// ContainerList returns running containers, optionally filtered by label.
	ContainerList(ctx context.Context, filters map[string]string) ([]RestartContainer, error)
	// ContainerInspect returns detailed info for a single container by ID or name.
	ContainerInspect(ctx context.Context, id string) (RestartContainerInfo, error)
	// ContainerRestart restarts a container. timeout is in seconds; 0 uses Docker default.
	ContainerRestart(ctx context.Context, id string, timeout int) error
}

// RestartContainer is a minimal container descriptor returned by ContainerList.
type RestartContainer struct {
	ID      string
	Name    string
	Service string
}

// RestartContainerInfo holds the health state of an inspected container.
type RestartContainerInfo struct {
	ID     string
	Health string // healthy, unhealthy, starting, none
}

// ShellDockerClient implements DockerClient by delegating to the `docker`
// CLI. Use NewShellDockerClient to construct one.
type ShellDockerClient struct {
	projectName string
}

// NewShellDockerClient returns a ShellDockerClient scoped to the given project.
// The project name is used to filter containers by the compose project label.
func NewShellDockerClient(projectName string) *ShellDockerClient {
	return &ShellDockerClient{projectName: projectName}
}

// ContainerList returns running containers for this project using docker ps.
func (s *ShellDockerClient) ContainerList(ctx context.Context, _ map[string]string) ([]RestartContainer, error) {
	filter := fmt.Sprintf("label=com.docker.compose.project=%s", s.projectName)
	cmd := exec.CommandContext(ctx, "docker", "ps",
		"--filter", filter,
		"--filter", "status=running",
		"--format", "{{.ID}}\t{{.Names}}\t{{.Label \"com.docker.compose.service\"}}",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}

	var result []RestartContainer
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		result = append(result, RestartContainer{
			ID:      parts[0],
			Name:    parts[1],
			Service: parts[2],
		})
	}
	return result, nil
}

// ContainerInspect returns health info for the given container ID using
// docker inspect. Only the Health field is needed here (unlike
// internal/docker.InspectContainer in core, which parses the full container
// record for other callers), so this queries just that field.
func (s *ShellDockerClient) ContainerInspect(ctx context.Context, id string) (RestartContainerInfo, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect",
		"--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}", id)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if strings.Contains(stderr, "No such object") || strings.Contains(stderr, "not found") {
				return RestartContainerInfo{}, fmt.Errorf("container %q not found", id)
			}
			return RestartContainerInfo{}, fmt.Errorf("docker inspect %q: %s", id, stderr)
		}
		return RestartContainerInfo{}, fmt.Errorf("docker inspect %q: %w", id, err)
	}
	return RestartContainerInfo{
		ID:     id,
		Health: strings.TrimSpace(string(out)),
	}, nil
}

// ContainerRestart restarts the given container using docker restart.
func (s *ShellDockerClient) ContainerRestart(ctx context.Context, id string, timeout int) error {
	args := []string{"restart"}
	if timeout > 0 {
		args = append(args, "-t", strconv.Itoa(timeout))
	}
	args = append(args, id)
	cmd := exec.CommandContext(ctx, "docker", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker restart %s: %w\n%s", id, err, string(out))
	}
	return nil
}
