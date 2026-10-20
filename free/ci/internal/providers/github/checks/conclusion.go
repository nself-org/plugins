package checks

// Purpose: the conclusion a check run carries, derived from the status that was posted and the
// published evidence, so the check run never disagrees with the status of record.
// Mapping (Ticket P7-AUTO-22): pass -> success; code failure -> failure; infra error -> neutral;
// cancelled -> cancelled; lost -> neutral.
// Constraints: pure; success needs both a success status and a passing run.

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// Conclusion is a GitHub check-run conclusion.
type Conclusion string

// The conclusions this adapter uses.
const (
	Success   Conclusion = "success"
	Failure   Conclusion = "failure"
	Neutral   Conclusion = "neutral"
	Cancelled Conclusion = "cancelled"
)

// ConclusionFor maps the posted status and the evidence to a conclusion. It returns "" for a
// pending status (the run has no conclusion yet).
func ConclusionFor(state report.State, ev model.Evidence) Conclusion {
	if ev.Result == "cancelled" {
		return Cancelled
	}
	switch state {
	case report.Success:
		if ev.Result == "pass" {
			return Success
		}
		// a success status over a run that did not pass cannot happen through the coordinator;
		// never report better than the evidence
		return Failure
	case report.Error:
		return Neutral
	case report.Failure:
		return failureConclusion(ev)
	}
	return ""
}

// failureConclusion looks at what failed: when nothing failed for a reason of the code under test
// (every unsuccessful required check was an infrastructure error, a lost runner or a cancel), the
// run says "neutral" or "cancelled" rather than blaming the code.
func failureConclusion(ev model.Evidence) Conclusion {
	var infra, cancelled, code int
	for _, c := range ev.Checks {
		if c.Result == "pass" || c.Result == "skip" || !c.Required && c.Result != "error" || c.AllowFailure && c.Result == "fail" {
			continue
		}
		switch {
		case c.FailureClass == "cancelled":
			cancelled++
		case c.Result == "error" || c.FailureClass == "infra" || c.FailureClass == "lost":
			infra++
		default:
			code++
		}
	}
	switch {
	case code > 0:
		return Failure
	case cancelled > 0 && infra == 0:
		return Cancelled
	case infra > 0 || cancelled > 0:
		return Neutral
	}
	return Failure
}
