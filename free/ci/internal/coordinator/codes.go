package coordinator

// Purpose: the error type and reason keys of the V15 coordinator (P7-NODE-10, Epic D7).
// Inputs:  none.
// Outputs: *Error with a code (E607 store, E608 idempotency, E663 deploy host: all registered by their
//          owners), a reason key and a message; ReasonOf reads the key.
// Constraints: this Ticket registers no code of its own (E664 is NODE-19's); a message holds reason text
//          and ids only, never a secret, a header value or a request body; the missing-secret refusal
//          keeps the exact v1.4 text and carries no code.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
)

// The codes this package raises; each is registered by the package that owns it.
const (
	CodeStore       = "E607"
	CodeIdempotency = "E608"
	CodeDeployHost  = deployguard.CodeDeployHost
)

// Reason keys carried by Error.Reason.
const (
	ReasonStoreUnavailable = "store_unavailable"
	ReasonNoSecret         = "no_secret"
	ReasonInvalidDelivery  = "invalid_delivery"
	ReasonInvalidConfig    = "invalid_config"
	ReasonDuplicateName    = "duplicate_name"
	ReasonKeyReused        = "key_reused"
)

// Error is a coordinator failure.
type Error struct {
	Code, Reason, Message string
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func newErr(code, reason, format string, a ...any) error {
	return &Error{Code: code, Reason: reason, Message: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the reason key of a coordinator error, or "".
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
