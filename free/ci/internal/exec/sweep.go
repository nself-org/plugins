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

// Sweep removes only labelled containers (docker or podman) owned by stale coordinators.
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

// sweepRuntimes are the container runtimes whose orphans the sweep removes: the profile may have run a job under
// either, and an agent crash leaves the container behind in the one that started it (D-0965).
var sweepRuntimes = []string{"docker", "podman"}

// sweepContainers removes the labelled containers of stale coordinators from every installed runtime. A runtime
// that fails does not stop the others from being swept; the first failure is returned.
func (e *Executor) sweepContainers(ctx context.Context, current string) error {
	var first error
	for _, binary := range sweepRuntimes {
		if err := e.sweepRuntime(ctx, current, binary); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// sweepRuntime removes the stale coordinators' labelled containers of one runtime; a missing or stopped runtime
// has nothing to sweep.
func (e *Executor) sweepRuntime(ctx context.Context, current, binary string) error {
	if _, err := osexec.LookPath(binary); err != nil {
		return nil
	}
	if _, err := RuntimeQuery(ctx, binary, "info"); err != nil { // the shared probe: hygiene and the isolation facts ask the same question
		return nil
	}
	out, err := RuntimeQuery(ctx, binary, "ps", "-a", "--filter", "label=nself.ci.coordinator", "--filter", "label=nself.ci.attempt", "--format", "{{.ID}} "+labelExpr(binary, "nself.ci.coordinator"))
	if err != nil {
		return coded("E613", binary+" orphan listing failed")
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
		if err := removeStaleContainer(ctx, binary, parts[0]); err != nil {
			return coded("E613", binary+" orphan removal failed")
		}
	}
	return nil
}

func removeStaleContainer(ctx context.Context, binary, id string) error {
	out, err := osexec.CommandContext(ctx, binary, "rm", "-f", id).CombinedOutput()
	if err != nil && strings.Contains(strings.ToLower(string(out)), "no such container") {
		return nil
	}
	return err
}
