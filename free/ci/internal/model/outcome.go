package model

// CheckResultFor classifies execution failures consistently across runners.
func CheckResultFor(state string, code string, class FailureClass) CheckResult {
	switch state {
	case "passed":
		return "pass"
	case "failed":
		// Only the code under test can `fail`: a broken runner, a lost lease and
		// a stopped job say nothing about it (AUTO D6, D-0285).
		switch {
		case code != "", class == "infra", class == "timeout", class == "lost":
			return "error"
		}
		return "fail"
	default:
		return "skip"
	}
}

// RunResult is the verdict of the checks with infrastructure told apart: a run
// that is red only because a required check ended `error` (a full disk, an
// unreachable registry, a lost runner, a timeout) is `error`, so the posted
// status is not `failure`; any required check that really failed, or a
// required check that was skipped on its own, keeps it `fail`.
func RunResult(checks []Check) Result {
	v := Verdict(checks)
	if v != "fail" {
		return v
	}
	broken := false
	for _, c := range checks {
		switch {
		case !c.Required:
		case c.Result == "skip" && (c.Reason == "dep.failed" || c.Reason == "fail_fast.cancelled"):
			// a consequence of another check (it needed one that did not pass, or fail-fast stopped it): the other check decides
		case c.Result == "fail" && !c.AllowFailure, c.Result == "skip":
			return v
		case c.Result == "error":
			broken = true
		}
	}
	if broken {
		return "error"
	}
	return v
}
