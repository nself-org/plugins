package remote

// Purpose: the coordinator's release-inputs list: what is whitelisted for the blob route, and
// what the Snapshot answers back to the signer's checks.
// Constraints: no network, no process; an in-memory store view.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type memBackend struct {
	pipe     store.PipelineRow
	jobs     []store.RunJob
	attempts []store.AttemptView
	evidence []store.EvidenceView
	recs     []records.Record
}

func (m *memBackend) PipelineByID(_ context.Context, id string) (store.PipelineRow, bool, error) {
	return m.pipe, id == m.pipe.ID, nil
}
func (m *memBackend) PipelineJobs(context.Context, string) ([]store.RunJob, error) {
	return m.jobs, nil
}
func (m *memBackend) PipelineAttempts(context.Context, string) ([]store.AttemptView, error) {
	return m.attempts, nil
}
func (m *memBackend) EvidenceByRevision(context.Context, string, string) ([]store.EvidenceView, error) {
	return m.evidence, nil
}
func (m *memBackend) ArtifactList(_ context.Context, f records.Filter) ([]records.Record, error) {
	var out []records.Record
	for _, r := range m.recs {
		if r.Project == f.Project && r.Run == f.Run {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *memBackend) AttemptCoordinator(context.Context, string) (string, error) {
	return "coord-1", nil
}

type fixedRuns struct{ id, project string }

func (f fixedRuns) Get(_ context.Context, ref string) (clientapi.RunItem, bool, error) {
	if ref != f.id {
		return clientapi.RunItem{}, false, nil
	}
	it := clientapi.RunItem{Project: f.project}
	it.ID = f.id
	return it, true, nil
}

func rec(name string, kind records.Kind, attempt, sha string, mod func(*records.Record)) records.Record {
	r := records.Record{ID: "r-" + name, Project: tProj, Run: tRun, Attempt: attempt, Revision: tRev, Name: name, Kind: kind, SHA256: sha,
		Size: 3, State: records.StateCommitted, Visibility: authz.VisibilityProject}
	if mod != nil {
		mod(&r)
	}
	return r
}

func newMem() *memBackend {
	return &memBackend{pipe: store.PipelineRow{ID: tRun, Project: tProj, Revision: tRev, InputDigest: "in"},
		jobs:     []store.RunJob{{ID: "t", Name: "test", Kind: "test"}, {ID: "r", Name: "publish", Kind: "release"}},
		attempts: []store.AttemptView{{ID: "t.a1", JobID: "t", N: 1, State: "passed"}, {ID: "r.a1", JobID: "r", N: 1, State: "failed"}, {ID: "r.a2", JobID: "r", N: 2, State: "passed"}},
		evidence: []store.EvidenceView{{ID: "job-ev", PipelineID: tRun, AttemptID: "r.a2", Body: []byte(`{"job":true}`)}, {ID: "run-ev", PipelineID: tRun, Body: []byte(`{"run":true}`)}}}
}

func TestCollectListsOnlyTheReleaseInputs(t *testing.T) {
	m := newMem()
	m.recs = []records.Record{
		rec("app", records.KindBinary, "r.a2", "a1", nil),
		rec("app.cdx.json", records.KindSBOM, "r.a2", "a2", nil),
		rec("secret-report", records.KindReport, "r.a2", "a3", func(r *records.Record) { r.Visibility = authz.VisibilityRestricted }),
		rec("build.log", records.KindLog, "r.a2", "b1", nil),
		rec("cover", records.KindCoverage, "r.a2", "b2", nil),
		rec("bench", records.KindBench, "r.a2", "b3", nil),
		rec("from-failed-attempt", records.KindBinary, "r.a1", "c1", nil),
		rec("from-test-job", records.KindBinary, "t.a1", "c2", nil),
		rec("missing", records.KindBinary, "r.a2", "c3", func(r *records.Record) { r.Missing = true }),
		rec("staged", records.KindBinary, "r.a2", "c4", func(r *records.Record) { r.State = records.StateStaged }),
		rec("other-revision", records.KindBinary, "r.a2", "c5", func(r *records.Record) { r.Revision = "bbbb" }),
		rec("other-run", records.KindBinary, "r.a2", "c6", func(r *records.Record) { r.Run = "run-2" }),
	}
	d := Deps{Store: m}
	names := func(role authz.Role) map[string]bool {
		in, err := d.collect(context.Background(), tRun, role)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, r := range in.Records {
			out[r.Name] = true
		}
		if string(in.Evidence) != `{"run":true}` || in.Schema != Schema || in.Pipeline.Project != tProj {
			t.Fatalf("inputs %+v", in)
		}
		return out
	}
	rel := names(authz.RoleRelease)
	if len(rel) != 3 || !rel["app"] || !rel["app.cdx.json"] || !rel["secret-report"] {
		t.Fatalf("release role sees %v", rel)
	}
	if mem := names(authz.RoleMember); len(mem) != 2 || mem["secret-report"] {
		t.Fatalf("member role sees %v", mem)
	}
	in, _ := d.collect(context.Background(), tRun, authz.RoleRelease)
	if _, ok := in.blob("a1"); !ok {
		t.Fatal("a listed digest is not served")
	}
	for _, d := range []string{"b1", "c1", "c2", "c3", "c4", "c5", "c6", "", "unknown"} {
		if _, ok := in.blob(d); ok {
			t.Fatalf("digest %q must not be served", d)
		}
	}
	if len(in.Attempts) != 3 || in.Attempts[2].Coordinator != "coord-1" || in.Attempts[1].Coordinator != "" {
		t.Fatalf("attempts %+v", in.Attempts)
	}
}

func TestCollectRefusesUnknownRunMissingEvidenceAndHugeLists(t *testing.T) {
	m := newMem()
	d := Deps{Store: m}
	if _, err := d.collect(context.Background(), "nope", authz.RoleRelease); err == nil {
		t.Fatal("an unknown run was collected")
	}
	m.evidence = m.evidence[:1] // only a job's evidence
	if _, err := d.collect(context.Background(), tRun, authz.RoleRelease); err == nil {
		t.Fatal("a run without run-level evidence was collected")
	}
	m = newMem()
	for i := 0; i < MaxRecords; i++ {
		m.recs = append(m.recs, rec("f", records.KindBinary, "r.a2", "d", nil))
	}
	if _, err := (Deps{Store: m}).collect(context.Background(), tRun, authz.RoleRelease); err == nil {
		t.Fatal("a listing at the store cap was accepted")
	}
}

func TestResolveUsesTheRunsOwnProject(t *testing.T) {
	d := Deps{Runs: fixedRuns{id: tRun, project: "acme/app"}}
	if p, err := d.resolve(context.Background(), tRun); err != nil || p != "acme/app" {
		t.Fatalf("%q %v", p, err)
	}
	for _, d := range []Deps{{Runs: fixedRuns{id: tRun, project: ""}}, {Runs: fixedRuns{id: "other", project: "acme/app"}}} {
		if _, err := d.resolve(context.Background(), tRun); err != errNotFound {
			t.Fatalf("want not found, got %v", err)
		}
	}
}

func TestSnapshotAnswersLikeAStore(t *testing.T) {
	in := goodInputs([]byte("abc"))
	in.Attempts = append(in.Attempts, Attempt{ID: "j1.a2", Job: "j1", N: 2, State: "passed", Coordinator: "coord-9"})
	s := NewSnapshot(in)
	ctx := context.Background()
	if _, ok, _ := s.PipelineByID(ctx, "other"); ok {
		t.Fatal("another run answered")
	}
	if rows, _ := s.EvidenceByRevision(ctx, tRev, "wrong"); rows != nil {
		t.Fatal("evidence for another input digest")
	}
	rows, _ := s.EvidenceByRevision(ctx, tRev, "in")
	rows[0].Body[0] = 'X' // a copy: the snapshot is not changed by a caller
	if rows2, _ := s.EvidenceByRevision(ctx, tRev, "in"); rows2[0].Body[0] != '{' {
		t.Fatal("the snapshot's evidence was changed through a returned slice")
	}
	if list, _ := s.ArtifactList(ctx, records.Filter{Project: tProj, Run: tRun, Kind: records.KindSBOM}); len(list) != 0 {
		t.Fatalf("kind filter: %v", list)
	}
	if list, _ := s.ArtifactList(ctx, records.Filter{Project: tProj, Run: tRun, Name: in.Records[0].Name}); len(list) != 1 {
		t.Fatalf("name filter: %v", list)
	}
	if c, _ := s.AttemptCoordinator(ctx, "j1.a2"); c != "coord-9" {
		t.Fatalf("coordinator %q", c)
	}
	// a published row is never replaced by a failure
	_ = s.RecordRelease(ctx, store.ReleaseRow{RunID: tRun, Project: tProj, Revision: tRev, State: store.ReleasePublished, ManifestSHA256: hexSum([]byte("m")), KeyID: "k"})
	_ = s.RecordRelease(ctx, store.ReleaseRow{RunID: tRun, Project: tProj, Revision: tRev, State: store.ReleaseFailed, Reason: "x"})
	if row, _, _ := s.ReleaseFor(ctx, tRun); row.State != store.ReleasePublished {
		t.Fatalf("ledger %+v", row)
	}
}
