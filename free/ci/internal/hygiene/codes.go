package hygiene

// Purpose: the error codes of workspace hygiene and UID separation (Epic D25, E681-E682) and the
// typed error that carries one plus a stable reason.
// E681 a leak was detected or cleanup could not finish, so the node must drain; E682 no job UID
// is provisioned for the requested separation.
// Constraints: registered through model.Register, never a panic.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes this package raises.
const (
	CodeLeak       = "E681"
	CodeNoJobUID   = "E682"
	ReasonOSUnsupp = "os_unsupported"
)

// Stable reasons carried by Error.
const (
	ReasonLeak          = "hygiene.leak_detected"
	ReasonCleanupFailed = "hygiene.cleanup_failed"
	ReasonBadAttempt    = "hygiene.bad_attempt_id"
	ReasonUnconfined    = "hygiene.path_escapes_root"
	ReasonNoJobUID      = "isolation.job_uid_unprovisioned"
	ReasonNoSeparation  = "isolation.uid_separation"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeLeak, Class: "infra", Summary: "workspace leak detected or cleanup incomplete; node drained", Fix: "Run `nself ci doctor`, remove the reported leftovers, then undrain the node"},
		{ID: CodeNoJobUID, Class: "infra", Summary: "no job UID is provisioned for this separation", Fix: "Re-run `nself ci nodes add --system`, or enable unprivileged user namespaces for the agent user"},
	} {
		model.Register(c)
	}
}

// Error is a coded hygiene failure.
type Error struct {
	Code, Reason, Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Reason) }

func leakError(reason, format string, a ...any) *Error {
	return &Error{Code: CodeLeak, Reason: reason, Message: fmt.Sprintf(format, a...)}
}

func noUIDError(reason, format string, a ...any) *Error {
	return &Error{Code: CodeNoJobUID, Reason: reason, Message: fmt.Sprintf(format, a...)}
}
