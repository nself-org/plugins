package exec

import (
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Error carries the registered machine-readable error code.
type Error struct{ Code, Message string }

func (e *Error) Error() string         { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func coded(code, message string) error { return &Error{code, message} }

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
