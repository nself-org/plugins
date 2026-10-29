package job

// Purpose: the error codes of the release job class (Epic CACHE D20).
// E715 is a release job the admission step refused; E716 is an operator
// resolution `ci runs resolve` refused. Each carries a stable reason.
// Constraints: registered through model.Register, never a panic.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Codes this package raises.
const (
	CodeNotAdmitted    = "E715"
	CodeResolveRefused = "E716"
)

// E715 reasons: one per way the evidence of a release job's deps falls short.
const (
	DepsFailing      = "deps_failing"         // a dep failed, errored or was skipped
	DepsStale        = "deps_stale"           // the inputs changed since the evidence, or freshness is unknown
	DepsUnbound      = "deps_unbound"         // dirty worktree or no commit: the evidence names no revision
	DepsJobsMismatch = "deps_jobs_mismatch"   // the evidence lacks a job the release job depends on
	SelfReported     = "self_reported"        // recorded by the job's own process, not the coordinator
	Adhoc            = "adhoc"                // an ad hoc or drill pipeline never gates
	Trust            = "trust"                // the source is trusted below internal
	DepsMismatch     = "deps_mismatch"        // another repository, revision or policy
	SelectionPartial = "selection_partial"    // not the full job selection
	DepsInvalid      = "deps_invalid"         // malformed, inconsistent or unverifiable evidence
	NoEvidence       = "no_evidence"          // the coordinator supplied no evidence for the deps
	DeployEnv        = "deploy_env"           // the job declares a deploy credential
	NoKey            = "no_idempotency_key"   // the attempt would run without NSELF_CI_IDEMPOTENCY_KEY
	RemotePlacement  = "remote_placement"     // a release job is placed away from the machine that runs the pipeline
	FactsExpired     = "facts_expired"        // the facts were not refreshed recently enough to judge
	Duplicate        = "duplicate_submission" // the attempt is already being submitted
	SignerRevoked    = "signer_revoked"       // the evidence signer was revoked between guard and admission
	SignerUnverified = "signer_unverified"    // the revocation ledger could not be read, so the signer is not known unrevoked
)

// E716 reasons.
const (
	KeyMismatch     = "key_mismatch"
	NotResolvable   = "not_resolvable"
	AlreadyResolved = "already_resolved"
)

func init() {
	model.Register(model.Code{ID: CodeNotAdmitted, Class: "usage", Summary: "release job not admitted",
		Fix: "Run the release job's dependencies again on a clean, committed tree, then retry the release"})
	model.Register(model.Code{ID: CodeResolveRefused, Class: "usage", Summary: "operator resolution refused",
		Fix: "Use the attempt id from `nself ci runs <run>` and the NSELF_CI_IDEMPOTENCY_KEY the release job received (its publish step logs it), then run `nself ci runs resolve` again"})
}

// Error is a refusal with its code and stable reason.
type Error struct{ Code, Reason, Message string }

func (e *Error) Error() string { return e.Code + " " + e.Reason + ": " + e.Message }

func refuse(code, reason, format string, a ...any) *Error {
	return &Error{Code: code, Reason: reason, Message: fmt.Sprintf(format, a...)}
}
