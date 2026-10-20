package update

// Purpose: the recovery half of the upgrade state machine (engine.go): rollback to the kept binary, putting a drained node back,
//          and the audit helpers.
// Inputs:  the Node, the Result so far, the undo func Drain returned.
// Outputs: errors with their E656 or E666 code; audit rows.
// Constraints: a rollback never waits for its own audit row, uses a context that survives a cancelled run, and puts the node back
//          only after the previous binary said hello at the old version and a probe passed; audit details hold codes and versions only.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// rollback restores the previous binary after a failure past the swap and re-checks it; the node is put back only when that works.
func (e *Engine) rollback(ctx context.Context, n Node, res *Result, undo func(context.Context) error, rq Request, cause error) error {
	res.RolledBack = true
	// A cancelled run must still be able to put the node right.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), max(3*time.Minute, orDur(rq.HelloWait, DefaultHelloWait)+2*time.Minute))
	defer cancel()
	// The restore does not wait for its audit row: a store that fails right now must not leave a broken binary in place.
	auditErr := e.step(rctx, StepRollback, n, "from=%s to=%s code=%s", res.To, res.From, codeOf(cause))
	var rerr error
	if rerr = n.Restore(rctx); rerr == nil {
		if rerr = n.Restart(rctx); rerr == nil {
			var got string
			got, rerr = n.Hello(rctx, orDur(rq.HelloWait, DefaultHelloWait))
			if rerr != nil {
				// An agent that never left the old binary (it refused the upgrade) says no new hello: its current session is the proof.
				if v, verr := n.Version(rctx); verr == nil && v == res.From {
					got, rerr = v, nil
				}
			}
			if rerr == nil && got != res.From {
				rerr = fmt.Errorf("said hello as %s, not %s", got, res.From)
			}
		}
	}
	if rerr == nil {
		rerr = n.Probe(rctx)
	}
	if rerr != nil {
		return fmt.Errorf("E656: upgrade of %s failed (%v) and the rollback failed (%v); the node stays drained", n.ID(), oneLine(cause), oneLine(rerr))
	}
	if undo != nil {
		if err := undo(rctx); err != nil {
			return fmt.Errorf("E656: upgrade of %s failed (%v); the previous agent is back but the node could not be put back (%v)", n.ID(), oneLine(cause), oneLine(err))
		}
		_ = e.step(rctx, StepEnable, n, "version=%s", res.From)
	}
	if auditErr != nil {
		return fmt.Errorf("E656: upgrade of %s to %s failed and was rolled back to %s: %s (the rollback row could not be written: %s)", n.ID(), res.To, res.From, oneLine(cause), oneLine(auditErr))
	}
	return fmt.Errorf("E656: upgrade of %s to %s failed and was rolled back to %s: %s", n.ID(), res.To, res.From, oneLine(cause))
}

// reenable puts the node back after a failure before anything was installed.
func (e *Engine) reenable(ctx context.Context, n Node, undo func(context.Context) error, cause error) error {
	if undo != nil {
		if err := undo(context.WithoutCancel(ctx)); err != nil {
			return fmt.Errorf("%w (and the node could not be put back: %s)", cause, oneLine(err))
		}
		_ = e.step(context.WithoutCancel(ctx), StepEnable, n, "restored after a failure")
	}
	return cause
}

// fail audits a failure with its code only (no free text: an ssh error names the destination) and returns the error.
func (e *Engine) fail(ctx context.Context, n Node, cause error) error {
	if err := e.step(ctx, StepFailed, n, "code=%s", codeOf(cause)); err != nil {
		return fmt.Errorf("%w (and the failure could not be recorded: %s)", cause, oneLine(err))
	}
	return cause
}

func codeOf(err error) string {
	if c := lifecycle.CodeOf(err); c != "" {
		return c
	}
	return "none"
}

// refuse audits a refusal and returns it.
func (e *Engine) refuse(ctx context.Context, n Node, rerr error) error {
	if err := e.step(ctx, StepRefused, n, "reason=%s", ReasonOf(rerr)); err != nil {
		return err
	}
	return rerr
}

func (e *Engine) step(ctx context.Context, step string, n Node, format string, a ...any) error {
	if err := e.Audit.Step(ctx, step, n.ID(), fmt.Sprintf(format, a...)); err != nil {
		return fmt.Errorf("E656: cannot record the %s step of %s: %w", step, n.ID(), err)
	}
	return nil
}

func orDur(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}

func oneLine(err error) string {
	s := err.Error()
	for i, r := range s {
		if r == '\n' || r == '\r' {
			return s[:i]
		}
	}
	return s
}
