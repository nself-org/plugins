package remote

// Purpose: the coordinator's side of the release inputs: read one finished run from the store
// and list exactly what a release of it is built from, for a caller of a given role.
// Inputs:  Deps (run resolver, run store view, blobs), a run id and the caller's role.
// Outputs: Inputs and the run's owning project.
// Constraints: only reads; the run is found by its exact id (no prefix, no "latest"); the
// project comes from the run's intake row, never from the caller; only committed, present
// records of the passed attempts of release jobs are listed, of release kinds and SBOMs, and
// only those authz.ArtifactVisibility shows the caller; the list is the whitelist the blob
// route serves from, so a digest outside it (another project, a cache realm, a log) is
// "not found"; no key, secret value or resolver is reachable from here.

import (
	"context"
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Backend is what the handlers read from the coordinator's store.
type Backend interface {
	signing.Source
	AttemptCoordinator(ctx context.Context, attemptID string) (string, error)
}

// Runs resolves a run to its owning project (clientapi.StoreBackend).
type Runs interface {
	Get(ctx context.Context, ref string) (clientapi.RunItem, bool, error)
}

// Deps are what the two handlers need; the coordinator builds them when it starts.
type Deps struct {
	Runs  Runs
	Store Backend
	Blobs signing.Blobs
}

// errNotFound is the one refusal for a run or blob the caller may not see or that is absent.
var errNotFound = fail(records.CodeNotFound, "not_found", "no such artifact")

// resolve returns the project that owns run, or errNotFound.
func (d Deps) resolve(ctx context.Context, run string) (string, error) {
	it, ok, err := d.Runs.Get(ctx, run)
	if err != nil {
		return "", err
	}
	if !ok || it.ID != run || it.Project == "" {
		return "", errNotFound
	}
	// A run that has not ended has no release, and its project is only a guess by revision (D-1338): evidence is
	// recorded a moment before the pipeline is finished, so an unfinished run must never reach a project's holder.
	pipe, found, err := d.Store.PipelineByID(ctx, run)
	if err != nil {
		return "", err
	}
	if !found || !pipelineEnded(pipe.Status) {
		return "", errNotFound
	}
	return it.Project, nil
}

// pipelineEnded mirrors the user API: a run is over once it is no longer queued or running.
func pipelineEnded(status string) bool {
	switch status {
	case "", "queued", "running":
		return false
	}
	return true
}

func sbomOrRelease(k records.Kind) bool {
	switch k {
	case records.KindLog, records.KindCoverage, records.KindBench:
		return false
	}
	return true
}

// collect builds the release inputs of run as the caller (role) may see them.
func (d Deps) collect(ctx context.Context, run string, role authz.Role) (Inputs, error) {
	pipe, ok, err := d.Store.PipelineByID(ctx, run)
	if err != nil {
		return Inputs{}, err
	}
	if !ok {
		return Inputs{}, errNotFound
	}
	jobs, err := d.Store.PipelineJobs(ctx, run)
	if err != nil {
		return Inputs{}, err
	}
	attempts, err := d.Store.PipelineAttempts(ctx, run)
	if err != nil {
		return Inputs{}, err
	}
	body, err := d.evidence(ctx, pipe)
	if err != nil {
		return Inputs{}, err
	}
	in := Inputs{Schema: Schema, Run: run, Evidence: body,
		Pipeline: Pipeline{ID: pipe.ID, Project: pipe.Project, Revision: pipe.Revision, InputDigest: pipe.InputDigest}}
	releaseJobs := map[string]bool{}
	for _, j := range jobs {
		in.Jobs = append(in.Jobs, Job{ID: j.ID, Name: j.Name, Kind: j.Kind})
		releaseJobs[j.ID] = j.Kind == "release"
	}
	passed := map[string]bool{}
	for _, a := range attempts {
		w := Attempt{ID: a.ID, Job: a.JobID, N: a.N, State: a.State}
		if a.State == "passed" && releaseJobs[a.JobID] {
			passed[a.ID] = true
			if w.Coordinator, err = d.Store.AttemptCoordinator(ctx, a.ID); err != nil {
				return Inputs{}, err
			}
		}
		in.Attempts = append(in.Attempts, w)
	}
	all, err := d.Store.ArtifactList(ctx, records.Filter{Project: pipe.Project, Run: run})
	if err != nil {
		return Inputs{}, err
	}
	if len(all) >= MaxRecords {
		return Inputs{}, fail(signing.CodeSigning, signing.ReasonSignerFailed, "the run has too many records to release")
	}
	for _, r := range all {
		if r.State != records.StateCommitted || r.Missing || r.Run != run || r.Revision != pipe.Revision || !passed[r.Attempt] || !sbomOrRelease(r.Kind) {
			continue
		}
		if authz.ArtifactVisibility(authz.ArtifactMeta{Project: r.Project, Visibility: r.Visibility}, authz.Viewer{Project: r.Project, Role: role}) {
			in.Records = append(in.Records, r)
		}
	}
	return in, nil
}

// evidence returns the stored run-level evidence bytes (the newest row of this run).
func (d Deps) evidence(ctx context.Context, pipe store.PipelineRow) ([]byte, error) {
	rows, err := d.Store.EvidenceByRevision(ctx, pipe.Revision, pipe.InputDigest)
	if err != nil {
		return nil, err
	}
	for _, e := range rows { // newest first
		if e.PipelineID == pipe.ID && e.AttemptID == "" {
			if !json.Valid(e.Body) {
				return nil, fail(signing.CodeSigning, signing.ReasonPayloadRejected, "the stored evidence does not parse")
			}
			return e.Body, nil
		}
	}
	return nil, errNotFound
}

// blob finds the record of digest in the caller's release inputs.
func (in Inputs) blob(digest string) (records.Record, bool) {
	for _, r := range in.Records {
		if r.SHA256 == digest {
			return r, true
		}
	}
	return records.Record{}, false
}
