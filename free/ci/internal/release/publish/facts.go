package publish

// Purpose: read, for one finished run, everything the release is built from:
// the stored pipeline evidence (bytes and digest), the release job and its
// passed attempt, and the committed records of that attempt (artifacts and
// their two SBOMs).
// Inputs:  Deps.Src and a run id.
// Outputs: *facts, ErrNotRelease (nothing to publish) or an E717 refusal.
// Constraints: only reads; the run must belong to the project of its record,
// every attempt is final; only records of the passed release attempt count
// (committed, not marked missing, same run and revision); a listing at the
// store's 1000-row cap is refused (it may hide records); more than one release
// job is refused (v1 publishes exactly one); release kinds exclude logs,
// coverage, benchmarks and SBOMs (the signer applies the same rule).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// listCap is the most records the store returns for one query.
const listCap = 1000

// sbomDirs are the places an SBOM record may sit (the signer accepts the same).
var sbomDirs = []string{"", "sbom/", ".nself-ci/sbom/"}

// artifact is one released file and its two SBOM records.
type artifact struct {
	rec       records.Record
	cdx, spdx records.Record
}

type facts struct {
	pipe      store.PipelineRow
	ev        model.Evidence
	evBytes   []byte
	evDigest  string
	job       store.RunJob
	attempt   store.AttemptView
	artifacts []artifact // sorted by name
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// gather reads the facts of runID.
func gather(ctx context.Context, d Deps, runID string) (*facts, error) {
	if err := signing.CheckTerminal(ctx, d.Src, runID); err != nil {
		return nil, refuse(reasonOf(err), "%s", msgOf(err))
	}
	pipe, _, err := d.Src.PipelineByID(ctx, runID)
	if err != nil {
		return nil, refuse(signing.ReasonSignerFailed, "the run store could not be read")
	}
	f := &facts{pipe: pipe}
	if err = f.loadEvidence(ctx, d, runID); err != nil {
		return nil, err
	}
	if err = f.findJob(ctx, d, runID); err != nil {
		return nil, err
	}
	if err = f.loadRecords(ctx, d, runID); err != nil {
		return nil, err
	}
	return f, nil
}

// msgOf is the text of a signing refusal without its code prefix, or a fixed sentence.
func msgOf(err error) string {
	var se *signing.Error
	if errors.As(err, &se) {
		return se.Msg
	}
	return "the run could not be checked"
}

// reasonOf maps a signing refusal to its reason, any other error to signer_failed.
func reasonOf(err error) string {
	var se *signing.Error
	if errors.As(err, &se) {
		return se.Reason
	}
	return signing.ReasonSignerFailed
}

// loadEvidence picks the newest run-level evidence row of the run, as the
// signer does, and applies the publication trigger: result pass with a passing
// release check.
func (f *facts) loadEvidence(ctx context.Context, d Deps, runID string) error {
	rows, err := d.Src.EvidenceByRevision(ctx, f.pipe.Revision, f.pipe.InputDigest)
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "the run store could not be read")
	}
	found := false
	for _, e := range rows { // newest first
		if e.PipelineID != runID || e.AttemptID != "" {
			continue
		}
		if json.Unmarshal(e.Body, &f.ev) != nil {
			return refuse(signing.ReasonPayloadRejected, "the stored evidence does not parse")
		}
		f.evBytes, f.evDigest, found = e.Body, sha(e.Body), true
		break
	}
	if !found {
		return refuse(signing.ReasonPayloadRejected, "run %s has no stored pipeline evidence", runID)
	}
	if f.ev.RunID != runID || f.ev.Revision != f.pipe.Revision {
		return refuse(signing.ReasonPayloadRejected, "the stored evidence is not a record of this run and revision")
	}
	if f.ev.Result != "pass" {
		return ErrNotRelease
	}
	return nil
}

// findJob returns the one release job the evidence passed, and its passed attempt.
func (f *facts) findJob(ctx context.Context, d Deps, runID string) error {
	names := map[string]bool{}
	for _, c := range f.ev.Checks {
		if c.Kind == "release" {
			if c.Result != "pass" {
				return ErrNotRelease
			}
			names[c.JobID] = true
		}
	}
	switch len(names) {
	case 0:
		return ErrNotRelease
	case 1:
	default:
		return refuse(signing.ReasonPayloadRejected, "the run has %d release jobs: v1 publishes exactly one", len(names))
	}
	jobs, err := d.Src.PipelineJobs(ctx, runID)
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "the run store could not be read")
	}
	attempts, err := d.Src.PipelineAttempts(ctx, runID)
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "the run store could not be read")
	}
	for _, j := range jobs {
		// the evidence names a job by its plan id, which is the row's name
		if j.Kind != "release" || !names[j.Name] {
			continue
		}
		f.job = j
		for _, a := range attempts {
			if a.JobID == j.ID && a.State == "passed" && a.N >= f.attempt.N {
				f.attempt = a
			}
		}
		if f.attempt.ID == "" {
			return ErrNotRelease
		}
		return nil
	}
	return refuse(signing.ReasonPayloadRejected, "the evidence names a release job the run store does not hold")
}

// loadRecords collects the artifacts and their SBOMs from the passed attempt.
func (f *facts) loadRecords(ctx context.Context, d Deps, runID string) error {
	all, err := d.Src.ArtifactList(ctx, records.Filter{Project: f.pipe.Project, Run: runID})
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "the run store could not be read")
	}
	if len(all) >= listCap {
		return refuse(signing.ReasonSignerFailed, "the run has too many records to publish")
	}
	byName := map[string]records.Record{}
	var arts []records.Record
	for _, r := range all {
		if r.State != records.StateCommitted || r.Run != runID || r.Revision != f.pipe.Revision || r.Missing || r.Attempt != f.attempt.ID {
			continue
		}
		byName[r.Name] = r
		if releaseKind(r.Kind) {
			arts = append(arts, r)
		}
	}
	if len(arts) == 0 {
		return refuse(signing.ReasonPayloadRejected, "the release job committed no artifact")
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].Name < arts[j].Name })
	for _, r := range arts {
		a := artifact{rec: r}
		var ok1, ok2 bool
		a.cdx, ok1 = sbomOf(byName, r.Name+".cdx.json")
		a.spdx, ok2 = sbomOf(byName, r.Name+".spdx.json")
		if !ok1 || !ok2 {
			return refuse(signing.ReasonPayloadRejected, "artifact %q has no CycloneDX and SPDX SBOM record", r.Name)
		}
		f.artifacts = append(f.artifacts, a)
	}
	return nil
}

func sbomOf(byName map[string]records.Record, want string) (records.Record, bool) {
	for _, dir := range sbomDirs {
		if r, ok := byName[dir+want]; ok && r.Kind == records.KindSBOM {
			return r, true
		}
	}
	return records.Record{}, false
}

// releaseKind reports whether a record kind can be a release artifact.
func releaseKind(k records.Kind) bool {
	switch k {
	case records.KindLog, records.KindCoverage, records.KindBench, records.KindSBOM:
		return false
	}
	return true
}
