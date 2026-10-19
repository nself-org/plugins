package signing

// Purpose: load, for one run, everything a payload is checked against: the
// pipeline row, its attempts, the stored pipeline evidence and, on demand,
// the committed records whose bytes verify (Epic CACHE D12).
// Inputs:  a Source, Blobs, a run id and the project root the signer serves.
// Outputs: *runFacts with record lookups by name or kind.
// Constraints: the run must belong to the signer's project; every attempt of
// the run is final (read once, so the check and the facts agree); a record
// counts only when it is committed, belongs to this run and revision, is not
// marked missing, and its stored bytes still hash to the recorded digest at
// the recorded size; a listing that hits the store's 1000-row cap is refused
// (it may hide records); byte verification happens once per digest and size.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// listCap is the most records the store returns for one query.
const listCap = 1000

type runFacts struct {
	src      Source
	blobs    Blobs
	runID    string
	pipe     store.PipelineRow
	attempts []store.AttemptView
	jobs     []store.RunJob
	evidence model.Evidence
	evDigest string
	lists    map[string][]records.Record
	verified map[string]error // sha256|size -> result of the byte check
}

// loadFacts reads the run. It refuses a run of another project and a run with
// a non-final attempt.
func loadFacts(ctx context.Context, src Source, blobs Blobs, runID, project string) (*runFacts, error) {
	pipe, attempts, err := finalAttempts(ctx, src, runID)
	if err != nil {
		return nil, err
	}
	if project != "" && filepath.Clean(pipe.Project) != filepath.Clean(project) {
		return nil, Failf(ReasonPayloadRejected, "run %q belongs to another project than the one this signer serves", echo(runID))
	}
	f := &runFacts{src: src, blobs: blobs, runID: runID, pipe: pipe, attempts: attempts, lists: map[string][]records.Record{}, verified: map[string]error{}}
	if f.jobs, err = src.PipelineJobs(ctx, runID); err != nil {
		return nil, Failf(ReasonSignerFailed, "the run store could not be read")
	}
	if err = f.loadEvidence(ctx); err != nil {
		return nil, err
	}
	return f, nil
}

// loadEvidence picks the newest run-level evidence row of this run: the one
// the coordinator recorded for it, never a job's own report.
func (f *runFacts) loadEvidence(ctx context.Context) error {
	rows, err := f.src.EvidenceByRevision(ctx, f.pipe.Revision, f.pipe.InputDigest)
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

// list returns the committed records of the run that match the filter, from
// the same revision and not marked missing.
func (f *runFacts) list(ctx context.Context, name string, kind records.Kind) ([]records.Record, error) {
	key := name + "\x00" + string(kind)
	if got, ok := f.lists[key]; ok {
		return got, nil
	}
	all, err := f.src.ArtifactList(ctx, records.Filter{Project: f.pipe.Project, Run: f.runID, Name: name, Kind: kind})
	if err != nil {
		return nil, Failf(ReasonSignerFailed, "the run store could not be read")
	}
	if len(all) >= listCap {
		return nil, Failf(ReasonSignerFailed, "the run has too many matching records for the signer to check")
	}
	var out []records.Record
	for _, r := range all {
		if r.State == records.StateCommitted && r.Run == f.runID && r.Revision == f.pipe.Revision && !r.Missing {
			out = append(out, r)
		}
	}
	f.lists[key] = out
	return out, nil
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

// match reports whether a record of the run, filtered by name and kind, has
// this digest, passes ok and verifies; a refusal error is for a store failure.
func (f *runFacts) match(ctx context.Context, name string, kind records.Kind, digest string, ok func(records.Record) bool) (bool, error) {
	recs, err := f.list(ctx, name, kind)
	if err != nil {
		return false, err
	}
	for _, r := range recs {
		if r.SHA256 == digest && ok(r) && f.verify(r) == nil {
			return true, nil
		}
	}
	return false, nil
}
