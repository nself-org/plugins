package hygiene

// Purpose: Cleanup, the one end-of-attempt routine the local run loop and the agent call: kill what
// the attempt left running, remove the resources labelled with its id, then remove its root.
// Inputs:  an attempt id.
// Outputs: nil when nothing of the attempt remains; E681 (hygiene.cleanup_failed) otherwise.
// Constraints: only the attempt's root and resources labelled with its id are touched; nothing that
// is unlabelled and nothing that belongs to a runner service; every step runs even if one fails;
// the routine is idempotent.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// Cleanup removes everything the attempt left behind.
func (h *Host) Cleanup(ctx context.Context, attempt string) error {
	if !ValidAttempt(attempt) {
		return leakError(ReasonBadAttempt, "invalid attempt id")
	}
	var problems []string
	add := func(what string, err error) {
		if err != nil {
			problems = append(problems, what+": "+firstLine(err.Error()))
			h.logf("hygiene: attempt %s: %s: %v", attempt, what, err)
		}
	}
	if h.DryRun {
		h.logf("hygiene: dry run: would kill, remove labelled resources and root %s/%s", h.Base, attempt)
		return nil
	}
	add("process kill", h.killProcesses(attempt))
	add("labelled resources", h.removeResources(ctx, attempt))
	if h.Base != "" {
		add("job root", RemoveConfined(h.Base, attempt, h.Logf))
	}
	if len(problems) > 0 {
		return leakError(ReasonCleanupFailed, "cleanup of attempt %s incomplete: %s", attempt, strings.Join(problems, "; "))
	}
	return nil
}

// killProcesses kills the attempt's marked process group and every process that still carries the
// attempt tag (a daemonised child that left its group).
func (h *Host) killProcesses(attempt string) error {
	var err error
	if h.Home != "" {
		err = exec.KillAttemptGroup(h.Home, attempt)
	}
	procs, scanErr := scanProcs(attempt, 0)
	if scanErr == nil {
		pids := make([]int, 0, len(procs))
		for _, p := range procs {
			pids = append(pids, p.PID)
		}
		killProcs(pids)
	}
	return err
}

// removeResources removes the containers, then the networks and volumes, labelled with the attempt.
func (h *Host) removeResources(ctx context.Context, attempt string) error {
	var failed []string
	rt := h.runtime()
	for _, kind := range Kinds {
		found, err := rt.List(ctx, kind, attempt)
		if err != nil {
			failed = append(failed, fmt.Sprintf("list %s: %s", kind, firstLine(err.Error())))
			continue
		}
		for _, r := range found {
			if err := rt.Remove(ctx, kind, r.ID); err != nil {
				failed = append(failed, fmt.Sprintf("remove %s %s: %s", kind, r.ID, firstLine(err.Error())))
			}
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%s", strings.Join(failed, "; "))
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
