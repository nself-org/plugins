package bootstrap

// Purpose: the one error code of node bootstrap and of the V15 runner download (Epic D15: NODE owns
// E650-E669 and P7-NODE-06 uses E669 only). Every failure carries a stable Reason key, so callers and
// tests match on the key, never on the message.
// Inputs:  a reason key and a message.
// Outputs: *Error, whose text starts with the code.
// Constraints: the message never holds a token, a key or a remote command's raw output; a caller adds
//          only a short, control-free detail (see Detail).

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func init() {
	model.Register(model.Code{ID: CodeBootstrap, Class: "infra", Summary: "node bootstrap or pinned runner download failed",
		Fix: "Read the reason key in the message: no_root needs root or passwordless sudo for the ssh user; checksum means the downloaded runner does not match its pin"})
}

// CodeBootstrap is the code of every bootstrap and runner-download failure.
const CodeBootstrap = "E669"

// Reason keys.
const (
	ReasonNoRoot      = "no_root"      // the ssh user is neither root nor passwordless sudo
	ReasonUnsupported = "unsupported"  // no registered family matches the node's OS
	ReasonStep        = "step"         // a bootstrap step failed on the node
	ReasonUsage       = "usage"        // bad options (slots, missing seam)
	ReasonChecksum    = "checksum"     // a downloaded runner tarball did not match its pin
	ReasonRunnerArch  = "runner_arch"  // the node's CPU architecture has no pinned runner
	ReasonDuplicate   = "duplicate"    // a family name was registered twice
	ReasonFamilyEmpty = "family_empty" // a family without a name or nil
)

// Error is an E669 failure.
type Error struct {
	Reason string
	Msg    string
}

// Error implements error.
func (e *Error) Error() string { return CodeBootstrap + ": " + e.Msg }

// NewError builds an E669 with a reason key.
func NewError(reason, format string, args ...any) *Error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, args...)}
}

// ReasonOf returns the reason key of an E669 in err's chain, or "".
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

// Detail cuts err to one short line without control characters, for a message.
func Detail(err error) string {
	if err == nil {
		return ""
	}
	line := strings.SplitN(err.Error(), "\n", 2)[0]
	var b strings.Builder
	for _, r := range line {
		if r >= ' ' && r != 0x7f {
			b.WriteRune(r)
		}
	}
	out := []rune(b.String())
	if len(out) > 160 {
		out = out[:160]
	}
	return string(out)
}
