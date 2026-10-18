package hygiene

// Purpose: Cleanup, the one end-of-attempt routine the local run loop and the agent call: kill what
// the attempt left running, remove the resources labelled with its id, then remove its root.
// Inputs:  an attempt id, and for a separated attempt the job UID it ran under.
// Outputs: nil when nothing of the attempt remains; E681 (hygiene.cleanup_failed) otherwise.
// Constraints: only the attempt's root and resources labelled with its id are touched; nothing that
// is unlabelled and nothing that belongs to a runner service; every step runs even if one fails and
// every failure is reported (a kill that was refused is a failure, not a success); idempotent.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// Cleanup removes everything an attempt that ran under the caller's UID left behind.
func (h *Host) Cleanup(ctx context.Context, attempt string) error {
	return h.CleanupJob(ctx, attempt, 0)
}

// CleanupJob is Cleanup for an attempt that ran under job UID uid (job-shim or userns). The agent
// cannot signal that UID's processes or unlink in directories it owns, so with a JobRunner the kill
// and the removal run as the job user, followed by a Reset of the user's own state (crontab, at
// jobs, SysV IPC, HOME); without one a refusal surfaces as E681 and the node drains.
func (h *Host) CleanupJob(ctx context.Context, attempt string, uid int) error {
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
	separated := uid > 0 && uid != os.Geteuid()
	add("process kill", h.killProcesses(ctx, attempt, uid, separated))
	add("labelled resources", h.removeResources(ctx, attempt))
	if h.Base != "" {
		if separated && h.Jobs != nil {
			add("job root contents (as job user)", h.Jobs.EmptyRoot(ctx, uid, h.Base, attempt))
		}
		add("job root", RemoveConfined(h.Base, attempt, h.Logf))
	}
	if separated && h.Jobs != nil {
		// the slot UID is reused: crontab, at jobs, SysV IPC and HOME must not reach the next project
		add("job user state (crontab, at, IPC, HOME)", h.Jobs.Reset(ctx, uid))
	}
	if len(problems) > 0 {
		return leakError(ReasonCleanupFailed, "cleanup of attempt %s incomplete: %s", attempt, strings.Join(problems, "; "))
	}
	return nil
}

// killProcesses kills the attempt's marked process group, every process of the job UID and every
// process that still carries the attempt tag (a daemonised child that left its group).
func (h *Host) killProcesses(ctx context.Context, attempt string, uid int, separated bool) error {
	var problems []string
	if separated && h.Jobs != nil {
		if err := h.Jobs.KillAll(ctx, uid); err != nil {
			problems = append(problems, "job user kill: "+firstLine(err.Error()))
		}
	}
	if h.Home != "" {
		if err := exec.KillAttemptGroup(h.Home, attempt); err != nil {
			problems = append(problems, firstLine(err.Error()))
		}
	}
	scanUID := 0 // a UID scan is valid only for a dedicated job UID: the caller's own UID is every process it owns
	if separated {
		scanUID = uid
	}
	procs, err := scanProcs(attempt, scanUID)
	switch {
	case err != nil && !isNoProc(err):
		problems = append(problems, "process scan: "+firstLine(err.Error()))
	case err == nil:
		if kerr := killProcs(procs); kerr != nil {
			problems = append(problems, firstLine(kerr.Error()))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// removeResources removes the containers, then the networks and volumes, labelled with the attempt.
func (h *Host) removeResources(ctx context.Context, attempt string) error {
	var failed []string
	rt := h.runtime()
	for _, kind := range Kinds {
		found, note, err := h.listResources(ctx, kind, attempt)
		if err != nil {
			failed = append(failed, fmt.Sprintf("list %s: %s", kind, firstLine(err.Error())))
			continue
		}
		if note != "" {
			h.logf("hygiene: attempt %s: skipped %s", attempt, note)
		}
		for _, r := range found {
			if err := rt.Remove(ctx, kind, r.ID, attempt); err != nil {
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
