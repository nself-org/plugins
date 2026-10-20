//go:build cachesec

package cachesec

// Purpose: fixtures of group g2 (local artifact and blob stores, P7-CACHE-11): an artifact world on
// top of the frame's `world` (real store, audit sink, cas), a job workspace, the attempt lifecycle
// (running at epoch 2, finalizing at epoch 3, ended at epoch 4) and the byte counters the cases use
// for Observed.Landed.
// Inputs: newWorld. Outputs: g2World and helpers prefixed g2.
// Constraints: production packages only (records, store, cas, authz, redact); fake values only;
// scratch lives under t.TempDir().

import (
	"context"
	"errors"
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
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

const g2Project = "proj"

// g2World is an artifact fixture: the frame world plus a workspace, a scanner and a policy.
type g2World struct {
	*world
	Ws   string
	Red  *redact.Redactor
	Pol  records.Policy
	mark g2Mark
}

// g2Mark is the state of the attacked store when the honest control has finished and the attack starts.
type g2Mark struct {
	audit            int
	blobBytes, comit int64
}

func newG2World(t *testing.T) *g2World {
	t.Helper()
	w := newWorld(t)
	g := &g2World{world: w, Ws: filepath.Join(t.TempDir(), "ws"), Red: redact.New()}
	g.Pol = records.Policy{MaxFile: 1 << 20, MaxTotal: 64 << 20, ProjectShare: 50, StorePath: w.Home,
		Statfs: func(string) (uint64, uint64, error) { return 100 << 30, 200 << 30, nil }}
	if err := os.MkdirAll(g.Ws, 0o755); err != nil {
		t.Fatal(err)
	}
	return g
}

// Write puts a file in the workspace.
func (g *g2World) Write(rel, content string) {
	g.t.Helper()
	p := filepath.Join(g.Ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		g.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		g.t.Fatal(err)
	}
}

func (g *g2World) Collector() records.Collector {
	return records.Collector{Blobs: g.Blobs, Scanner: g.Red, Policy: g.Pol}
}

// Attempt creates attempt id, moves it to running (epoch 2) and declares paths.
func (g *g2World) Attempt(id string, paths ...string) (records.Facts, []records.Decl) {
	g.t.Helper()
	ctx, s := g.ctx, g.St
	steps := []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Revision: "rev1", Project: g2Project, Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}),
		s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""),
		storetest.Admit(ctx, s, id),
		s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}),
	}
	for i, err := range steps {
		if err != nil {
			g.t.Fatalf("attempt %s step %d: %v", id, i, err)
		}
	}
	facts, err := s.ArtifactFacts(ctx, id)
	if err != nil {
		g.t.Fatal(err)
	}
	facts.Platform, facts.Isolation, facts.Producer = "linux/amd64", "process", "nself-ci/"+id
	var decls []model.ArtifactDecl
	for _, p := range paths {
		decls = append(decls, model.ArtifactDecl{Path: p})
	}
	ds, err := records.Normalize(model.Job{Artifacts: decls}, facts.Release, nil)
	if err != nil {
		g.t.Fatal(err)
	}
	if err = s.ArtifactDeclare(ctx, id, 2, ds); err != nil {
		g.t.Fatal(err)
	}
	return facts, ds
}

// Stage collects the declarations from the workspace and stages the records at epoch 2.
func (g *g2World) Stage(id string, f records.Facts, ds []records.Decl) []records.Staged {
	g.t.Helper()
	st, err := g.Collector().Collect(g.ctx, f.Project, g.Ws, ds)
	if err != nil {
		g.t.Fatalf("collect %s: %v", id, err)
	}
	if err = g.St.ArtifactStage(g.ctx, id, 2, records.ToRecords(id, f, 2, st, g.clockTime())); err != nil {
		g.t.Fatalf("stage %s: %v", id, err)
	}
	return st
}

func (g *g2World) Finalizing(id string) {
	g.t.Helper()
	if err := g.St.Transition(g.ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}); err != nil {
		g.t.Fatal(err)
	}
}

// End moves a finalizing attempt (epoch 3) to passed or failed (epoch 4).
func (g *g2World) End(id string, to model.JobState) {
	g.t.Helper()
	if err := g.St.Transition(g.ctx, id, "finalizing", to, store.TransitionDetail{Epoch: 3}); err != nil {
		g.t.Fatal(err)
	}
}

// Pass runs the honest lifecycle: collect, stage, finalizing, CommitAndVerify at epoch 3, passed.
func (g *g2World) Pass(id string, paths ...string) []records.Record {
	g.t.Helper()
	f, ds := g.Attempt(id, paths...)
	g.Stage(id, f, ds)
	g.Finalizing(id)
	recs, err := records.CommitAndVerify(g.ctx, g.St, g.Blobs, id, 3, g.Pol)
	if err != nil {
		g.t.Fatalf("commit %s: %v", id, err)
	}
	g.End(id, "passed")
	return recs
}

// CommittedBytes is the size of every committed record of the attempt ("" = all attempts).
func (g *g2World) CommittedBytes(attempt string) (n int64) {
	g.t.Helper()
	list, err := g.St.ArtifactList(g.ctx, records.Filter{Attempt: attempt})
	if err != nil {
		g.t.Fatal(err)
	}
	for _, r := range list {
		n += r.Size
	}
	return n
}

// BlobFiles counts the artifact blobs on disk (the published ones, not the staging area).
func (g *g2World) BlobFiles() (n int) {
	root := filepath.Join(g.Home, "blobs")
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if rel, _ := filepath.Rel(root, p); err == nil && info.Mode().IsRegular() && strings.HasPrefix(rel, "artifacts-") {
			n++
		}
		return nil
	})
	return n
}

// BlobBytes is the size of every published artifact blob on disk.
func (g *g2World) BlobBytes() (n int64) {
	root := filepath.Join(g.Home, "blobs")
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if rel, _ := filepath.Rel(root, p); err == nil && info.Mode().IsRegular() && strings.HasPrefix(rel, "artifacts-") {
			n += info.Size()
		}
		return nil
	})
	return n
}

// Mark remembers the store state before the attack (call it after the honest control).
func (g *g2World) Mark() {
	g.mark = g2Mark{audit: len(g.Audit("")), blobBytes: g.BlobBytes(), comit: g.CommittedBytes("")}
}

// AuditSince is "none" when the attack appended no audit row since Mark, else the action of the first new row.
func (g *g2World) AuditSince() string {
	rows := g.Audit("")
	if len(rows) == g.mark.audit {
		return "none"
	}
	return rows[g.mark.audit].Action
}

// LandedSince is the bytes that became reachable since Mark: new artifact blobs plus new committed records.
func (g *g2World) LandedSince() int64 {
	return max(0, g.BlobBytes()-g.mark.blobBytes) + max(0, g.CommittedBytes("")-g.mark.comit)
}

// g2TreeBytes is the size of every regular file below dir (symlinks are not followed).
func g2TreeBytes(dir string) (n int64) {
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			n += info.Size()
		}
		return nil
	})
	return n
}

// BlobPath is where the cas keeps the artifact blob with digest sha.
func (g *g2World) BlobPath(sha string) string {
	r := string(cas.ArtifactRealm(g2Project))
	return filepath.Join(g.Home, "blobs", r, "sha256", sha[:2], sha)
}

func (g *g2World) clockTime() time.Time { return time.Unix(0, g.clock.Load()) }

func g2RecordsReason(t *testing.T, err error) string {
	t.Helper()
	var e *records.Error
	if !errors.As(err, &e) {
		t.Fatalf("want a records refusal, got %v", err)
	}
	return e.Reason
}

// g2PathObserved turns an authz E683 refusal into an Observed (PathError has no Code method; the
// production constant authz.CodeArtifactPath is its code).
func g2PathObserved(t *testing.T, err error) (Observed, authz.PathReason) {
	t.Helper()
	var pe *authz.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("want an E683 path refusal, got %v", err)
	}
	// reason keys are [a-z0-9_]: "entry.hardlink" becomes "entry_hardlink"
	return Observed{Outcome: Refused, Code: authz.CodeArtifactPath, Reason: strings.ReplaceAll(string(pe.Reason), ".", "_")}, pe.Reason
}

var _ = context.Background
