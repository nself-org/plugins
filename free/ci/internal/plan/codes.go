package plan

// Purpose: the error codes of the planner (Epic D14, range E616-E619) and the
// typed error Build returns.
// E616 a dependency cycle or a dangling dep, naming the jobs; E617 a required
// job that depends on an allow_failure job; E618 a job that cannot be placed
// (raised at run time; the planner reports the same condition as an
// unplaceable.* reason); E619 an unknown pipeline, job or fail-fast name.
// Constraints: registered through model.Register, never a panic.

import "github.com/nself-org/plugins/free/ci/internal/model"

// The codes this package raises.
const (
	CodeCycle                = "E616"
	CodeRequiredAllowFailure = "E617"
	CodeUnplaceable          = "E618"
	CodeUnknownName          = "E619"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeCycle, Class: "usage", Summary: "job dependencies form a cycle or name a job that does not exist", Fix: "Remove the cycle or define the missing job in .nself/ci.yaml"},
		{ID: CodeRequiredAllowFailure, Class: "usage", Summary: "a required job depends on an allow_failure job", Fix: "Make the dependency required or the dependant optional"},
		{ID: CodeUnplaceable, Class: "infra", Summary: "no runner can run this job", Fix: "Install the missing tool or Docker, register a runner, or relax the job requirements"},
		{ID: CodeUnknownName, Class: "usage", Summary: "unknown pipeline, job or fail-fast scope", Fix: "Use a name defined under pipelines or jobs in .nself/ci.yaml"},
	} {
		model.Register(c)
	}
}

// Error is a planner failure with its code and the jobs it names.
type Error struct {
	Code    string
	Message string
	Jobs    []string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, msg string, jobs []string) *Error {
	return &Error{Code: code, Message: msg, Jobs: jobs}
}
