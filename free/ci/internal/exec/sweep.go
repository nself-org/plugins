package exec

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
)

// StaleChecker reports whether a coordinator incarnation is no longer live.
type StaleChecker interface {
	Stale(context.Context, string) (bool, error)
}

// Sweep removes only labelled containers owned by stale coordinators.
func (e *Executor) Sweep(ctx context.Context, current string) error {
	home := os.Getenv("NSELF_CI_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return e.sweep(ctx, current, home)
}

func (e *Executor) sweep(ctx context.Context, current, home string) error {
	if e.Stale == nil {
		return nil
	}
	if err := e.sweepProcesses(ctx, home, current); err != nil {
		return err
	}
	if _, err := osexec.LookPath("docker"); err != nil {
		return nil
	}
	if osexec.CommandContext(ctx, "docker", "info").Run() != nil {
		return nil
	}
	out, err := osexec.CommandContext(ctx, "docker", "ps", "-a", "--filter", "label=nself.ci.coordinator", "--filter", "label=nself.ci.attempt", "--format", "{{.ID}} {{.Label \"nself.ci.coordinator\"}}").Output()
	if err != nil {
		return coded("E613", "docker orphan listing failed")
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 || parts[1] == current {
			continue
		}
		stale, err := e.Stale.Stale(ctx, parts[1])
		if err != nil {
			return err
		}
		if !stale {
			continue
		}
		if err := removeStaleContainer(ctx, parts[0]); err != nil {
			return coded("E613", "docker orphan removal failed")
		}
	}
	return nil
}

func removeStaleContainer(ctx context.Context, id string) error {
	out, err := osexec.CommandContext(ctx, "docker", "rm", "-f", id).CombinedOutput()
	if err != nil && strings.Contains(strings.ToLower(string(out)), "no such container") {
		return nil
	}
	return err
}
