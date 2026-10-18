package attest

// Purpose: the error code and the stable reason keys of a refused evidence
// document (Epic D25: E685 "evidence rejected").
// Inputs:  none (registration at init).
// Outputs: Reason, the Reason constants, Rejection (the error form).
// Constraints: reason keys are API: a consumer branches on them, so a key is
// never renamed or reused. The first ten are fixed by the Ticket; the last
// three are selection_partial, not_passing and invalid (documented in the CI trust guide, a P7-TRUST-11 docs follow-up).

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeEvidenceRejected is E685.
const CodeEvidenceRejected = "E685"

func init() {
	model.Register(model.Code{ID: CodeEvidenceRejected, Class: "usage", Summary: "evidence rejected",
		Fix: "Run the gate again on the exact revision with a clean worktree, or fix the reason named in the error"})
}

// Reason is one stable reason a document does not satisfy a gate.
type Reason string

const (
	ReasonStale                   Reason = "evidence.stale"
	ReasonUnbound                 Reason = "evidence.unbound"
	ReasonAdhoc                   Reason = "evidence.adhoc"
	ReasonRepoMismatch            Reason = "evidence.repo_mismatch"
	ReasonPolicyMismatch          Reason = "evidence.policy_mismatch"
	ReasonJobsMismatch            Reason = "evidence.jobs_mismatch"
	ReasonTrustBelowMin           Reason = "evidence.trust_below_min"
	ReasonSelfReportedNotAdmitted Reason = "evidence.self_reported_not_admitted"
	ReasonSignatureUnverifiable   Reason = "evidence.signature_unverifiable"
	ReasonRevisionMismatch        Reason = "evidence.revision_mismatch"
	ReasonSelectionPartial        Reason = "evidence.selection_partial"
	ReasonNotPassing              Reason = "evidence.not_passing"
	ReasonInvalid                 Reason = "evidence.invalid"
)

// reasonOrder fixes the order of every returned list, so output is stable.
var reasonOrder = []Reason{ReasonInvalid, ReasonRepoMismatch, ReasonRevisionMismatch, ReasonPolicyMismatch, ReasonJobsMismatch,
	ReasonStale, ReasonUnbound, ReasonAdhoc, ReasonTrustBelowMin, ReasonSelfReportedNotAdmitted, ReasonSignatureUnverifiable,
	ReasonSelectionPartial, ReasonNotPassing}

// reasonSet collects reasons and lists them in reasonOrder.
type reasonSet map[Reason]bool

func (s reasonSet) add(r ...Reason) {
	for _, x := range r {
		s[x] = true
	}
}

func (s reasonSet) list() []Reason {
	out := []Reason{}
	for _, r := range reasonOrder {
		if s[r] {
			out = append(out, r)
		}
	}
	return out
}

// Rejection is the error form of a refusal; it carries code E685.
type Rejection struct {
	Reasons []Reason
	Detail  string
}

func (r *Rejection) Error() string {
	keys := make([]string, len(r.Reasons))
	for i, x := range r.Reasons {
		keys[i] = string(x)
	}
	msg := CodeEvidenceRejected + ": evidence rejected: " + strings.Join(keys, ", ")
	if r.Detail != "" {
		msg += " (" + r.Detail + ")"
	}
	return msg
}

// Reject returns nil for no reasons and a *Rejection otherwise.
func Reject(reasons []Reason, detail string) error {
	if len(reasons) == 0 {
		return nil
	}
	return &Rejection{Reasons: append([]Reason(nil), reasons...), Detail: detail}
}
