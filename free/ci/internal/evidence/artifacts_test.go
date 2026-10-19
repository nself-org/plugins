package evidence

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

type fakeLister struct {
	recs []records.Record
	err  error
}

func (f fakeLister) ArtifactList(context.Context, records.Filter) ([]records.Record, error) {
	return f.recs, f.err
}

// jobs: the release job passed on attempt a-ok; another attempt of it failed.
var jobs = []cirun.JobOutcome{{ID: "build", Kind: "test", State: cirun.StatePassed, AttemptID: "a-build"}, {ID: "publish", Kind: "release", State: cirun.StatePassed, AttemptID: "a-ok"}}

func rec(name string, kind records.Kind, mut func(*records.Record)) records.Record {
	r := records.Record{Name: name, Kind: kind, SHA256: name + "-digest", Size: 3, Run: "run-1", Attempt: "a-ok", State: records.StateCommitted, Release: true}
	if mut != nil {
		mut(&r)
	}
	return r
}

// The evidence lists the committed release artifacts of the run, sorted, and
// nothing else: so a release gate can bind a digest to exactly this run.
func TestBindArtifactsListsCommittedReleaseFiles(t *testing.T) {
	st := fakeLister{recs: []records.Record{
		rec("zeta", records.KindBinary, nil), rec("alpha", records.KindArchive, nil),
		rec("alpha.cdx.json", records.KindSBOM, nil), rec("run.log", records.KindLog, nil), rec("cov", records.KindCoverage, nil),
		rec("staged", records.KindBinary, func(r *records.Record) { r.State = records.StateStaged }),
		rec("gone", records.KindBinary, func(r *records.Record) { r.Missing = true }),
		rec("not-release", records.KindBinary, func(r *records.Record) { r.Release = false }),
		rec("other-run", records.KindBinary, func(r *records.Record) { r.Run = "run-2" }),
		rec("old-attempt", records.KindBinary, func(r *records.Record) { r.Attempt = "a-failed" }),
		rec("build-output", records.KindBinary, func(r *records.Record) { r.Attempt = "a-build" }),
	}}
	var ev model.Evidence
	bindArtifacts(context.Background(), st, "proj", "run-1", jobs, &ev)
	if len(ev.Artifacts) != 2 || ev.Artifacts[0].Path != "alpha" || ev.Artifacts[1].Path != "zeta" || !ev.Artifacts[0].Committed || ev.Artifacts[1].SHA256 != "zeta-digest" || ev.Artifacts[1].Size != 3 {
		t.Fatalf("artifacts %+v", ev.Artifacts)
	}
	// a store failure, or no store, leaves an empty list (a release then fails closed at the gate)
	for name, l := range map[string]artifactLister{"error": fakeLister{err: errors.New("down")}, "nil": nil} {
		ev = model.Evidence{Artifacts: []model.Artifact{{Path: "stale"}}}
		bindArtifacts(context.Background(), l, "proj", "run-1", jobs, &ev)
		if ev.Artifacts == nil || len(ev.Artifacts) != 0 {
			t.Fatalf("%s: %+v", name, ev.Artifacts)
		}
	}
}
