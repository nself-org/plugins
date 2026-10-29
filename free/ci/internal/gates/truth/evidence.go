package truth

// Purpose: fold the per-kind checks of one repository into one check, and a
// list of those into the one ci.evidence/v1 document the truth-sweep drill
// prints (one check per public nself-org repository).
// Inputs:  per-repository check lists, a revision, a time.
// Outputs: model.Check (Fold), model.Evidence (SweepEvidence).
// Constraints: a repository that could not be cloned is an error check, never
// a skip or a pass; the document is a compact, unsigned, coordinator-recorded
// drill document; no matched text is carried (Excerpt holds paths and ids).

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// SweepEntry is the outcome for one repository: its checks, or the reason it
// could not be examined.
type SweepEntry struct {
	Repo   string
	Checks []model.Check
	Err    string // clone or scan failure; set means error
}

// Fold merges the per-kind checks of one repository into one check.
func Fold(repo string, checks []model.Check, errText string) model.Check {
	c := model.Check{ID: "truth-sweep." + repoSlug(repo), JobID: "truth-sweep", Kind: Kind, Tool: "nself-ci", Required: true, Substantive: true}
	if errText != "" {
		c.Result, c.FailureClass, c.Excerpt = "error", "infra", clip(repo+": "+errText)
		return c
	}
	var notes []string
	for _, k := range checks {
		c.Findings += k.Findings
		switch k.Result {
		case "error":
			c.Result, c.FailureClass = "error", k.FailureClass
			notes = append(notes, k.Excerpt)
		case "fail":
			if c.Result != "error" {
				c.Result, c.FailureClass = "fail", "code"
			}
			notes = append(notes, k.ID+":\n"+k.Excerpt)
		}
	}
	if c.Result == "" {
		c.Result = "pass"
		return c
	}
	c.Excerpt = clip(repo + "\n" + strings.Join(notes, "\n"))
	return c
}

// repoSlug makes a repository name safe as a check id ('.github' -> 'dot-github').
func repoSlug(repo string) string {
	if strings.HasPrefix(repo, ".") {
		return "dot-" + repo[1:]
	}
	return repo
}

// SweepEvidence builds the nested evidence document of a sweep.
func SweepEvidence(revision, rulesDigest string, entries []SweepEntry, now time.Time) model.Evidence {
	checks := make([]model.Check, 0, len(entries))
	var counts model.Counts
	for _, e := range entries {
		c := Fold(e.Repo, e.Checks, e.Err)
		checks = append(checks, c)
		switch c.Result {
		case "pass":
			counts.Pass++
		case "fail":
			counts.Fail++
		case "skip":
			counts.Skip++
		case "error":
			counts.Error++
		}
	}
	counts.Total = len(checks)
	sum := sha256.Sum256([]byte(revision + "\x00" + rulesDigest))
	stamp := now.UTC().Format(time.RFC3339)
	ev := model.Evidence{
		Schema: "ci.evidence/v1", Revision: revision, RepoID: "local:" + hex.EncodeToString(sum[:4]), RunID: "truth-sweep-" + now.UTC().Format("20060102T150405Z"),
		Pipeline: model.PipelineRef{ID: "truth-sweep", Name: "truth-sweep", Kind: "drill"}, Trigger: "local", SourceTrust: "owner", PolicyDigest: rulesDigest,
		InputDigests:  model.InputDigests{Config: rulesDigest, Lockfiles: []model.DigestRef{}, Generated: []model.DigestRef{}},
		WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		Runner:   model.RunnerRef{ID: "local", Provider: "local", Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process"},
		Producer: model.Producer{Kind: "coordinator-recorded", Version: "1"}, ToolVersions: map[string]string{},
		Timings: model.Timings{QueuedAt: &stamp, StartedAt: &stamp, FinishedAt: &stamp},
		Checks:  checks, Artifacts: []model.Artifact{}, Retries: []model.Retry{},
		Selection: model.Selection{Mode: "full", Reason: "selection.no_base", Digest: fmt.Sprintf("%x", sum[4:8])},
		Counts:    counts, Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
	ev.Result = model.Verdict(checks)
	if ev.Result == "fail" {
		for _, c := range checks {
			if c.Result == "error" {
				ev.Result = "error"
				break
			}
		}
	}
	return ev
}
