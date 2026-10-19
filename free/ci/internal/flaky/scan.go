package flaky

// Purpose: read the run history from the store and hand it to Detect.
// Inputs:  the store and how many recent runs to read.
// Outputs: Flake rows, never nil.
// Constraints: read-only; a run without verifiable evidence is skipped; at
// most MaxRuns runs are read per call.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// DefaultRuns is how many recent runs `ci runs --flaky` reads (the retention default).
const DefaultRuns = 200

// MaxRuns bounds one scan.
const MaxRuns = 1000

// Scan reads up to limit recent runs and lists the flaky checks among them.
func Scan(ctx context.Context, st *store.Store, limit int) ([]Flake, error) {
	if st == nil {
		return []Flake{}, nil
	}
	if limit < 1 {
		limit = DefaultRuns
	}
	docs, err := scanDocs(ctx, st, min(limit, MaxRuns), "")
	if err != nil {
		return nil, err
	}
	flakes := Detect(docs)
	if flakes == nil {
		flakes = []Flake{}
	}
	return flakes, nil
}

// scanDocs reads the evidence of up to limit recent runs, newest first. A
// non-empty revision decodes only the runs recorded at that revision: the others
// belong to other groups (groupKey names the revision) and cannot change the
// answer, so reading them is wasted work.
func scanDocs(ctx context.Context, st *store.Store, limit int, revision string) ([]Doc, error) {
	var docs []Doc
	read := 0
	for before := int64(0); read < limit; {
		page, err := st.History(ctx, before, min(limit-read, 100))
		if err != nil {
			return nil, err
		}
		if len(page.Pipelines) == 0 {
			break
		}
		for _, row := range page.Pipelines {
			read++
			if revision != "" && row.Revision != revision {
				continue
			}
			stored, err := evidence.ForRun(ctx, st, row)
			if err != nil {
				return nil, err
			}
			if len(stored) > 0 {
				docs = append(docs, Doc{RunID: row.ID, Evidence: stored[0].Evidence})
			}
		}
		before = page.NextCreatedAt
	}
	return docs, nil
}
