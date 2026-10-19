package detach

// Purpose: the error code of `nself ci --detach` (P7-CI-63, Epic D30): E648
// covers every way a detach can fail, told apart by a reason key.
// Inputs: none. Outputs: Error, the reason keys, the E648 registration.
// Constraints: one code only (E648); reasons are spawn, no_queued_event,
// child_exit; the message of child_exit carries a redacted tail of the child
// stream (at most TailBytes of it).

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeID is the registered code.
const CodeID = "E648"

// Reason keys of E648.
const (
	ReasonSpawn      = "spawn"
	ReasonNoQueued   = "no_queued_event"
	ReasonChildExit  = "child_exit"
	reasonSeparator  = ": "
	tailHeaderPrefix = "\nchild output (last bytes, redacted):\n"
)

func init() {
	model.Register(model.Code{ID: CodeID, Class: "infra", Summary: "detached run did not start (reason spawn, no_queued_event or child_exit)",
		Fix: "Run the same command without --detach to see the error, then retry"})
}

// Error is a failed detach.
type Error struct {
	Reason  string // spawn, no_queued_event or child_exit
	Message string
	Tail    string // redacted tail of the child stream, may be empty
	Err     error
}

func (e *Error) Error() string { return CodeID + " " + e.Detail() }

// Detail is the message without the code: "<reason>: <message>" and the tail.
func (e *Error) Detail() string {
	s := e.Reason + reasonSeparator + e.Message
	if e.Tail != "" {
		s += tailHeaderPrefix + e.Tail
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

func fail(reason string, err error, format string, a ...any) *Error {
	return &Error{Reason: reason, Message: fmt.Sprintf(format, a...), Err: err}
}
