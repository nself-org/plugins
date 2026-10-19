package evidence

// Purpose: find the evidence this machine recorded: for a run, or for a
// revision (the answer `ci evidence verify <rev>` gives without running).
// Inputs:  the coordinator's store. Outputs: decoded, strictly verified
// documents, newest first.
// Constraints: read-only; only run-level evidence of default and named
// pipelines is returned for a revision (adhoc and drill evidence never gates);
// a stored body that fails attest.Verify is skipped, never trusted.

import (
	"context"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Stored is one evidence document as the store holds it.
type Stored struct {
	ID, PipelineID string
	Created        int64
	Body           []byte
	Evidence       model.Evidence
}

func decodeRows(rows []store.EvidenceView, pipelineID string, seen map[string]bool) []Stored {
	var out []Stored
	for _, r := range rows {
		if r.AttemptID != "" || r.PipelineID != pipelineID || seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if ev, err := attest.Verify(r.Body); err == nil {
			out = append(out, Stored{ID: r.ID, PipelineID: r.PipelineID, Created: r.CreatedAt, Body: r.Body, Evidence: ev})
		}
	}
	return out
}

// ForRun returns the run-level evidence of one run, newest first.
func ForRun(ctx context.Context, st *store.Store, row store.PipelineRow) ([]Stored, error) {
	rows, err := st.EvidenceByRevision(ctx, row.Revision, row.InputDigest)
	if err != nil {
		return nil, err
	}
	out := decodeRows(rows, row.ID, map[string]bool{})
	sortStored(out)
	return out, nil
}

// ForRevision returns the run-level evidence of default and named pipelines
// recorded for exactly rev, newest first.
func ForRevision(ctx context.Context, st *store.Store, rev string) ([]Stored, error) {
	pipes, err := st.PipelinesByRevision(ctx, rev)
	if err != nil {
		return nil, err
	}
	var out []Stored
	seen := map[string]bool{}
	for _, p := range pipes {
		rows, err := st.EvidenceByRevision(ctx, rev, p.InputDigest)
		if err != nil {
			return nil, err
		}
		for _, s := range decodeRows(rows, p.ID, seen) {
			if k := s.Evidence.Pipeline.Kind; k == "default" || k == "named" {
				out = append(out, s)
			}
		}
	}
	sortStored(out)
	return out, nil
}

func sortStored(l []Stored) {
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].Created != l[j].Created {
			return l[i].Created > l[j].Created
		}
		return l[i].ID > l[j].ID
	})
}

// AnyForRevision returns every run-level document recorded for rev, whatever
// the pipeline kind, newest first (`ci evidence <rev>` shows adhoc runs too).
func AnyForRevision(ctx context.Context, st *store.Store, rev string) ([]Stored, error) {
	pipes, err := st.PipelinesByRevision(ctx, rev)
	if err != nil {
		return nil, err
	}
	var out []Stored
	seen := map[string]bool{}
	for _, p := range pipes {
		rows, err := st.EvidenceByRevision(ctx, rev, p.InputDigest)
		if err != nil {
			return nil, err
		}
		out = append(out, decodeRows(rows, p.ID, seen)...)
	}
	sortStored(out)
	return out, nil
}
