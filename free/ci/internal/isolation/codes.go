package isolation

// Purpose: the isolation error codes and the one error type that carries them.
// Inputs:  none. Outputs: E678 (level unavailable) and E679 (network scope
// unenforceable), registered through model.Register.
// Constraints: only these two codes belong to this package (Epic D25); a profile
// reports a stable Reason next to the code and never falls back to a weaker level.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Stable reasons carried by Error and Availability.
const (
	ReasonNoProfile      = "isolation.no_profile"
	ReasonUnknownLevel   = "isolation.unknown_level"
	ReasonRuntimeMissing = "isolation.runtime_missing"
	ReasonRootful        = "isolation.rootful_runtime"
	ReasonProbeFailed    = "isolation.probe_failed"
	ReasonScope          = "network.scope_unenforceable"
)

func init() {
	for _, code := range []model.Code{
		{ID: "E678", Class: "infra", Summary: "requested isolation level unavailable on this runner", Fix: "Install or enable the runtime for the level, or run the job on a runner that has it"},
		{ID: "E679", Class: "infra", Summary: "network scope cannot be enforced by this isolation profile", Fix: "Use the none or lan scope, or install the network scope profile"},
	} {
		model.Register(code)
	}
}

// Error is a coded isolation refusal.
type Error struct{ Code, Reason, Message string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Reason) }

// Unavailable builds an E678 error.
func Unavailable(reason, format string, args ...any) error {
	return &Error{Code: "E678", Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// ScopeUnenforceable builds an E679 error.
func ScopeUnenforceable(format string, args ...any) error {
	return &Error{Code: "E679", Reason: ReasonScope, Message: fmt.Sprintf(format, args...)}
}
