package exec

import (
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Error carries the registered machine-readable error code.
type Error struct {
	Code, Message string
	// Err is the underlying cause, when there is one.
	Err error
}

func (e *Error) Error() string         { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func (e *Error) Unwrap() error         { return e.Err }
func coded(code, message string) error { return &Error{Code: code, Message: message} }

func init() {
	for _, code := range []model.Code{
		{ID: "E610", Class: "infra", Summary: "local process execution unavailable", Fix: "Use a supported local host"},
		{ID: "E611", Class: "infra", Summary: "requested isolation unavailable", Fix: "Choose process or baseline container isolation"},
		{ID: "E612", Class: "infra", Summary: "job preparation failed", Fix: "Check workspace and secret source"},
		{ID: "E613", Class: "infra", Summary: "local container execution failed", Fix: "Check Docker and the selected image"},
	} {
		model.Register(code)
	}
}
