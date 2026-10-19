package reproduce

// Purpose: tell the command which failures are refusals (exit 1: choose
// another run, job, repository or host) and which are infrastructure (exit 2).
// Constraints: the codes are those this package and the packages it reads
// through raise; an error without a code is not a refusal.

import (
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// Refusal returns the code of err when the user can fix it by choosing a
// different run, job, repository or host.
func Refusal(err error) (string, bool) {
	switch code := cirun.CodeOf(err); code {
	case CodePlatform, evidence.CodeNone, evidence.CodeUsage, evidence.CodeRejected, cirun.CodeRun, cirun.CodeOptions:
		return code, true
	}
	return "", false
}
