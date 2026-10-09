package world

import (
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type Error struct {
	Reason string
	Cause  error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("E699: %s: %v", e.Reason, e.Cause)
	}
	return "E699: " + e.Reason
}
func (e *Error) Unwrap() error              { return e.Cause }
func failed(reason string, err error) error { return &Error{Reason: reason, Cause: err} }
func init() {
	model.Register(model.Code{ID: "E699", Class: "infra", Summary: "world snapshot unavailable", Fix: "Check the store and contributor registrations"})
}
