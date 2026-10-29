package inventory

// Purpose: the code and typed error of `nself ci nodes sync` (P7-AUTO-04).
// Inputs:  none.
// Outputs: E793 registered through model.Register; *Error carrying the code and a stable reason key.
// Constraints: one code for every refusal of the whole input (size, time, strict decode, schema, a
//          failed discovery command, a TEAM coordinator); a rejected single entry is a Reject row in the
//          summary, never an error.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeInventory is the code every whole-input refusal carries.
const CodeInventory = "E793"

// Reason keys carried by Error.Reason.
const (
	ReasonTooLarge   = "too_large"
	ReasonTimeout    = "timeout"
	ReasonCmdFailed  = "cmd_failed"
	ReasonDecode     = "decode"
	ReasonSchema     = "schema"
	ReasonTeam       = "team_coordinator"
	ReasonNoRunner   = "no_runner"
	ReasonReadFailed = "read_failed"
)

func init() {
	model.Register(model.Code{ID: CodeInventory, Class: "usage", Summary: "runner inventory refused",
		Fix: "Give a file or a command that prints one inventory/v1 document under 1 MiB within 10 seconds; run sync on the machine that owns the nodes, not on a TEAM coordinator"})
}

// Error is a refusal of the whole inventory input.
type Error struct{ Code, Reason, Msg string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Msg) }

func newErr(reason, format string, a ...any) *Error {
	return &Error{Code: CodeInventory, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the reason key of an inventory error, or "".
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
