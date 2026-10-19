package model

import (
	"strings"
	"unicode/utf8"
)

// Verdict evaluates the gate. A required skip or error fails closed.
func Verdict(checks []Check) Result {
	substantive := false
	for _, check := range checks {
		if check.Substantive && check.Result != "skip" {
			substantive = true
		}
		if check.Required && check.Result != "pass" && (!check.AllowFailure || check.Result != "fail") {
			return "fail"
		}
	}
	if !substantive {
		return "fail"
	}
	return "pass"
}

// FailedResult is the check result of a job that ended failed. Only the code
// under test can `fail`: an executor error code, a broken runner, a timeout and
// a lost lease say nothing about it, so they are `error` (AUTO D6, D-0285).
func FailedResult(code string, failureClass FailureClass) CheckResult {
	switch {
	case code != "", failureClass == "infra", failureClass == "timeout", failureClass == "lost":
		return "error"
	}
	return "fail"
}

// RunVerdict is Verdict with infrastructure told apart: a run that is red only
// because a required check ended `error` (a full disk, an unreachable registry,
// a lost runner, a timeout) is `error`, so the posted status is not `failure`;
// any required check that really failed, or a required check that was skipped
// on its own, keeps it `fail`. Every producer of a result (the local run, the
// remote agent, attempt documents, Projection) uses this one rule.
func RunVerdict(checks []Check) Result {
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

// Projection is the compact result shown to users and legacy consumers.
type ProjectionResult struct {
	Result  Result `json:"result"`
	Passed  int    `json:"passed"`
	Total   int    `json:"total"`
	Summary string `json:"summary"`
}

// Projection derives a verdict and bounded summary from evidence checks.
func Projection(e Evidence) ProjectionResult {
	p := ProjectionResult{Result: RunVerdict(e.Checks), Total: len(e.Checks)}
	for _, check := range e.Checks {
		if check.Result == "pass" {
			p.Passed++
		}
	}
	p.Summary = string(p.Result) + ": " + countText(p.Passed) + "/" + countText(p.Total) + " checks passed"
	var allowed []string
	for _, check := range e.Checks {
		if check.AllowFailure && check.Result == "fail" {
			allowed = append(allowed, check.ID)
		}
	}
	if len(allowed) > 0 {
		p.Summary += "; allowed failures: " + strings.Join(allowed, ", ")
	}
	for utf8.RuneCountInString(p.Summary) > 140 {
		p.Summary = p.Summary[:len(p.Summary)-1]
	}
	return p
}

func countText(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
