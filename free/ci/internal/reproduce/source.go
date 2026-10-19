package reproduce

// Purpose: find the stored evidence a reproduction starts from: the run, the
// job's checks and the newest attempt document of that job.
// Inputs:  the store, a run reference ("" or "latest", an id or a unique id
// prefix) and a job id (the planned job id, as in `ci --job`).
// Outputs: a Source holding the recorded facts of that job.
// Constraints: read-only; a document that fails attest.Verify is never used;
// without a run reference the newest runs are searched for the job; the
// result and failure class are those of the job's checks alone.

import (
	"context"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// searchRuns bounds how many recent runs a reference-less lookup reads.
const searchRuns = 50

// Outcome is how a job ended, from its checks.
type Outcome struct {
	RunID        string             `json:"run_id"`
	AttemptID    string             `json:"attempt_id,omitempty"`
	EvidenceID   string             `json:"evidence_id,omitempty"`
	Result       string             `json:"result"`
	FailureClass model.FailureClass `json:"failure_class,omitempty"`
}

// Source is what the evidence recorded about one job.
type Source struct {
	Run      store.PipelineRow
	Doc      model.Evidence // the run-level document
	JobID    string
	Checks   []model.Check // the job's checks (from the attempt document when it exists)
	Original Outcome
}

// OutcomeOf reads the result and failure class of one job from a document.
func OutcomeOf(ev model.Evidence, jobID string) (string, model.FailureClass, bool) {
	var checks []model.Check
	for _, c := range ev.Checks {
		if c.JobID == jobID {
			checks = append(checks, c)
		}
	}
	if len(checks) == 0 {
		return "", "", false
	}
	return outcomeOfChecks(checks)
}

func outcomeOfChecks(checks []model.Check) (string, model.FailureClass, bool) {
	result, class := "skip", model.FailureClass("")
	for _, c := range checks {
		switch c.Result {
		case "fail", "error":
			// A failed check outranks an errored one, and the class comes from
			// the check that decided the result (the first of its kind).
			switch {
			case result == "fail" && c.Result == "fail" && class == "",
				result == "error" && c.Result == "error" && class == "":
				class = c.FailureClass
			case result != "fail" && (result != "error" || c.Result == "fail"):
				result, class = string(c.Result), c.FailureClass
			}
		case "pass":
			if result == "skip" {
				result = "pass"
			}
		}
	}
	return result, class, len(checks) > 0
}

// Locate finds the evidence of job in the run runRef names.
func Locate(ctx context.Context, st *store.Store, runRef, jobID string) (Source, error) {
	if st == nil {
		return Source{}, fail(evidence.CodeNone, "this machine has no run history")
	}
	if jobID == "" {
		return Source{}, fail(evidence.CodeUsage, "name the job to reproduce")
	}
	rows, err := candidates(ctx, st, runRef)
	if err != nil {
		return Source{}, err
	}
	for _, row := range rows {
		src, ok, err := fromRun(ctx, st, row, jobID)
		if err != nil {
			return Source{}, err
		}
		if ok {
			return src, nil
		}
	}
	if runRef != "" && runRef != "latest" {
		return Source{}, fail(evidence.CodeNone, "run %s has no evidence for job %q", rows[0].ID, jobID)
	}
	return Source{}, fail(evidence.CodeNone, "none of the newest runs holds evidence for job %q; list runs with `nself ci runs list`", jobID)
}

func candidates(ctx context.Context, st *store.Store, runRef string) ([]store.PipelineRow, error) {
	if runRef != "" && runRef != "latest" {
		row, err := cirun.ResolveRun(ctx, st, runRef)
		if err != nil {
			return nil, err
		}
		return []store.PipelineRow{row}, nil
	}
	page, err := st.History(ctx, 0, searchRuns)
	if err != nil {
		return nil, err
	}
	if len(page.Pipelines) == 0 {
		return nil, fail(evidence.CodeNone, "no runs recorded yet")
	}
	return page.Pipelines, nil
}

func fromRun(ctx context.Context, st *store.Store, row store.PipelineRow, jobID string) (Source, bool, error) {
	docs, err := evidence.ForRun(ctx, st, row)
	if err != nil || len(docs) == 0 {
		return Source{}, false, err
	}
	run := docs[0]
	if _, _, ok := OutcomeOf(run.Evidence, jobID); !ok {
		return Source{}, false, nil
	}
	src := Source{Run: row, Doc: run.Evidence, JobID: jobID, Original: Outcome{RunID: row.ID, EvidenceID: run.ID}}
	for _, c := range run.Evidence.Checks {
		if c.JobID == jobID {
			src.Checks = append(src.Checks, c)
		}
	}
	if att, ok, err := newestAttempt(ctx, st, row, jobID); err != nil {
		return Source{}, false, err
	} else if ok {
		src.Original.AttemptID, src.Original.EvidenceID = att.Evidence.Attempt.ID, att.ID
		src.Checks = nil
		for _, c := range att.Evidence.Checks {
			if c.JobID == jobID {
				src.Checks = append(src.Checks, c)
			}
		}
	}
	src.Original.Result, src.Original.FailureClass, _ = outcomeOfChecks(src.Checks)
	return src, true, nil
}

// newestAttempt returns the attempt document of job with the highest attempt number.
func newestAttempt(ctx context.Context, st *store.Store, row store.PipelineRow, jobID string) (evidence.Stored, bool, error) {
	views, err := st.EvidenceByRevision(ctx, row.Revision, row.InputDigest)
	if err != nil {
		return evidence.Stored{}, false, err
	}
	var found []evidence.Stored
	for _, v := range views {
		if v.AttemptID == "" || v.PipelineID != row.ID {
			continue
		}
		ev, err := attest.Verify(v.Body)
		if err != nil || ev.Job == nil || *ev.Job != jobID || ev.Attempt == nil {
			continue
		}
		found = append(found, evidence.Stored{ID: v.ID, PipelineID: v.PipelineID, Created: v.CreatedAt, Body: v.Body, Evidence: ev})
	}
	if len(found) == 0 {
		return evidence.Stored{}, false, nil
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].Evidence.Attempt.N > found[j].Evidence.Attempt.N })
	return found[0], true, nil
}
