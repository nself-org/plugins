package policy

// Purpose: pin and clear-pin of the protected-branch commit (P7-TRUST-26, D15).
// Inputs: a Change with Op pin (SHA, Root) or clear, a Runner and the Store.
// Outputs: a Result; E672 when the commit is not reachable from the fetched
// origin default branch (or there is no origin to check against).
// Constraints: a pin is a full lowercase commit id; reachability is
// `git merge-base --is-ancestor <sha> <tip>` through the Runner after a fetch;
// the pin row and its audit row commit together; a machine-invoked caller can
// neither pin nor clear (a pin can select an older, looser policy).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func (w *Writer) pin(ctx context.Context, actor roles.Actor, c Change, t target) (Result, error) {
	res := Result{Op: c.Op, Scope: "project", Subject: t.subject, SHA: c.SHA}
	if w.Machine {
		return Result{}, w.refuseMachine(ctx, actor, c.Op, "pin/"+t.subject)
	}
	pw := store.PolicyWrite{Op: store.PolicyClear, Subject: t.subject, Actor: actor.Label()}
	detail := "subject=" + t.subject
	if c.Op == OpPin {
		if !fullSHA.MatchString(c.SHA) {
			return Result{}, refuse("a pin needs a full lowercase commit id")
		}
		if w.Run == nil {
			return Result{}, refuse("policy writer has no process runner")
		}
		b, err := readBranch(ctx, w.Run, c.Root)
		if err != nil {
			return Result{}, err
		}
		if b.Tip == "" {
			return Result{}, refuse("no fetched origin default branch to pin against")
		}
		if c.SHA != b.Tip && !reachable(ctx, w.Run, c.Root, c.SHA, b.Tip) {
			return Result{}, refuse("%s is not reachable from origin/%s", c.SHA, b.Name)
		}
		pw.Op, pw.SHA = store.PolicyPinOp, c.SHA
		detail += " sha=" + c.SHA
	}
	ev := audit.Event{Actor: actor.Label(), Action: "policy." + c.Op, Target: "pin", Detail: detail}
	return res, w.Store.ApplyWrite(ctx, pw, func(tx *store.Tx) error { return audit.AppendTx(tx, ev) })
}
