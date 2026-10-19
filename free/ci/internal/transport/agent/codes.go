package agent

// Purpose: E661 "job token rejected" and its reason keys (Epic D15, R-DS-15), plus the transport error of the
//          pull carrier (E656, the code the ssh transport registers for a transport failure).
// Inputs:  none.
// Outputs: Denial (the one error type of the token gate), Error (carrier failures), the code registered through
//          model.Register.
// Constraints: a message holds the reason key and ids only, never a token, a key or a certificate body; a
//          Denial answers Refusal() true, which is how agentapi tells "refuse this call" from "the coordinator
//          failed".

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeToken is E661: a job token was refused.
const CodeToken = "E661"

// CodeTransport is E656, registered by the ssh transport; this package only uses the id.
const CodeTransport = "E656"

// Reason keys of an E661 refusal.
const (
	ReasonMissing          = "missing"           // the call carried no token
	ReasonInvalid          = "invalid"           // malformed, unknown key or bad signature
	ReasonExpired          = "expired"           // past the token's exp
	ReasonWrongRunner      = "wrong_runner"      // minted for another node
	ReasonWrongJob         = "wrong_job"         // minted for another attempt or epoch
	ReasonTerminal         = "terminal"          // the attempt is over
	ReasonReplayed         = "replayed"          // the job's result was already accepted with this token
	ReasonWrongCertificate = "wrong_certificate" // bound to another certificate
	ReasonWrongSession     = "wrong_session"     // bound to another session of this certificate
)

// Reason keys of an E656 failure on this carrier.
const (
	ReasonNoAgent      = "transport.agent_not_connected"
	ReasonLeaseLost    = "transport.agent_lease_lost"
	ReasonQueueFull    = "transport.agent_queue_full"
	ReasonCancelLate   = "transport.agent_cancel_timeout"
	ReasonNotConfigure = "transport.agent_not_configured"
	ReasonDuplicate    = "transport.agent_duplicate_lease"
)

func init() {
	model.Register(model.Code{ID: CodeToken, Class: "usage", Summary: "job token rejected",
		Fix: "The agent must present the token of its own lease from the session and certificate that first used it; restart the job if the token expired or the attempt ended"})
}

// Denial is an E661 refusal.
type Denial struct{ Reason, Msg string }

func (d *Denial) Error() string { return fmt.Sprintf("%s %s: %s", CodeToken, d.Reason, d.Msg) }

// Refusal marks the error as a refusal of the call, not a failure of the coordinator.
func (d *Denial) Refusal() bool { return true }

// ErrCode and ErrReason map the error to a wire body.
func (d *Denial) ErrCode() string   { return CodeToken }
func (d *Denial) ErrReason() string { return d.Reason }

func deny(reason, format string, a ...any) error {
	return &Denial{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// DenialReason returns the E661 reason key of err, or "" when err is not a Denial.
func DenialReason(err error) string {
	var d *Denial
	if errors.As(err, &d) {
		return d.Reason
	}
	return ""
}

// Error is a failure of the pull carrier on the coordinator side.
type Error struct {
	Code, Reason, Msg string
	Err               error
}

func (e *Error) Error() string {
	s := e.Code + " " + e.Reason
	if e.Msg != "" {
		s += ": " + e.Msg
	}
	return s
}
func (e *Error) Unwrap() error { return e.Err }

// ReasonOf returns the transport reason key of err, or "".
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

func fail(reason, format string, a ...any) *Error {
	return &Error{Code: CodeTransport, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}
