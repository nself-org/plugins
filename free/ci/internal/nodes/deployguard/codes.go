package deployguard

// Purpose: the one error code of the boundary (Epic D15) and its typed error.
// Inputs:  none.
// Outputs: E663 registered through model.Register; *Error carrying the code and a reason key.
// Constraints: messages hold reason text, node ids and fixture-safe host names only; never a key,
//          a token or a command's stderr.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeDeployHost is raised when a host is, or looks like, a deploy host.
const CodeDeployHost = "E663"

// Reason keys carried by Error.Reason.
const (
	ReasonMatched      = "matched"
	ReasonMarker       = "prod_marker"
	ReasonNotConfirmed = "override_not_confirmed"
	ReasonNoTTY        = "override_needs_terminal"
	ReasonSideEffect   = "side_effect_not_read"
	ReasonInventory    = "inventory_unreadable"
	ReasonNoIdentity   = "identity_unreadable"
)

func init() {
	model.Register(model.Code{ID: CodeDeployHost, Class: "usage", Summary: "host is a deploy host and is CI-ineligible",
		Fix: "Run CI on another machine; only an admin may pass --allow-deploy-host after confirming"})
}

// Error is a boundary failure.
type Error struct {
	Code, Reason, Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func newErr(reason, format string, a ...any) error {
	return &Error{Code: CodeDeployHost, Reason: reason, Message: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the reason key of a boundary error, or "".
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
