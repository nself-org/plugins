package ssh

// Purpose: the typed error of the transport and the classification of what
// ssh printed before the agent said hello.
// Inputs:  ssh's stderr tail and exit status.
// Outputs: *Error with a code (E655, E656 or a code another package owns), a
// stable reason key and a message that never holds secret values.
// Constraints: only reason keys and fixed text reach a message; the stderr
// tail is truncated and included for connect-time failures only.

import (
	"errors"
	"fmt"
	"strings"
)

// Error is a transport failure.
type Error struct {
	Code    string
	Reason  string
	Message string
	Err     error
}

func (e *Error) Error() string {
	s := e.Code + " " + e.Reason
	if e.Message != "" {
		s += ": " + e.Message
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// IsLost reports whether err means the attempt is lost: the SSH session ended
// before a result arrived. There is no resume on this carrier (X11).
func IsLost(err error) bool {
	var te *Error
	return errors.As(err, &te) && te.Reason == ReasonSessionLost
}

// Reason returns the reason key of a transport error, or "".
func Reason(err error) string {
	var te *Error
	if errors.As(err, &te) {
		return te.Reason
	}
	return ""
}

func newErr(code, reason, format string, a ...any) *Error {
	return &Error{Code: code, Reason: reason, Message: fmt.Sprintf(format, a...)}
}

// rotateHint is the remediation named in every host-key refusal.
const rotateHint = "run `nself ci nodes rotate <node> --host-key SHA256:<fingerprint>` after verifying the node"

// classify turns ssh's stderr tail into an error for a connection that ended
// before hello.
func classify(node, stderr string, waitErr error) *Error {
	low := strings.ToLower(stderr)
	switch {
	case strings.Contains(low, "remote host identification has changed"), strings.Contains(low, "host key for") && strings.Contains(low, "has changed"),
		strings.Contains(low, "offending"):
		return newErr(CodeHostKey, ReasonHostKeyChanged, "host key of node %s changed; %s", node, rotateHint)
	case strings.Contains(low, "host key verification failed"), strings.Contains(low, "no matching host key"), strings.Contains(low, "host key is known"):
		return newErr(CodeHostKey, ReasonHostKeyUnknown, "host key of node %s is not pinned or does not match; %s", node, rotateHint)
	case strings.Contains(low, "timed out"), strings.Contains(low, "timeout"):
		return newErr(CodeTransport, ReasonConnectTimeout, "node %s did not answer in time", node)
	}
	e := newErr(CodeTransport, ReasonConnect, "cannot reach node %s over ssh", node)
	if tail := tailText(stderr, 300); tail != "" {
		e.Message += ": " + tail
	}
	e.Err = waitErr
	return e
}

// tailText keeps the last n bytes of s as one printable line.
func tailText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return strings.Join(strings.Fields(s), " ")
}
