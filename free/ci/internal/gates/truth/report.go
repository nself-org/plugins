package truth

// Purpose: print the checks and findings of a `check` run, as text or JSON.
// Inputs:  the checks, every finding, the output writer.
// Outputs: the process exit code (0 pass, 1 findings, 2 error).
// Constraints: the text form has one line per finding,
// `truth: <rule-id> <file>:<line>: <match>`, then the rule's message indented;
// a dead-host finding prints a token hash prefix, never the host; the JSON form
// is one document with `data.result` (pass, fail, error), per-check results and
// the findings.

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// matchText is what a finding prints as its match.
func matchText(f Finding) string {
	if f.Hash != "" {
		return "token sha256 " + f.Hash
	}
	return f.Match
}

// report prints the checks and returns the exit code.
func report(checks []model.Check, found []Finding, out io.Writer) int {
	code := 0
	for _, c := range checks {
		fmt.Fprintf(out, "%s %s (%d finding(s))\n", strings.ToUpper(string(c.Result)), c.ID, c.Findings)
		if c.Result == "error" {
			fmt.Fprintln(out, indent(c.Excerpt))
		}
		for _, f := range found {
			if CheckIDOf(f.Kind) == c.ID {
				fmt.Fprintf(out, "truth: %s %s:%d: %s\n    fix: %s\n", f.RuleID, f.Path, f.Line, matchText(f), f.Message)
			}
		}
		switch {
		case c.Result == "error":
			code = 2
		case c.Result != "pass" && code == 0:
			code = 1
		}
	}
	return code
}

type jsonFinding struct {
	Rule    string `json:"rule"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Match   string `json:"match"`
	Message string `json:"message"`
}

type jsonCheck struct {
	ID       string `json:"id"`
	Result   string `json:"result"`
	Findings int    `json:"findings"`
	Excerpt  string `json:"excerpt,omitempty"`
}

// reportJSON prints the run as one JSON document and returns the exit code.
func reportJSON(checks []model.Check, found []Finding, out io.Writer) int {
	doc := struct {
		Data struct {
			Result   string        `json:"result"`
			Checks   []jsonCheck   `json:"checks"`
			Findings []jsonFinding `json:"findings"`
		} `json:"data"`
	}{}
	doc.Data.Result = "pass"
	for _, c := range checks {
		doc.Data.Checks = append(doc.Data.Checks, jsonCheck{ID: c.ID, Result: string(c.Result), Findings: c.Findings, Excerpt: c.Excerpt})
		switch {
		case c.Result == "error":
			doc.Data.Result = "error"
		case c.Result != "pass" && doc.Data.Result == "pass":
			doc.Data.Result = "fail"
		}
	}
	doc.Data.Findings = []jsonFinding{}
	for _, f := range found {
		doc.Data.Findings = append(doc.Data.Findings, jsonFinding{Rule: f.RuleID, File: f.Path, Line: f.Line, Match: matchText(f), Message: f.Message})
	}
	b, _ := json.Marshal(doc)
	fmt.Fprintln(out, string(b))
	switch doc.Data.Result {
	case "error":
		return 2
	case "fail":
		return 1
	}
	return 0
}
