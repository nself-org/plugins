package records_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type env struct {
	t     *testing.T
	home  string
	st    *store.Store
	blobs *cas.Store
	red   *redact.Redactor
	pol   records.Policy
	ws    string
	now   time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := cas.New(home)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, home: home, st: st, blobs: blobs, red: redact.New(), ws: filepath.Join(t.TempDir(), "ws"), now: time.Now()}
	e.pol = records.Policy{MaxFile: 1 << 20, MaxTotal: 64 << 20, ProjectShare: 50, StorePath: home,
		Statfs: func(string) (uint64, uint64, error) { return 100 << 30, 200 << 30, nil }}
	if err = os.MkdirAll(e.ws, 0o755); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) collector() records.Collector {
	return records.Collector{Blobs: e.blobs, Scanner: e.red, Policy: e.pol}
}

// write puts a file in the workspace.
func (e *env) write(rel, content string) string {
	e.t.Helper()
	p := filepath.Join(e.ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return p
}

// attempt seeds a pipeline/job/attempt, moves it to running (epoch 2) and returns the id.
func (e *env) attempt(id, kind string, decls ...model.ArtifactDecl) (records.Facts, []records.Decl) {
	e.t.Helper()
	ctx := context.Background()
	p := store.PipelineRow{ID: "p" + id, Revision: "rev1", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := e.st.CreatePipeline(ctx, p); err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: p.ID, Name: id, Kind: kind, Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""); err != nil {
		e.t.Fatal(err)
	}
	for _, s := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "running", 1}} {
		if err := e.st.Transition(ctx, id, s.from, s.to, store.TransitionDetail{Epoch: s.epoch}); err != nil {
			e.t.Fatal(err)
		}
	}
	facts, err := e.st.ArtifactFacts(ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	facts.Platform, facts.Isolation, facts.Producer = "linux/amd64", "process", "nself-ci/"+id
	ds, err := records.Normalize(model.Job{Artifacts: decls}, facts.Release, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	if err = e.st.ArtifactDeclare(ctx, id, 2, ds); err != nil {
		e.t.Fatal(err)
	}
	return facts, ds
}

// stage collects and stages the declarations of attempt id.
func (e *env) stage(id string, facts records.Facts, ds []records.Decl) ([]records.Staged, error) {
	e.t.Helper()
	staged, err := e.collector().Collect(context.Background(), facts.Project, e.ws, ds)
	if err != nil {
		return nil, err
	}
	return staged, e.st.ArtifactStage(context.Background(), id, 2, records.ToRecords(id, facts, 2, staged, e.now))
}

func (e *env) finalizing(id string) {
	e.t.Helper()
	if err := e.st.Transition(context.Background(), id, "running", "finalizing", store.TransitionDetail{Epoch: 2}); err != nil {
		e.t.Fatal(err)
	}
}

// run does attempt+stage+finalizing+commit and returns the committed records.
func (e *env) commit(id string, decls ...model.ArtifactDecl) []records.Record {
	e.t.Helper()
	facts, ds := e.attempt(id, "test", decls...)
	if _, err := e.stage(id, facts, ds); err != nil {
		e.t.Fatal(err)
	}
	e.finalizing(id)
	recs, err := records.CommitAndVerify(context.Background(), e.st, e.blobs, id, 3, e.pol)
	if err != nil {
		e.t.Fatal(err)
	}
	return recs
}

func (e *env) blobFiles() []string {
	var out []string
	root := filepath.Join(e.home, "blobs")
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if rel, _ := filepath.Rel(root, p); err == nil && info.Mode().IsRegular() && strings.HasPrefix(rel, "artifacts-") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func decl(path string) model.ArtifactDecl { return model.ArtifactDecl{Path: path} }

var viewer = authz.Viewer{Project: "proj", Role: authz.RoleMember}
