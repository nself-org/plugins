package registry

import (
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type Error struct{ Code, Reason string }

func (e *Error) Error() string    { return fmt.Sprintf("%s: %s", e.Code, e.Reason) }
func invalid(reason string) error { return &Error{"E651", reason} }

func init() {
	model.Register(model.Code{ID: "E650", Class: "usage", Summary: "node not found", Fix: "List nodes and use a registered id"})
	model.Register(model.Code{ID: "E651", Class: "usage", Summary: "invalid node document or version", Fix: "Refresh the capability and retry"})
}
