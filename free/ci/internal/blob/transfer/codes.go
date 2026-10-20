package transfer

// Purpose: the error code of a blob transfer (Epic CACHE D20: E714). One error
// type carries the stable reason, so a caller branches on the reason and never
// on message text.
// Inputs:  none (registration at init).
// Outputs: Error, Failf, IsReason and the reason keys.
// Constraints: a message never holds blob bytes or a secret; digests are cut to
// 12 hex characters.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeTransfer is E714.
const CodeTransfer = "E714"

// Reason keys of E714.
const (
	ReasonStaleEpoch  = "stale_epoch"
	ReasonDigest      = "digest_mismatch"
	ReasonInterrupted = "interrupted"
	ReasonProtocol    = "protocol"
	ReasonSizeCap     = "size_cap"
)

// Error is a refused or failed transfer.
type Error struct {
	Reason string
	Msg    string
}

func (e *Error) Error() string { return CodeTransfer + " " + e.Reason + ": " + e.Msg }

// Code returns the E-code.
func (e *Error) Code() string { return CodeTransfer }

// Is matches on the code, and on the reason when the target names one.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && (t.Reason == "" || t.Reason == e.Reason)
}

// Failf builds an E714 error.
func Failf(reason, format string, a ...any) *Error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// IsReason reports whether err is an E714 error with the given reason.
func IsReason(err error, reason string) bool {
	var e *Error
	return errors.As(err, &e) && e.Reason == reason
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func init() {
	model.Register(model.Code{ID: CodeTransfer, Class: "infra", Summary: "blob transfer failed",
		Fix: "A remote attempt could not hand its artifact to the coordinator (stale lease, digest mismatch, interruption, protocol fault or size cap); run the job again, and raise NSELF_CI_ARTIFACT_MAX_FILE or free store space for a size_cap"})
}
