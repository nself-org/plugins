// Package doctor implements `nself ci doctor` (P7-CI-50, X18, R-DX-09): local
// checks with stable ids CI-D001… plus a section registry that later Epics use
// to add checks (toolchain drift, nodes, providers, isolation) without editing
// this package.
//
// Purpose: one report of what the machine and repository need for nself-ci.
// Inputs:  the repository root, the process environment and, per check, the
// output of short processes (all run through report.Capture, the exec funnel).
// Outputs: an ordered list of Check values and a process exit code.
// Constraints: offline apart from `gh auth status` and `git remote`; a missing
// tool or Docker is a warning unless a configured required job needs it, then
// a failure naming that job; no file outside the repository and the state
// directory is read.
package doctor

import (
	"encoding/json"
	"sort"
)

// Status is the outcome of one check.
type Status string

// The three outcomes (cli:internal/doctor CheckResult shape).
const (
	Pass Status = "pass"
	Warn Status = "warn"
	Fail Status = "fail"
)

// Process exit codes (P7-REG D10).
const (
	ExitClean    = 0
	ExitFailed   = 10
	ExitWarnings = 12
)

// Check is one doctor finding. ID is stable across releases.
type Check struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      Status `json:"status"`
	Message     string `json:"message"`
	Detail      string `json:"detail,omitempty"`
	Remediation string `json:"remediation"`
	Subject     string `json:"subject,omitempty"`

	// HasVerifiedAt makes the JSON carry "verified_at": the RFC 3339 time in VerifiedAt, or null when it is nil
	// (a section that reports a last-verified date, P7-TRUST-25). Left false, the field is absent.
	HasVerifiedAt bool    `json:"-"`
	VerifiedAt    *string `json:"-"`
}

type plainCheck Check

// MarshalJSON adds verified_at, null included, to a check that reports one; every other check is unchanged.
func (c Check) MarshalJSON() ([]byte, error) {
	if !c.HasVerifiedAt {
		return json.Marshal(plainCheck(c))
	}
	return json.Marshal(struct {
		plainCheck
		VerifiedAt *string `json:"verified_at"`
	}{plainCheck(c), c.VerifiedAt})
}

// ExitCode maps a list of checks to the doctor exit code: 10 when any check
// failed, 12 when only warnings were raised, 0 otherwise.
func ExitCode(checks []Check) int {
	code := ExitClean
	for _, c := range checks {
		switch c.Status {
		case Fail:
			return ExitFailed
		case Warn:
			code = ExitWarnings
		}
	}
	return code
}

// Summary counts the checks by status.
type Summary struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

// Summarise counts checks by status.
func Summarise(checks []Check) Summary {
	var s Summary
	for _, c := range checks {
		switch c.Status {
		case Pass:
			s.Pass++
		case Warn:
			s.Warn++
		case Fail:
			s.Fail++
		}
	}
	return s
}

// sortChecks orders by id, then subject, so output is stable.
func sortChecks(checks []Check) {
	sort.SliceStable(checks, func(i, j int) bool {
		if checks[i].ID != checks[j].ID {
			return checks[i].ID < checks[j].ID
		}
		return checks[i].Subject < checks[j].Subject
	})
}
