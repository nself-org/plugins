package evidence

// Purpose: list, in the run's evidence, the artifacts the release job of the run
// committed (contract:ci.evidence v1 `artifacts[{path, sha256, size, committed}]`),
// so a release gate (authz.Promotable, `ci artifacts verify`) can tell that a
// digest is a build of exactly this run.
// Inputs:  the run's artifact records (the store) and the base document.
// Outputs: ev.Artifacts, sorted by path.
// Constraints: only committed records of this run, not marked missing and
// produced by a passed release attempt count; logs, coverage, benchmarks and SBOMs are
// run products, not released files (the signer and the publisher apply the same
// rule); a store error leaves the list empty, so a release then fails closed
// at the gate (artifact.unbound) instead of claiming files the store could not
// vouch for; nothing here changes any other field.

import (
	"context"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// artifactLister is the part of the run store this file reads.
type artifactLister interface {
	ArtifactList(ctx context.Context, f records.Filter) ([]records.Record, error)
}

// releasedKind reports whether a record kind is a file a release publishes.
func releasedKind(k records.Kind) bool {
	switch k {
	case records.KindLog, records.KindCoverage, records.KindBench, records.KindSBOM:
		return false
	}
	return true
}

// bindArtifacts fills ev.Artifacts from the committed release records of the
// run's passed release attempts (an earlier attempt of a retried job, or any
// other attempt, never counts).
func bindArtifacts(ctx context.Context, st artifactLister, project, runID string, jobs []cirun.JobOutcome, ev *model.Evidence) {
	ev.Artifacts = []model.Artifact{}
	if st == nil {
		return
	}
	passed := map[string]bool{}
	for _, j := range jobs {
		if j.Kind == "release" && j.State == cirun.StatePassed && j.AttemptID != "" {
			passed[j.AttemptID] = true
		}
	}
	recs, err := st.ArtifactList(ctx, records.Filter{Project: project, Run: runID})
	if err != nil {
		return
	}
	for _, r := range recs {
		if r.State == records.StateCommitted && r.Run == runID && r.Release && !r.Missing && passed[r.Attempt] && releasedKind(r.Kind) {
			ev.Artifacts = append(ev.Artifacts, model.Artifact{Path: r.Name, SHA256: r.SHA256, Size: r.Size, Committed: true})
		}
	}
	sort.Slice(ev.Artifacts, func(i, j int) bool { return ev.Artifacts[i].Path < ev.Artifacts[j].Path })
}
