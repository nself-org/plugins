package remote

// Purpose: Snapshot, the operator's read-only copy of one TEAM run: it answers the questions
// the signer's checks and the publication ask of a run store (signing.Source) and holds the
// release ledger of this one signing in memory (publish.Ledger), so neither the operator's
// own run store nor the coordinator's is touched.
// Inputs:  an Inputs document that passed Validate.
// Outputs: signing.Source and publish.Ledger over that document.
// Constraints: immutable after New except the in-memory ledger; every answer is a copy; the
// facts are the coordinator's claims, so nothing here is trusted beyond "what was said":
// the blobs are re-hashed (fetch.go) and the signer checks the manifest against these facts
// before it signs (signing.Handler), the verifier checks the result against the signer's key.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Snapshot is a signing.Source and a publish.Ledger over one run's release inputs.
type Snapshot struct {
	in       Inputs
	mu       sync.Mutex
	releases map[string]store.ReleaseRow
}

// NewSnapshot wraps validated inputs.
func NewSnapshot(in Inputs) *Snapshot {
	return &Snapshot{in: in, releases: map[string]store.ReleaseRow{}}
}

// Inputs returns the document the snapshot was built from.
func (s *Snapshot) Inputs() Inputs { return s.in }

// PipelineByID answers the one run of the snapshot.
func (s *Snapshot) PipelineByID(_ context.Context, id string) (store.PipelineRow, bool, error) {
	p := s.in.Pipeline
	if id != s.in.Run || p.ID != id {
		return store.PipelineRow{}, false, nil
	}
	return store.PipelineRow{ID: p.ID, Project: p.Project, Revision: p.Revision, InputDigest: p.InputDigest}, true, nil
}

// PipelineJobs lists the jobs in name order, as the store does.
func (s *Snapshot) PipelineJobs(_ context.Context, id string) ([]store.RunJob, error) {
	if id != s.in.Run {
		return nil, nil
	}
	out := make([]store.RunJob, 0, len(s.in.Jobs))
	for _, j := range s.in.Jobs {
		out = append(out, store.RunJob{ID: j.ID, Name: j.Name, Kind: j.Kind})
	}
	return out, nil
}

// PipelineAttempts lists the attempts of the run.
func (s *Snapshot) PipelineAttempts(_ context.Context, id string) ([]store.AttemptView, error) {
	if id != s.in.Run {
		return nil, nil
	}
	out := make([]store.AttemptView, 0, len(s.in.Attempts))
	for _, a := range s.in.Attempts {
		out = append(out, store.AttemptView{ID: a.ID, JobID: a.Job, N: a.N, State: a.State})
	}
	return out, nil
}

// EvidenceByRevision answers the run-level evidence row for the run's own revision and input digest.
func (s *Snapshot) EvidenceByRevision(_ context.Context, revision, inputDigest string) ([]store.EvidenceView, error) {
	p := s.in.Pipeline
	if revision != p.Revision || inputDigest != p.InputDigest {
		return nil, nil
	}
	return []store.EvidenceView{{ID: "remote-" + s.in.Run, PipelineID: s.in.Run, Revision: revision, InputDigest: inputDigest,
		Body: append([]byte(nil), s.in.Evidence...)}}, nil
}

// ArtifactList filters the records the way the store's listing does.
func (s *Snapshot) ArtifactList(_ context.Context, f records.Filter) ([]records.Record, error) {
	var out []records.Record
	for _, r := range s.in.Records {
		if (f.Project != "" && r.Project != f.Project) || (f.Run != "" && r.Run != f.Run) || (f.Attempt != "" && r.Attempt != f.Attempt) ||
			(f.Name != "" && r.Name != f.Name) || (f.Kind != "" && r.Kind != f.Kind) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// RecordRelease keeps the ledger row of this signing in memory.
func (s *Snapshot) RecordRelease(_ context.Context, r store.ReleaseRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.releases[r.RunID]; ok && prior.State == store.ReleasePublished && r.State != store.ReleasePublished {
		return nil // a published row is never replaced by a failure
	}
	s.releases[r.RunID] = r
	return nil
}

// ReleaseFor reads the in-memory ledger row.
func (s *Snapshot) ReleaseFor(_ context.Context, run string) (store.ReleaseRow, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.releases[run]
	return r, ok, nil
}

// AttemptCoordinator is the coordinator id the coordinator reported for the attempt ("" if none).
func (s *Snapshot) AttemptCoordinator(_ context.Context, attempt string) (string, error) {
	for _, a := range s.in.Attempts {
		if a.ID == attempt {
			return a.Coordinator, nil
		}
	}
	return "", nil
}
