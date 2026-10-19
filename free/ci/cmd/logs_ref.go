package main

// Purpose: choose the default attempt for `ci logs <job>` (D-1050): the one the
// run document's check names in log_ref, which after --confirm-flaky is the
// original failure and not the newer passing rerun.
// Inputs:  the store, a run id and a job (its name and its store id).
// Outputs: the attempt id the evidence names, or "" when it names none.
// Constraints: read-only; a run without stored evidence (still running, or
// evidence not stored) or a ref that names no attempt of the job yields "" and
// the caller falls back to the newest attempt.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// evidenceAttempt returns the attempt id check.log_ref names for the job.
func evidenceAttempt(ctx context.Context, st *store.Store, runID string, jobNames ...string) string {
	row, found, err := st.PipelineByID(ctx, runID)
	if err != nil || !found {
		return ""
	}
	docs, err := evidence.ForRun(ctx, st, row)
	if err != nil || len(docs) == 0 {
		return ""
	}
	for _, c := range docs[0].Evidence.Checks {
		if c.LogRef == nil {
			continue
		}
		for _, n := range jobNames {
			if c.JobID == n {
				return (*c.LogRef)[strings.LastIndex(*c.LogRef, "/")+1:]
			}
		}
	}
	return ""
}
