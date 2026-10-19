package toolchain

// Purpose: the error code of the toolchain image (Epic D14, E645) and the typed
// error that carries it.
// E645 the pinned toolchain image cannot be used: its row is still `pending`,
// an --image value is not ref@digest, the digest is not pullable, or a pin
// source is invalid.
// Constraints: registered through model.Register, never a panic.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeToolchain is the one code this package raises.
const CodeToolchain = "E645"

func init() {
	model.Register(model.Code{
		ID: CodeToolchain, Class: "usage",
		Summary: "the pinned toolchain image is not available",
		Fix:     "Pass --image <ref@digest> (or set NSELF_CI_IMAGE=<ref@digest>), or run without --container; the first publish of the pinned image is a separate owner-named action",
	})
}

// Error is a toolchain failure with its code.
type Error struct{ Message string }

func (e *Error) Error() string { return CodeToolchain + ": " + e.Message }

func fail(format string, a ...any) *Error { return &Error{Message: fmt.Sprintf(format, a...)} }
