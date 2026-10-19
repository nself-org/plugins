package reproduce

// Purpose: the error codes of `ci runs reproduce` (Epic D14, E634-E635) and
// the typed error that carries one.
// E634 the recorded platform differs from this host, so the job is refused
// (an error); E635 the original ran on uncommitted changes, so only the
// recorded revision is reproduced (a warning, never an error).
// Constraints: registered through model.Register, never a panic. Missing
// evidence, a missing commit and a foreign repository reuse the evidence
// package's codes (E628, E630, E631).

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes this package raises.
const (
	CodePlatform = "E634"
	CodeUnbound  = "E635"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodePlatform, Class: "usage", Summary: "recorded platform differs from this host", Fix: "Reproduce the job on a host of the recorded platform, or read the stored evidence with `nself ci evidence <run>`"},
		{ID: CodeUnbound, Class: "usage", Summary: "the original run was not bound to its commit; only the recorded revision is reproduced", Fix: "Commit the changes and run the job again to get bound evidence"},
	} {
		model.Register(c)
	}
}

// Error is a reproduce failure with its code. Error() reads "E###: message".
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}

// warn is a warning line that carries its code.
func warn(code, format string, a ...any) string {
	return code + ": " + fmt.Sprintf(format, a...)
}
