package dispatch

// Purpose: the error codes of the dispatcher (Epic D17). E693 is the only code
// this package registers: a foreground job that waited past max_queue.
// E618 (unplaceable) is registered by internal/plan and only raised here.
// Constraints: registered through model.Register, never a panic.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Codes the dispatcher raises.
const (
	CodeQueueTimeout = "E693"
	CodeUnplaceable  = "E618" // plan.CodeUnplaceable, reused (X17)
)

func init() {
	model.Register(model.Code{ID: CodeQueueTimeout, Class: "usage", Summary: "job waited past max_queue", Fix: "Raise --max-queue or free capacity, then run again"})
}

// Error is a dispatcher failure with its code.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}
