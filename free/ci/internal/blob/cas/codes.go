package cas

import (
	"errors"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Error carries the public blob error code and a stable reason.
type Error struct {
	Reason string
	Err    error
}

func (e *Error) Error() string { return "E710 " + e.Reason + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }
func (e *Error) Code() string  { return "E710" }
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && (t.Reason == "" || t.Reason == e.Reason)
}

var ErrDigestMismatch = &Error{Reason: "digest_mismatch", Err: errors.New("stored digest differs")}
var ErrSizeMismatch = &Error{Reason: "size_mismatch", Err: errors.New("stored size differs")}
var ErrPartialConflict = &Error{Reason: "partial_conflict", Err: errors.New("partial bytes or offset conflict")}

func init() {
	model.Register(model.Code{ID: "E710", Class: "infra", Summary: "blob integrity failure", Fix: "Discard the corrupt blob and retry transfer"})
}
