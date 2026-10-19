// Package truth is the `truth` check kind of the check catalogue (P7-CI-34
// registers kinds, P7-CI-08 adds this one): banned literals, retired terms and
// dead hosts, read from repo-relative rule files, with no network (Epic CI D21).
package truth

// Purpose: define the check, load rule files and report one model.Check per
// rule kind, so `nself ci` shows which kind of truth broke.
// Inputs:  a scan root, the rule files, the directory the rule files live in.
// Outputs: gates.CheckDef (Def, Register), []model.Check (Run).
// Constraints: the engine holds no nself-org rule data (the rules are data in
// .github/truth/org.yaml, never shipped in the plugin tarball); a rule file
// error is E644 on all three checks (result error, failure class code); an
// excerpt never carries matched text of a dead-host rule, only the token's hash
// prefix; the kind is `truth`, required and substantive.

import (
	"path/filepath"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CheckID is the catalogue id of the truth check and the job it implies.
const CheckID = "truth"

// Kind is the check kind recorded in evidence.
const Kind = "truth"

// ImpliedBy is the config key whose presence plans the check as a job.
const ImpliedBy = "checks.truth.rules"

const excerptLimit = 1024

// Def is the catalogue row. The row is never planned from detector facts and
// its argv is a placeholder: the check runs through Run (or the runner in
// run.go), which needs no external tool.
func Def() gates.CheckDef {
	return gates.CheckDef{
		ID: CheckID, Kind: Kind, Tool: "nself-ci",
		Argv:            []string{"nself-ci", "truth"},
		VersionProbe:    []string{"nself-ci", "--version"},
		RequiredDefault: true, Substantive: true, Path: "fast",
		Remediation: "Fix the rule file and key named in the E644 message, or remove the finding it reports",
		ImpliedBy:   ImpliedBy,
	}
}

// Register adds the truth check to a catalogue.
func Register(r *gates.Registry) error { return r.Register(Def()) }

// CheckIDOf is the evidence id of the check for one rule kind.
func CheckIDOf(kind string) string { return CheckID + "." + kind }

// Run scans root with the rule files (paths relative to rulesRoot, which is
// root when empty) and returns one check per rule kind, in Kinds order.
func Run(root string, ruleFiles []string, rulesRoot string) []model.Check {
	checks, _ := RunFull(root, ruleFiles, rulesRoot)
	return checks
}

// RunFull is Run plus every finding (the checks' excerpts are clipped).
func RunFull(root string, ruleFiles []string, rulesRoot string) ([]model.Check, []Finding) {
	if rulesRoot == "" {
		rulesRoot = root
	}
	rs, err := Load(rulesRoot, ruleFiles)
	if err != nil {
		return allChecks(func(c model.Check) model.Check {
			c.Result, c.FailureClass, c.Excerpt = "error", "code", clip(err.Error())
			return c
		}), nil
	}
	res, err := Scan(root, rs, selfFiles(root, rulesRoot, rs.Files))
	if err != nil {
		return allChecks(func(c model.Check) model.Check {
			c.Result, c.FailureClass, c.Excerpt = "error", "infra", clip("cannot scan "+filepath.Base(root)+": "+err.Error())
			return c
		}), nil
	}
	return allChecks(func(c model.Check) model.Check {
		kind := c.ID[len(CheckID)+1:]
		var found []Finding
		rules := 0
		for _, f := range res.Findings {
			if f.Kind == kind {
				found = append(found, f)
			}
		}
		for _, r := range rs.Rules {
			if r.Kind == kind {
				rules++
			}
		}
		c.Findings = len(found)
		if len(found) > 0 {
			c.Result, c.FailureClass, c.Excerpt = "fail", "code", clip(Excerpt(found, 12))
			return c
		}
		c.Result = "pass"
		c.Excerpt = summary(rules, res)
		return c
	}), res.Findings
}

func allChecks(fill func(model.Check) model.Check) []model.Check {
	out := make([]model.Check, 0, len(Kinds))
	for _, k := range Kinds {
		out = append(out, fill(model.Check{ID: CheckIDOf(k), JobID: CheckID, Kind: Kind, Tool: "nself-ci", Required: true, Substantive: true}))
	}
	return out
}

func summary(rules int, res Result) string {
	return pluralise(rules, "rule") + ", " + pluralise(res.Files, "file") + " scanned, " + pluralise(res.Allowed, "allowlisted hit")
}

func pluralise(n int, noun string) string {
	s := noun
	if n != 1 {
		s += "s"
	}
	return strconv.Itoa(n) + " " + s
}

func clip(s string) string {
	if len(s) <= excerptLimit {
		return s
	}
	return s[:excerptLimit-3] + "..."
}

// selfFiles are the rule files that live inside the scanned tree.
func selfFiles(root, rulesRoot string, files []string) []string {
	a, err1 := filepath.Abs(root)
	b, err2 := filepath.Abs(rulesRoot)
	if err1 != nil || err2 != nil || a != b {
		return nil
	}
	return files
}
