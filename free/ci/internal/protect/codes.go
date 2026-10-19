package protect

// Purpose: the error codes of `nself ci policy protect` (P7-CI-07, Epic D14
// range E642-E643).
// E642 a pre-push hook already exists, so the installer stops and prints how
// to chain it; E643 branch protection cannot be read or written for this
// repository (a private repository on a free plan answers 403), so the
// documented equivalent (the hook and the auditor) applies.
// Constraints: registered through model.Register, never a panic.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes this package raises.
const (
	CodeHookExists = "E642"
	CodeNoPlan     = "E643"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeHookExists, Class: "usage", Summary: "a pre-push hook already exists", Fix: "Call the printed `nself ci evidence verify` line from your existing pre-push hook; nothing was overwritten"},
		{ID: CodeNoPlan, Class: "usage", Summary: "branch protection is not available for this repository", Fix: "Use the pre-push hook (`nself ci policy protect --hook`) and the auditor (`--audit <days>`) instead"},
	} {
		model.Register(c)
	}
}

// Error is a protect failure with its code; it formats as "E###: message".
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}
