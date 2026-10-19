package readiness

import "github.com/nself-org/plugins/free/ci/internal/model"

// E799 and E800 are this Ticket's share of the AUTO block (model.CodeRanges).
func init() {
	for _, code := range []model.Code{
		{ID: "E799", Class: "infra", Summary: "another watcher holds this host's run lock", Fix: "Wait for the other watcher or stop it, then retry"},
		{ID: "E800", Class: "infra", Summary: "readiness.cmd gave no usable answer", Fix: "Make the hook exit 0 (ready) or 3 (not ready, reason on its first output line) within 5 seconds; it runs with PATH and LANG only, so use absolute paths"},
	} {
		model.Register(code)
	}
}

// Error carries one of the registered readiness codes.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func coded(code, message string) error { return &Error{Code: code, Message: message} }
