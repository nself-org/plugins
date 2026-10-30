// Package approval holds the `approval` admission step (P7-TRUST-24, Epic D18) and the adapters from
// the store's approval types to the pure rules in trust/source.
//
// Purpose: hold a run that needs an approval until a valid one exists, in one place that both the
// composition root (cmd) and the security suite import, so the suite runs the production step.
// Inputs:  the dispatch.View of an admission attempt; store approval bindings and states.
// Outputs: nil (admit) or a *dispatch.Refusal with reason approval.required.
// Constraints: the step reads only stored facts inside the admission transaction (no network) and
//
//	never writes; failing to read the facts fails the admission closed.
package approval

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

// Register installs the `approval` admission step.
func Register() error { return dispatch.RegisterStep("approval", Step) }

// VoidCheck adapts source.VoidReasons to the store's bindings.
func VoidCheck(approved, cur store.ApprovalBinding) []string {
	return source.VoidReasons(source.Binding(approved), source.Binding(cur))
}

// Needed adapts source.RequiresApproval to the store's approval state.
func Needed(st store.ApprovalState) bool {
	return source.RequiresApproval(st.Requires, st.Facts, model.TrustClass(st.SourceTrust))
}

// operatorTyped reports a run the operator authored and sandboxed on purpose: it was admitted at owner or
// internal, and only its recorded class is lower (`nself ci --trust untrusted -- <cmd>` records the class it
// was asked to run under, internal/run Coordinator.classify). No stranger's change waits on it, and nobody
// else could approve the operator's own command, so holding it for `runs approve` is a deadlock (the
// gVisor ad-hoc test waited 5 minutes on it). It applies only to a run with no source facts: a run whose
// source hook stored facts, and any run admitted at a lower class itself, keep the rule.
func operatorTyped(st store.ApprovalState, admitted model.TrustClass) bool {
	if st.Facts || (admitted != "owner" && admitted != "internal") {
		return false
	}
	recorded := model.TrustClass(st.SourceTrust)
	return recorded != admitted && recorded.Valid()
}

// Step is the `approval` admission step (D18). It refuses with approval.required while the run
// needs an approval and none is valid for its stored head SHA, config digest and head repo. Failing to
// read the facts fails the admission closed.
func Step(ctx context.Context, v dispatch.View) error {
	st, ok, err := v.ApprovalState(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("approval: run %s has no stored facts", v.PipelineID)
	}
	if !Needed(st) || operatorTyped(st, v.Request.Job.Trust) {
		return nil
	}
	hold := func(why string) error {
		return &dispatch.Refusal{Reason: model.PlacementReason(source.ReasonApprovalRequired), Detail: why}
	}
	switch reasons := VoidCheck(st.Approval.ApprovalBinding, st.Current()); {
	case !st.HasApproval:
		return hold("run " + st.Run + " waits for `nself ci runs approve " + st.Run + "`")
	case st.Approval.VoidedAt != 0:
		return hold("the approval was voided (" + st.Approval.VoidReason + "); approve the current head again")
	case len(reasons) > 0:
		return hold("the approval no longer matches the run (" + strings.Join(reasons, ",") + "); approve the current head again")
	}
	return nil
}
