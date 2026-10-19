package exec

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	return e.sweepContainers(ctx, current)
}

// sweepOnStart is the sweep a job start runs: the process sweep every time, the
// container sweep once per coordinator and TTL window.
func (e *Executor) sweepOnStart(ctx context.Context, current, home string) error {
	if e.Stale == nil {
		return nil
	}
	if err := e.sweepProcesses(ctx, home, current); err != nil {
		return err
	}
	now := time.Now()
	if !sweepDue(current, now) {
		return nil
	}
	if err := e.sweepContainers(ctx, current); err != nil {
		// a sweep that failed or was cancelled did not clean up: the next job
		// start must try again instead of trusting the window
		sweepRelease(current, now)
		return err
	}
	return nil
}

// containerSweepTTL is how long one coordinator's container sweep stands for
// every job it starts after: a run's jobs start within a second of each other,
// and each sweep costs two docker spawns (info, ps) on the job's start path.
const containerSweepTTL = 30 * time.Second

var (
	containerSweepMu sync.Mutex
	containerSweeps  = map[string]time.Time{} // coordinator id -> start of its last container sweep
)

// sweepDue claims the container sweep of current: true for the first caller in
// each TTL window, false for the rest (they do not wait for the claimant).
func sweepDue(current string, now time.Time) bool {
	containerSweepMu.Lock()
	defer containerSweepMu.Unlock()
	if last, ok := containerSweeps[current]; ok && now.Sub(last) < containerSweepTTL {
		return false
	}
	containerSweeps[current] = now
	return true
}

// sweepRelease gives back the claim taken at claimed, unless a later window has
// been claimed since.
func sweepRelease(current string, claimed time.Time) {
	containerSweepMu.Lock()
	defer containerSweepMu.Unlock()
	if last, ok := containerSweeps[current]; ok && last.Equal(claimed) {
		delete(containerSweeps, current)
	}
}

// sweepContainers removes the labelled containers of stale coordinators.
func (e *Executor) sweepContainers(ctx context.Context, current string) error {
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
