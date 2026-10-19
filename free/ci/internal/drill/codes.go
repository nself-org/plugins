package drill

// Purpose: the error codes of drill jobs (P7-CI-02, Epic D14 range E640-E641).
// E640 a drill is invalid: it printed a ci.evidence document that fails the
// schema or contradicts itself, or its directory or drills_dir cannot be used;
// E641 a drill job has no passing run within its pipeline's max_age.
// Constraints: registered through model.Register, never a panic.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes this package raises.
const (
	CodeInvalid = "E640"
	CodeOverdue = "E641"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeInvalid, Class: "usage", Summary: "a drill is invalid", Fix: "Make the drill print one valid ci.evidence/v1 document on stdout (or none), and keep drills_dir entries to plain directories with an executable `run`"},
		{ID: CodeOverdue, Class: "usage", Summary: "a drill job is overdue", Fix: "Run the scheduled pipeline (`nself-ci --pipeline <name>`) and check why the schedule stopped"},
	} {
		model.Register(c)
	}
}

// Error is a drill failure with its code; it formats as "E###: message".
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}
