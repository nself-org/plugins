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
	limit = min(limit, MaxRuns)
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
	flakes := Detect(docs)
	if flakes == nil {
		flakes = []Flake{}
	}
	return flakes, nil
}
