package notify

import "github.com/nself-org/plugins/free/ci/internal/model"

// E813 is this Ticket's share of the AUTO block (model.CodeRanges).
func init() {
	model.Register(model.Code{ID: "E813", Class: "infra", Summary: "a notification sink did not take the message", Fix: "Delivery counts are kept per sink (`nself ci doctor --selftest` shows them, P7-AUTO-20); a webhook needs https and a public address (notify.allow_private for a LAN one); a command sink must exit 0 within 10 seconds"})
}

// Error carries E813. Its text never holds a URL, argv or token: a sink is named by its name.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func coded(message string) error { return &Error{Code: "E813", Message: message} }
