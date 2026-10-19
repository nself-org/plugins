package flaky

// Purpose: mark the checks of a finishing run that are flaky by history
// (P7-CI-40): the same check passed in one run and failed in another on
// identical inputs, counting this run too.
// Inputs:  the store and the finished run's evidence (input digests set).
// Outputs: a Flaky note per check id; the caller attaches it to the check.
// Constraints: read-only; a marker never changes a result; at most
// AnnotateRuns recent runs are read; a store error yields no marker (the
// listing in `ci runs --flaky` is the source of truth, evidence only echoes it).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// AnnotateRuns is how many recent runs a finishing run compares itself with.
const AnnotateRuns = 50

// Annotate returns, by check id, the history flake notes for ev.
func Annotate(ctx context.Context, st *store.Store, ev model.Evidence) map[string]*model.Flaky {
	out := map[string]*model.Flaky{}
	if st == nil || ev.Pipeline.Kind == "adhoc" {
		return out
	}
	past, err := scanDocs(ctx, st, AnnotateRuns, ev.Revision)
	if err != nil {
		return out
	}
	docs := append([]Doc{{RunID: ev.RunID, Evidence: ev}}, past...)
	for _, f := range Detect(docs) {
		if f.Source != SourceHistory || f.Revision != ev.Revision {
			continue
		}
		for _, r := range f.Runs {
			if r.Run == ev.RunID {
				out[f.Check] = &model.Flaky{Source: SourceHistory, Passes: f.Passes, Fails: f.Fails}
			}
		}
	}
	return out
}
