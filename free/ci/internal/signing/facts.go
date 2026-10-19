package signing

// Purpose: load, for one run, everything a payload is checked against: the
// pipeline row, the stored pipeline evidence and an index of the committed
// records whose bytes verify (Epic CACHE D12).
// Inputs:  a Source, Blobs and a run id.
// Outputs: *runFacts with Digest lookups.
// Constraints: a record counts only when it is committed, belongs to this run
// and this revision, is not marked missing, and its stored bytes still hash to
// the recorded digest at the recorded size; verification is per digest, once.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type runFacts struct {
	blobs    Blobs
	runID    string
	pipe     store.PipelineRow
	attempts []store.AttemptView
	jobs     []store.RunJob
	evidence model.Evidence
	evDigest string
	records  []records.Record
	verified map[string]error // sha256|size -> result of the byte check
}

// loadFacts reads the run. It refuses a run with a non-final attempt.
func loadFacts(ctx context.Context, src Source, blobs Blobs, runID string) (*runFacts, error) {
	if err := CheckTerminal(ctx, src, runID); err != nil {
		return nil, err
	}
	pipe, _, err := src.PipelineByID(ctx, runID)
	if err != nil {
		return nil, Failf(ReasonSignerFailed, "the run store could not be read")
	}
	f := &runFacts{blobs: blobs, runID: runID, pipe: pipe, verified: map[string]error{}}
	if f.attempts, err = src.PipelineAttempts(ctx, runID); err != nil {
		return nil, Failf(ReasonSignerFailed, "the run store could not be read")
	}
	if f.jobs, err = src.PipelineJobs(ctx, runID); err != nil {
		return nil, Failf(ReasonSignerFailed, "the run store could not be read")
	}
	if err = f.loadEvidence(ctx, src); err != nil {
		return nil, err
	}
	all, err := src.ArtifactList(ctx, records.Filter{Project: pipe.Project, Run: runID})
	if err != nil {
		return nil, Failf(ReasonSignerFailed, "the run store could not be read")
	}
	for _, r := range all {
		if r.State == records.StateCommitted && r.Run == runID && r.Revision == pipe.Revision && !r.Missing {
			f.records = append(f.records, r)
		}
	}
	return f, nil
}

// loadEvidence picks the newest run-level evidence row of this run: the one
// the coordinator recorded for it, never a job's own report.
func (f *runFacts) loadEvidence(ctx context.Context, src Source) error {
	rows, err := src.EvidenceByRevision(ctx, f.pipe.Revision, f.pipe.InputDigest)
	if err != nil {
		return Failf(ReasonSignerFailed, "the run store could not be read")
	}
	for _, e := range rows { // newest first
		if e.PipelineID != f.runID || e.AttemptID != "" {
			continue
		}
		var ev model.Evidence
		if json.Unmarshal(e.Body, &ev) != nil {
			return Failf(ReasonPayloadRejected, "the stored evidence of run %q does not parse", echo(f.runID))
		}
		sum := sha256.Sum256(e.Body)
		f.evidence, f.evDigest = ev, hex.EncodeToString(sum[:])
		return nil
	}
	return Failf(ReasonPayloadRejected, "run %q has no stored pipeline evidence", echo(f.runID))
}

// committed returns the records of the run with this digest whose bytes
// verify. size < 0 matches any size.
func (f *runFacts) committed(digest string, size int64) []records.Record {
	var out []records.Record
	for _, r := range f.records {
		if r.SHA256 == digest && (size < 0 || r.Size == size) && f.verify(r) == nil {
			out = append(out, r)
		}
	}
	return out
}

// verify re-hashes the stored bytes of a record (once per digest and size).
func (f *runFacts) verify(r records.Record) error {
	key := r.SHA256 + "|" + strconv.FormatInt(r.Size, 10)
	if err, ok := f.verified[key]; ok {
		return err
	}
	err := f.hashBlob(r)
	f.verified[key] = err
	return err
}

func (f *runFacts) hashBlob(r records.Record) error {
	realm := cas.ArtifactRealm(r.Project)
	info, err := f.blobs.Stat(realm, r.SHA256)
	if err != nil || info.Size() != r.Size {
		return Failf(ReasonDigestNotCommitted, "the stored bytes of a record are gone or changed")
	}
	rc, err := f.blobs.Open(realm, r.SHA256)
	if err != nil {
		return Failf(ReasonDigestNotCommitted, "the stored bytes of a record do not verify")
	}
	defer func() { _ = rc.Close() }()
	h := sha256.New()
	if n, err := io.Copy(h, io.LimitReader(rc, r.Size+1)); err != nil || n != r.Size || hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		return Failf(ReasonDigestNotCommitted, "the stored bytes of a record do not verify")
	}
	return nil
}
