package audit

// Purpose: the value scrubber (Epic D20). An event whose actor, action, target or detail contains a
// registered secret value, in any encoding the redactor knows, is refused and nothing is written.
// Inputs:  an Event and a Scanner (the redact registry; *redact.Redactor satisfies it).
// Outputs: nil, or an *Error with code E687 and reason audit_secret.
// Constraints: the error names the field, never the value or its position; E687 is registered once, by
//          internal/roles (this package only carries the code string); with no scanner nothing is
//          registered, so nothing can match.

import (
	"fmt"
	"sync/atomic"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// CodeRefused is E687 (registered in internal/roles/codes.go).
const CodeRefused = "E687"

// ReasonSecret is the reason key of a refused event.
const ReasonSecret = "audit_secret"

// Scanner finds registered secret values in a blob; *redact.Redactor is one.
type Scanner interface {
	Scan(blob []byte) []redact.Match
}

var defaultScanner atomic.Pointer[Scanner]

// SetScanner installs the process-wide scanner AppendTx and a Log without its own scanner use.
func SetScanner(s Scanner) {
	if s == nil {
		defaultScanner.Store(nil)
		return
	}
	defaultScanner.Store(&s)
}

// Error is a refused audit event.
type Error struct{ Reason, Field string }

func (e *Error) Error() string {
	return fmt.Sprintf("%s: audit event refused: field %s holds a registered secret value (%s)", CodeRefused, e.Field, e.Reason)
}

// ErrCode and ErrReason let callers map the error to a JSON body.
func (e *Error) ErrCode() string   { return CodeRefused }
func (e *Error) ErrReason() string { return e.Reason }

// Scrub refuses ev when any field holds a registered value.
func Scrub(sc Scanner, ev Event) error {
	if sc == nil {
		p := defaultScanner.Load()
		if p == nil {
			return nil
		}
		sc = *p
	}
	for _, f := range []struct{ name, v string }{{"actor", ev.Actor}, {"action", ev.Action}, {"target", ev.Target}, {"detail", ev.Detail}} {
		if len(sc.Scan([]byte(f.v))) > 0 {
			return &Error{Reason: ReasonSecret, Field: f.name}
		}
	}
	return nil
}
