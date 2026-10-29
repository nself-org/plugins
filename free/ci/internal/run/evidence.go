package run

// Purpose: the coordinator-recorded evidence of a run: one check per job, the
// verdict, the counts and the binding. P7-CI-38 owns the full evidence
// pipeline (signing, schema validation, `ci evidence`); this is the document
// the status description and the AfterRun hooks need now.
// Inputs:  job outcomes, the revision and the plan.
// Outputs: a model.Evidence and its sha256 digest.
// Constraints: evidence is unbound (never success) when the worktree is
// dirty or there is no commit; excerpts are bounded; no secret is read.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"runtime"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/diagnose"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const maxExcerpt = diagnose.ExcerptMax // contract:ci.evidence: excerpt <= 4 KiB

// evidenceInput gathers what BuildEvidence needs.
type evidenceInput struct {
	RunID, Project, Revision, ConfigDigest, PolicyDigest, Source string
	Dirty, Cancelled                                             bool
	Pipeline                                                     model.PipelineRef
	Selection                                                    model.Selection
	Trigger                                                      model.Trigger
	Jobs                                                         []JobOutcome
	Attempts                                                     []model.AttemptEvidence // runner and timings per attempt (P7-AUTO-21)
	Errors                                                       []model.EvidenceError
	Start, End                                                   time.Time
}

func checkResult(o JobOutcome) model.CheckResult {
	switch o.State {
	case StatePassed:
		return "pass"
	case StateFailed:
		// Only the code under test can `fail`: a broken runner, a lost lease and
		// a stopped job say nothing about it (AUTO D6, D-0285).
		switch {
		case o.Code != "", o.FailureClass == "infra", o.FailureClass == "timeout", o.FailureClass == "lost":
			return "error"
		}
		return "fail"
	default:
		return "skip"
	}
}

// runResult is the verdict of the checks with infrastructure told apart: a run
// that is red only because a required check ended `error` (a full disk, an
// unreachable registry, a lost runner, a timeout) is `error`, so the posted
// status is not `failure`; any required check that really failed, or a
// required check that was skipped on its own, keeps it `fail`.
func runResult(checks []model.Check) model.Result {
	v := model.Verdict(checks)
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

// BuildEvidence assembles the ci.evidence/v1 document of a run.
func buildEvidence(in evidenceInput) (model.Evidence, string, error) {
	sort.Slice(in.Jobs, func(i, j int) bool { return in.Jobs[i].ID < in.Jobs[j].ID })
	e := model.Evidence{
		Schema: "ci.evidence/v1", Revision: in.Revision, RepoID: shortDigest(in.Project), RunID: in.RunID,
		Pipeline: in.Pipeline, Trigger: in.Trigger, SourceTrust: model.TrustClass(firstNonEmpty(in.Source, "owner")), PolicyDigest: in.PolicyDigest,
		InputDigests: model.InputDigests{Config: in.ConfigDigest, Lockfiles: []model.DigestRef{}, Generated: []model.DigestRef{}},
		Runner:       model.RunnerRef{ID: "local", Provider: "local", Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process"},
		Producer:     model.Producer{Kind: "coordinator-recorded", Version: "1"},
		ToolVersions: map[string]string{}, Artifacts: []model.Artifact{}, Retries: []model.Retry{},
		Selection: in.Selection, Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
	e.WorktreeState, e.Binding = "clean", model.Binding{State: "bound"}
	if in.Dirty || in.Revision == "" {
		reason := model.BindingReason("dirty")
		e.WorktreeState, e.Binding = "dirty", model.Binding{State: "unbound", Reason: &reason}
		why := "the worktree is not the named commit"
		e.Freshness = model.Freshness{State: "unknown", Reason: &why} // an unbound document is never current (schema)
	}
	for _, o := range in.Jobs {
		c := model.Check{ID: firstNonEmpty(o.Check, o.ID), JobID: o.ID, Kind: string(o.Kind), Tool: o.Tool, Required: o.Required,
			Substantive: o.Substantive, AllowFailure: o.AllowFailure, Result: checkResult(o), Reason: o.Reason, FailureClass: o.FailureClass}
		if o.State == StateFailed || o.Code != "" {
			c.Excerpt = diagnose.Excerpt(o.Tail, maxExcerpt) // the tail already passed the exec funnel's redactor (D-0948, P7-TRUST-19)
			attachDiagnosis(&c, o)
		}
		c.Flaky = o.Flaky
		c.Findings = o.Findings
		if o.Excerpt != "" {
			c.Excerpt = diagnose.Excerpt(o.Excerpt, maxExcerpt) // built from redacted output by an OutcomeHook
		}
		e.Checks = append(e.Checks, c)
		switch c.Result {
		case "pass":
			e.Counts.Pass++
		case "fail":
			e.Counts.Fail++
		case "skip":
			e.Counts.Skip++
		case "error":
			e.Counts.Error++
		}
		e.Retries = append(e.Retries, o.Retries...)
	}
	e.Counts.Total = len(e.Checks)
	e.Attempts, e.Errors = sortedAttempts(in.Attempts), sortedErrors(in.Errors)
	e.Result = runResult(e.Checks)
	if in.Cancelled {
		e.Result = "cancelled"
	}
	started, finished := in.Start.UTC().Format(time.RFC3339), in.End.UTC().Format(time.RFC3339)
	ms := in.End.Sub(in.Start).Milliseconds()
	e.Timings = model.Timings{QueuedAt: &started, StartedAt: &started, FinishedAt: &finished, DurationMS: &ms}
	digest, err := evidenceDigest(e)
	return e, digest, err
}

// evidenceDigest is the sha256 of the document as it is stored.
func evidenceDigest(e model.Evidence) (string, error) {
	body, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func shortDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// beginEvidence reads the inputs and the binding before any job can change
// them. A nil builder or a panicking one yields nil (no evidence document).
func (c *Coordinator) beginEvidence(ctx context.Context, h Hooks, rev Revision) (begin any) {
	if h.Evidence != nil {
		_ = guard("evidence builder", func() error { begin = h.Evidence.Begin(ctx, c.Opts.Root, rev, c.capture()); return nil })
	}
	return begin
}

// attachDiagnosis copies the diagnostics of a failed job onto its check; a
// failure the coordinator classified without a signature gets its class default.
func attachDiagnosis(c *model.Check, o JobOutcome) {
	c.Exit, c.Diagnosis, c.Remediation = o.Exit, o.Diagnosis, o.Remediation
	if c.Diagnosis == "" {
		d := diagnose.ForClass(o.FailureClass)
		c.Diagnosis, c.Remediation = d.Signature, d.Remediation
	}
	if o.LogRef != "" {
		ref := o.LogRef
		c.LogRef = &ref
	}
}

// sortedAttempts copies the attempt records in a stable order (job, attempt);
// none yields nil so the additive key is absent, as in an older document.
func sortedAttempts(in []model.AttemptEvidence) []model.AttemptEvidence {
	if len(in) == 0 {
		return nil
	}
	out := append([]model.AttemptEvidence(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].JobID != out[j].JobID {
			return out[i].JobID < out[j].JobID
		}
		return out[i].Attempt < out[j].Attempt
	})
	return out
}

// sortedErrors copies the coded errors in a stable order (job, attempt, code).
func sortedErrors(in []model.EvidenceError) []model.EvidenceError {
	if len(in) == 0 {
		return nil
	}
	out := append([]model.EvidenceError(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.JobID != b.JobID {
			return a.JobID < b.JobID
		}
		if a.Attempt != b.Attempt {
			return a.Attempt < b.Attempt
		}
		return a.Code < b.Code
	})
	return out
}
