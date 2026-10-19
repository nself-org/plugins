package records

// Purpose: the error codes of the artifact records (Epic CACHE D20: E712
// "artifact commit refused", E713 "artifact not found"). E711 is the cache
// capability code P7-CACHE-01 registers; a stale-capability refusal here
// carries the same code and does not register it again.
// Inputs:  none (registration at init).
// Outputs: Error, ErrNotFound, the reason keys.
// Constraints: "not found" and "not visible" are ONE error value with one
// message (existence must not leak); every other refusal names a reason.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	// CodeArtifact is E712.
	CodeArtifact = "E712"
	// CodeNotFound is E713.
	CodeNotFound = "E713"
	// codeCapability is E711 (registered by P7-CACHE-01).
	codeCapability = "E711"
)

// Reason keys of E712 (and the E711 capability refusal).
const (
	ReasonSecretMatch = "secret_match"
	ReasonSizeCap     = "size_cap"
	ReasonFileCap     = "file_cap"
	ReasonDiskFloor   = "disk_floor"
	ReasonMissing     = "missing"
	ReasonMissingBlob = "missing_blob"
	ReasonDigest      = "digest_mismatch"
	ReasonUncommitted = "uncommitted"
	ReasonDeclaration = "declaration"
	ReasonImage       = "image_layout"
	ReasonConfig      = "config"
	ReasonUnreadable  = "unreadable"
	ReasonDuplicate   = "duplicate_name"
	ReasonAttempt     = "attempt_state"
	ReasonEpoch       = "epoch"
)

// Error is a refusal with a stable code and reason.
type Error struct {
	ID     string
	Reason string
	Msg    string
}

func (e *Error) Error() string { return e.ID + " " + e.Reason + ": " + e.Msg }

// Code returns the E-code.
func (e *Error) Code() string { return e.ID }

// Is matches on the code, and on the reason when the target names one.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.ID == e.ID && (t.Reason == "" || t.Reason == e.Reason)
}

// ErrNotFound is returned for a missing id and for an id the viewer may not
// see; the two are indistinguishable on purpose.
var ErrNotFound = &Error{ID: CodeNotFound, Reason: "not_found", Msg: "no such artifact"}

func failf(reason, format string, a ...any) *Error {
	return &Error{ID: CodeArtifact, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

func refusedf(reason, format string, a ...any) *Error {
	return &Error{ID: codeCapability, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// IsReason reports whether err is an artifact refusal with the given reason.
func IsReason(err error, reason string) bool {
	var e *Error
	return errors.As(err, &e) && e.Reason == reason
}

// Refuse builds the E711 capability refusal; the store uses it for a commit
// whose attempt state or epoch is no longer the one the records were staged under.
func Refuse(reason, msg string) error { return refusedf(reason, "%s", msg) }

// Failf builds an E712 refusal (store and cmd use it for their own reasons).
func Failf(reason, format string, a ...any) error { return failf(reason, format, a...) }

func init() {
	model.Register(model.Code{ID: CodeArtifact, Class: "infra", Summary: "artifact commit refused",
		Fix: "Declare outputs that exist inside the job workdir, stay under the size caps and the disk floor and hold no registered secret; `nself ci artifacts prune` frees space"})
	model.Register(model.Code{ID: CodeNotFound, Class: "usage", Summary: "artifact not found",
		Fix: "Check the id with `nself ci artifacts list`; an artifact you may not see reads as not found"})
}
