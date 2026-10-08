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
		if check.Required && !check.AllowFailure && check.Result != "pass" {
			return "fail"
		}
	}
	if !substantive {
		return "fail"
	}
	return "pass"
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
	p := ProjectionResult{Result: Verdict(e.Checks), Total: len(e.Checks)}
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
