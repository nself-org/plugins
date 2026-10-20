package source

// Purpose: the binding rules of a run approval (Epic D18): what an approval is bound to, when it is
// still valid, and which changes void it.
// Inputs:  the Binding an approver saw and the Binding the run has now.
// Outputs: Valid, VoidReasons, RequiresApproval, the Reason key of a held run.
// Constraints: pure (no os, no exec, no net, no database/sql). A binding is (run, head SHA, config
//          digest, head repo); an approval keyed by run id alone is never valid, so a binding with an
//          empty run, head SHA or config digest is never valid. The head repo may be empty on both
//          sides (a local run has no forge) but must then be empty on both. SHAs compare exactly
//          (case-folded hex); repositories compare case-insensitively as forges do; the config digest
//          compares exactly.

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// ReasonApprovalRequired is the placement reason of a held run (trust.ApprovalRequired).
const ReasonApprovalRequired = "approval.required"

// Void reason keys: each names one binding field that changed.
const (
	VoidRun          = "void.run"
	VoidHeadSHA      = "void.head_sha"
	VoidConfigDigest = "void.config_digest"
	VoidHeadRepo     = "void.head_repo"
	VoidIncomplete   = "void.incomplete"
)

// Binding is what an approval is bound to.
type Binding struct {
	Run          string
	HeadSHA      string
	ConfigDigest string
	HeadRepo     string
}

// Complete reports whether the binding names a run, a head SHA and a config digest.
func (b Binding) Complete() bool {
	return b.Run != "" && b.HeadSHA != "" && b.ConfigDigest != ""
}

// Approval is a recorded approval: the binding the approver saw, and who approved it.
type Approval struct {
	Binding
	Approver string
	Spend    bool
}

// VoidReasons lists the binding fields in which cur differs from the approval, in a fixed order.
// An incomplete approved or current binding voids with VoidIncomplete alone.
func VoidReasons(a Binding, cur Binding) []string {
	if !a.Complete() || !cur.Complete() {
		return []string{VoidIncomplete}
	}
	var out []string
	if a.Run != cur.Run {
		out = append(out, VoidRun)
	}
	if !strings.EqualFold(a.HeadSHA, cur.HeadSHA) {
		out = append(out, VoidHeadSHA)
	}
	if a.ConfigDigest != cur.ConfigDigest {
		out = append(out, VoidConfigDigest)
	}
	if !strings.EqualFold(a.HeadRepo, cur.HeadRepo) {
		out = append(out, VoidHeadRepo)
	}
	return out
}

// Valid reports whether the approval still holds for the current binding. A re-run of the same SHA
// and digest keeps it; any change of head SHA, config digest or head repo voids it.
func Valid(a Binding, cur Binding) bool { return len(VoidReasons(a, cur)) == 0 }

// RequiresApproval says whether a run waits for an approval. recorded is the classification fact the
// source hook stored for the run (ok=false when none was stored); without it only an untrusted class
// holds: a collaborator's pushed commit is not held, a stranger's is.
func RequiresApproval(recorded, ok bool, class model.TrustClass) bool {
	if ok {
		return recorded
	}
	return trust.DecodeTrust(string(class)) == untrusted
}
