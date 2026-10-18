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

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// evidenceInput gathers what BuildEvidence needs.
type evidenceInput struct {
	RunID, Project, Revision, ConfigDigest, PolicyDigest, Source string
	Dirty, Cancelled                                             bool
	Pipeline                                                     model.PipelineRef
	Selection                                                    model.Selection
	Trigger                                                      model.Trigger
	Jobs                                                         []JobOutcome
	Start, End                                                   time.Time
}

func checkResult(o JobOutcome) model.CheckResult {
	switch o.State {
	case StatePassed:
		return "pass"
	case StateFailed:
		if o.Code != "" || o.FailureClass == "infra" {
			return "error"
		}
		return "fail"
	default:
		return "skip"
	}
}

const maxExcerpt = 2000

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
	}
	for _, o := range in.Jobs {
		c := model.Check{ID: firstNonEmpty(o.Check, o.ID), JobID: o.ID, Kind: string(o.Kind), Tool: o.Tool, Required: o.Required,
			Substantive: o.Substantive, AllowFailure: o.AllowFailure, Result: checkResult(o), Reason: o.Reason, FailureClass: o.FailureClass}
		if o.State == StateFailed || o.Code != "" {
			c.Excerpt = clip(o.Tail, maxExcerpt) // the tail is redacted by the exec funnel (D-0948)
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
		if o.N > 1 {
			e.Retries = append(e.Retries, model.Retry{JobID: o.ID, Attempt: o.N, Class: o.FailureClass})
		}
	}
	e.Counts.Total = len(e.Checks)
	e.Result = model.Verdict(e.Checks)
	if in.Cancelled {
		e.Result = "cancelled"
	}
	for _, c := range e.Checks {
		if c.Result == "error" && c.Required && e.Result == "pass" {
			e.Result = "error"
		}
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

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// beginEvidence reads the inputs and the binding before any job can change
// them. A nil builder or a panicking one yields nil (no evidence document).
func (c *Coordinator) beginEvidence(ctx context.Context, h Hooks, rev Revision) (begin any) {
	if h.Evidence != nil {
		_ = guard("evidence builder", func() error { begin = h.Evidence.Begin(ctx, c.Opts.Root, rev, c.capture()); return nil })
	}
	return begin
}
