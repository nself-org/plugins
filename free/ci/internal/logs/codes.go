package logs

// Purpose: the error codes of the stored-log package (Epic D14, E632-E633) and
// the typed error that carries one.
// E632 the log store is unusable or a write was rejected (bad name, sequence
// gap, corrupt file, unwritable directory); E633 there are no stored logs for
// the job, attempt or run asked about.
// Constraints: registered through model.Register, never a panic.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes this package raises.
const (
	CodeStore = "E632"
	CodeNone  = "E633"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeStore, Class: "infra", Summary: "stored job log unusable", Fix: "Check that the CI state directory is writable and not full; `nself ci runs prune` frees space"},
		{ID: CodeNone, Class: "usage", Summary: "no stored logs for that job or attempt", Fix: "List runs with `nself ci runs list`; logs are kept per attempt until retention removes them"},
	} {
		model.Register(c)
	}
}

// Error is a log failure with its code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, format string, a ...any) *Error { return Errorf(code, format, a...) }

// Errorf builds an Error with the given code (CodeStore or CodeNone).
func Errorf(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}

// AsError returns the *Error inside err, if any.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
