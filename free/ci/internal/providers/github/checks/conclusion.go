package checks

// Purpose: the conclusion a check run carries, derived from the status that was posted and the
// published evidence, so the check run never disagrees with the status of record.
// Mapping (Ticket P7-AUTO-22): pass -> success; code failure -> failure (status failure); infra
// error and lost -> neutral (status error); cancelled -> cancelled.
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

// ConclusionFor maps the posted status and the evidence to a conclusion. The check run follows the
// status of record and never re-derives it: the coordinator already told a run that failed in the
// code (status failure) from one that broke in the infrastructure (status error, which covers a
// lost runner and a timeout), and a rule of our own would drift from it (run/evidence.go runResult).
// The only addition is cancelled: a failure whose every unsuccessful required check was cancelled.
// It returns "" for a pending status (the run has no conclusion yet).
func ConclusionFor(state report.State, ev model.Evidence) Conclusion {
	switch {
	case ev.Result == "cancelled":
		return Cancelled
	case state == report.Success && ev.Result == "pass":
		return Success
	case state == report.Success, state == report.Failure && !onlyCancelled(ev):
		// a success status over a run that did not pass cannot come out of the coordinator;
		// never report better than the evidence
		return Failure
	case state == report.Failure:
		return Cancelled
	case state == report.Error:
		return Neutral
	}
	return ""
}

// onlyCancelled says at least one required check was cancelled and no required check failed or
// ended in any other way.
func onlyCancelled(ev model.Evidence) bool {
	cancelled := 0
	for _, c := range ev.Checks {
		if !c.Required || c.Result == "pass" || c.AllowFailure && c.Result == "fail" {
			continue
		}
		if c.FailureClass != "cancelled" {
			return false
		}
		cancelled++
	}
	return cancelled > 0
}
