package store

import (
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Error preserves the fabric code at the store boundary.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string         { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func coded(code, message string) error { return &Error{code, message} }

func init() {
	for _, c := range []model.Code{
		{ID: "E604", Class: "usage", Summary: "illegal state transition", Fix: "Use a legal state transition"},
		{ID: "E605", Class: "infra", Summary: "state changed concurrently", Fix: "Refresh attempt state and retry"},
		{ID: "E606", Class: "infra", Summary: "database needs a newer reader", Fix: "Upgrade nself-ci before opening this database"},
		{ID: "E607", Class: "infra", Summary: "store operation failed", Fix: "Check local disk and retry"},
		{ID: "E608", Class: "usage", Summary: "idempotency key already used", Fix: "Use the original attempt or a new key"},
		{ID: "E609", Class: "usage", Summary: "operator action required", Fix: "Record an operator decision before proceeding"},
	} {
		model.Register(c)
	}
}
