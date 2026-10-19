package toolhook

// Purpose: give planned catalogue jobs the E614 classification Runner.Run gives
// its own checks (D-1026): a job that failed because the environment lacks
// pytest or mypy ends as an infra error with the remediation, not a code failure.
// Inputs:  a finished attempt of a catalogue job (OutcomeFacts).
// Outputs: an OutcomeChange that reclassifies the failure.
// Constraints: lives apart from gates because the run package imports plan,
// whose tests import gates; registered from init, so cmd imports it for effect.

import (
	"context"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func init() { cirun.RegisterOutcome(hook{}) }

// hook is the run.OutcomeHook (check-aware) of the tool-gap checks.
type hook struct{}

func (hook) Wants(model.Job) bool { return false }

func (hook) WantsCheck(check string) bool { return gates.HasToolGap(check) }

func (hook) Outcome(_ context.Context, f cirun.OutcomeFacts) (cirun.OutcomeChange, bool) {
	tool, fix, ok := gates.Unavailable(f.Info.Check, f.ExitCode, f.Output)
	if !ok || f.Passed {
		return cirun.OutcomeChange{}, false
	}
	msg := fmt.Sprintf("E614: %s unavailable; %s", tool, fix)
	return cirun.OutcomeChange{Fail: true, FailureClass: "infra", Detail: msg, Excerpt: msg}, true
}
