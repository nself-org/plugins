package schedpolicy

// Purpose: the one error code of this package (Epic D17) and the way every
// refusal names the offending field.
// Inputs: a policy key and a message. Outputs: an error that starts with
// E691 and a JSON pointer to the key.
// Constraints: never a panic; the pointer is the dotted key split on dots.

import (
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeInvalid is the code of an invalid scheduling policy value.
const CodeInvalid = "E691"

func init() {
	model.Register(model.Code{ID: CodeInvalid, Class: "usage", Summary: "scheduling policy value is invalid", Fix: "Use a registered sched key with a valid value: a positive whole number, a duration such as 10m, or a listed name"})
}

// pointer turns a dotted key into a JSON pointer (`/sched/limits/global`).
func pointer(key string) string { return "/" + strings.ReplaceAll(key, ".", "/") }

func fail(key, format string, args ...any) error {
	return fmt.Errorf("%s %s: %s", CodeInvalid, pointer(key), fmt.Sprintf(format, args...))
}
