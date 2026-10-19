package evidence

// Purpose: the error codes of the evidence producer and verifier (Epic D14,
// E628-E631) and the typed error that carries one.
// E628 the revision is not a full commit sha, or the arguments are unusable;
// E629 an evidence document is rejected (schema, revision, store mismatch,
// unreadable input at the revision); E630 the temporary worktree or the git
// facts could not be produced; E631 no evidence for that run or revision.
// Constraints: registered through model.Register, never a panic.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes this package raises.
const (
	CodeUsage    = "E628"
	CodeRejected = "E629"
	CodeWorktree = "E630"
	CodeNone     = "E631"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeUsage, Class: "usage", Summary: "evidence revision must be a full commit sha", Fix: "Pass the 40 or 64 hex digits of the commit, for example $(git rev-parse HEAD)"},
		{ID: CodeRejected, Class: "usage", Summary: "evidence document rejected", Fix: "Run `nself ci evidence verify <rev>` on a clean checkout to produce fresh evidence"},
		{ID: CodeWorktree, Class: "infra", Summary: "evidence worktree or git facts unavailable", Fix: "Check that git works in the repository and that the temporary directory is writable"},
		{ID: CodeNone, Class: "usage", Summary: "no evidence for that run or revision", Fix: "List runs with `nself ci runs`, or run `nself ci evidence verify <rev>`"},
	} {
		model.Register(c)
	}
}

// Error is an evidence failure with its code.
type Error struct {
	Code    string
	Message string
	// Exit is the `evidence verify` exit code the failure maps to (Epic D11).
	Exit int
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code string, exit int, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...), Exit: exit}
}

// AsError returns the *Error inside err, if any.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
