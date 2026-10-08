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
		if err := osexec.CommandContext(ctx, "docker", "rm", "-f", parts[0]).Run(); err != nil {
			return coded("E613", "docker orphan removal failed")
		}
	}
	return nil
}
