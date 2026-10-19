package run

// Purpose: the two per-attempt seams artifacts need (P7-CACHE-03, lead
// ruling in the CACHE Epic Invalidations, 2026-10-18). A WorkspaceHook sees
// the finished job's workspace before it is removed; an AttemptFinalizeHook
// runs while the attempt is `finalizing`, before it may pass.
// Inputs:  hooks registered with RegisterWorkspace / RegisterAttemptFinalize.
// Outputs: an error from either hook fails the attempt (class infra) with the
// error's E-code; a pass is possible only when every hook returned nil.
// Constraints: hooks run in registration order inside guard (a panic is an
// error, not a crash); the finalize hook gets the attempt's current epoch,
// read just before the call, so a store write can fence on it.

import (
	"context"
	"os"
)

// WorkspaceHook runs after a job's command exited 0, inside its workspace; the
// root was opened by the executor before the job started.
type WorkspaceHook interface {
	AfterJob(ctx context.Context, a AttemptInfo, workspace *os.Root) error
}

// AttemptFinalizeHook runs between running -> finalizing and finalizing -> passed.
type AttemptFinalizeHook interface {
	AttemptFinalize(ctx context.Context, a AttemptInfo, epoch int64) error
}

// finalizeWanter is an optional interface of an AttemptFinalizeHook: Wants
// reports whether it has work for the attempt, so the run reads the epoch only
// when some hook does.
type finalizeWanter interface{ Wants(a AttemptInfo) bool }

// RegisterWorkspace adds a WorkspaceHook; call from init().
func RegisterWorkspace(h WorkspaceHook) { Registered.Workspaces = append(Registered.Workspaces, h) }

// RegisterAttemptFinalize adds an AttemptFinalizeHook; call from init().
func RegisterAttemptFinalize(h AttemptFinalizeHook) {
	Registered.AttemptFinalizes = append(Registered.AttemptFinalizes, h)
}

// afterJob is the executor callback for info's attempt (nil without hooks).
func (rs *runState) afterJob(info AttemptInfo) func(context.Context, *os.Root) error {
	if len(rs.hooks.Workspaces) == 0 {
		return nil
	}
	return func(ctx context.Context, workspace *os.Root) error {
		for _, h := range rs.hooks.Workspaces {
			if err := guard("workspace hook", func() error { return h.AfterJob(ctx, info, workspace) }); err != nil {
				return err
			}
		}
		return nil
	}
}

// attemptFinalize runs the finalize hooks of an attempt that is finalizing.
func (rs *runState) attemptFinalize(info AttemptInfo) error {
	var todo []AttemptFinalizeHook
	for _, h := range rs.hooks.AttemptFinalizes {
		if w, ok := h.(finalizeWanter); !ok || w.Wants(info) {
			todo = append(todo, h)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	bg, done := background(context.Background())
	defer done()
	_, epoch, err := rs.sess.Store.AttemptStateEpoch(bg, info.AttemptID)
	if err != nil {
		return err
	}
	for _, h := range todo {
		if err := guard("attempt finalize hook", func() error { return h.AttemptFinalize(bg, info, epoch) }); err != nil {
			return err
		}
	}
	return nil
}

// settleFinalizeFailed ends a finalizing attempt whose hook refused.
func (rs *runState) settleFinalizeFailed(id string, o JobOutcome, cause error) JobOutcome {
	to, err := rs.settle(id, "finalizing", "failed", "infra")
	if err != nil {
		return failed(o, err)
	}
	if to == "cancelled" {
		return cancelled(o, "", "cancelled by the operator")
	}
	return failed(o, cause)
}
