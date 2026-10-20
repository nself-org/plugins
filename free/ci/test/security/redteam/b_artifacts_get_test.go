//go:build security && unix

package redteam

// Purpose: red team attacks on the artifact ledger and `artifacts get` (TM-13, TM-14, TM-15):
// hostile record names that reach the reader through a compromised collector, destinations in
// .git, blob substitution after commit, stale-epoch and post-commit writes.
// Inputs:  the real store.Store ledger, records.CommitAndVerify and records.Reader over temp dirs.
// Outputs: pass or fail; a pass means the attack failed.
// Constraints: local only; the destination checks run against temp directories.

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
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

var artViewer = authz.Viewer{Project: "proj", Role: authz.RoleMember}

type ledgerEnv struct {
	*artEnv
	st    *store.Store
	facts records.Facts
	ctx   context.Context
}

// newLedgerEnv creates attempt id in state running (declared at epoch 2).
func newLedgerEnv(t *testing.T, id string) *ledgerEnv {
	t.Helper()
	a := newArtEnv(t)
	st, err := store.Open(filepath.Join(a.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &ledgerEnv{artEnv: a, st: st, ctx: context.Background()}
	e.newAttempt(id)
	return e
}

func (e *ledgerEnv) newAttempt(id string) {
	t, ctx, s := e.t, e.ctx, e.st
	p := store.PipelineRow{ID: "p" + id, Revision: "rev1", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	for i, err := range []error{s.CreatePipeline(ctx, p),
		s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: p.ID, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""), storetest.Admit(ctx, s, id),
		s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1})} {
		if err != nil {
			t.Fatalf("attempt %s step %d: %v", id, i, err)
		}
	}
	f, err := s.ArtifactFacts(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	f.Platform, f.Isolation, f.Producer = "linux/amd64", "process", "nself-ci/"+id
	e.facts = f
}

// stage Puts content as a blob and stages a record called name for attempt id at epoch.
func (e *ledgerEnv) stage(id, name, content string, epoch int64) (records.Staged, error) {
	e.t.Helper()
	ds, err := records.Normalize(model.Job{Artifacts: []model.ArtifactDecl{{Path: "out/x"}}}, false, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	_ = e.st.ArtifactDeclare(e.ctx, id, 2, ds)
	digest, size, err := e.blobs.Put(e.ctx, cas.ArtifactRealm("proj"), strings.NewReader(content))
	if err != nil {
		e.t.Fatal(err)
	}
	s := records.Staged{Decl: ds[0], Name: name, Rel: "out/x", SHA256: digest, Size: size}
	return s, e.st.ArtifactStage(e.ctx, id, epoch, records.ToRecords(id, e.facts, epoch, []records.Staged{s}, time.Now()))
}

func (e *ledgerEnv) commit(id string) ([]records.Record, error) {
	if err := e.st.Transition(e.ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}); err != nil {
		e.t.Fatal(err)
	}
	return records.CommitAndVerify(e.ctx, e.st, e.blobs, id, 3, e.pol)
}

func (e *ledgerEnv) reader() records.Reader { return records.Reader{Ledger: e.st, Blobs: e.blobs} }

// A collector that was fooled (or a tampered ledger row) stores a hostile record name; the reader
// is the last gate before bytes reach a developer's disk.
func TestB_Artifacts_GetRefusesHostileRecordNames(t *testing.T) {
	for i, name := range []string{"../../escape.txt", "a/../../escape.txt", ".git/hooks/pre-commit", ".GIT/hooks/post-checkout", "/tmp/abs.txt", "x/.git /config", "ok/./../../e", `..\\win`, "a\x00b"} {
		id := "g" + string(rune('a'+i))
		e := newLedgerEnv(t, id)
		cwd := t.TempDir()
		dest := filepath.Join(cwd, "dl")
		if _, err := e.stage(id, name, "payload", 2); err != nil {
			t.Logf("%q: refused at stage: %.60v", name, err)
			continue // refused at the ledger: the attack ends there
		}
		recs, err := e.commit(id)
		if err != nil || len(recs) == 0 {
			t.Logf("%q: refused at commit: %.60v", name, err)
			continue
		}
		path, _, err := e.reader().Get(e.ctx, artViewer, recs[0].ID, dest, cwd)
		t.Logf("%q: refused at get: %.70v", name, err)
		if err == nil {
			t.Errorf("hostile name %q was extracted to %q", name, path)
		}
		for _, p := range []string{filepath.Join(cwd, "escape.txt"), filepath.Join(filepath.Dir(cwd), "escape.txt"), filepath.Join(dest, ".git")} {
			if _, serr := os.Lstat(p); serr == nil {
				t.Errorf("name %q wrote %s", name, p)
			}
		}
	}
}

func TestB_Artifacts_GetDestinationAttacks(t *testing.T) {
	e := newLedgerEnv(t, "dst")
	if _, err := e.stage("dst", "out/x", "payload", 2); err != nil {
		t.Fatal(err)
	}
	recs, err := e.commit("dst")
	if err != nil || len(recs) != 1 {
		t.Fatalf("commit: %v", err)
	}
	cwd := t.TempDir()
	mustMk := func(p string) string {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	hooks := mustMk(filepath.Join(cwd, ".git", "hooks"))
	other := mustMk(filepath.Join(t.TempDir(), "elsewhere"))
	nonEmpty := mustMk(filepath.Join(cwd, "full"))
	_ = os.WriteFile(filepath.Join(nonEmpty, "keep"), []byte("x"), 0o644)
	linkToHooks := filepath.Join(cwd, "linkdest")
	_ = os.Symlink(hooks, linkToHooks)
	_ = os.Symlink(filepath.Join(cwd, ".git"), filepath.Join(other, "gitlink"))
	for name, dest := range map[string]string{
		"git-hooks":           hooks,
		"git-dir-relative":    ".git/hooks",
		"git-dotdot":          "sub/../.git/hooks",
		"git-case":            filepath.Join(cwd, ".GIT", "hooks"),
		"git-trailing-dot":    filepath.Join(cwd, ".git.", "hooks"),
		"symlink-leaf":        linkToHooks,
		"symlink-parent":      filepath.Join(other, "gitlink", "hooks"),
		"cwd":                 cwd,
		"cwd-dot":             ".",
		"non-empty":           nonEmpty,
		"empty-string":        "",
		"parent-missing":      filepath.Join(cwd, "nope", "deeper"),
		"git-short-name":      filepath.Join(cwd, "GIT~1", "hooks"),
		"git-zero-width":      filepath.Join(cwd, ".g‌it", "hooks"),
		"root-of-repo-parent": filepath.Join(cwd, ".git") + "/",
	} {
		_, _, err := e.reader().Get(e.ctx, artViewer, recs[0].ID, dest, cwd)
		if err == nil {
			t.Errorf("%s: dest %q accepted", name, dest)
		}
	}
	entries, _ := os.ReadDir(hooks)
	if len(entries) != 0 {
		t.Errorf(".git/hooks received files: %v", entries)
	}
	// control: a fresh sibling directory works, so the refusals above are not a broken fixture.
	if _, _, err := e.reader().Get(e.ctx, artViewer, recs[0].ID, filepath.Join(cwd, "fresh"), cwd); err != nil {
		t.Errorf("control extraction failed: %v", err)
	}
}

func TestB_Artifacts_BlobSubstitutionAfterCommit(t *testing.T) {
	for _, mode := range []string{"same-size-garbage", "symlink-to-outside", "truncate", "hardlink-to-outside"} {
		e := newLedgerEnv(t, "sub")
		if _, err := e.stage("sub", "out/x", "payload-bytes", 2); err != nil {
			t.Fatal(err)
		}
		recs, err := e.commit("sub")
		if err != nil || len(recs) != 1 {
			t.Fatalf("commit: %v", err)
		}
		outside := filepath.Join(e.outside, "same-size")
		_ = os.WriteFile(outside, []byte("EVIL-BYTES-XX"), 0o644) // 13 bytes, same as the payload
		blobPath := filepath.Join(e.home, "blobs", string(cas.ArtifactRealm("proj")), "sha256", recs[0].SHA256[:2], recs[0].SHA256)
		_ = os.Remove(blobPath)
		switch mode {
		case "same-size-garbage":
			_ = os.WriteFile(blobPath, []byte("EVIL-BYTES-XX"), 0o644)
		case "symlink-to-outside":
			_ = os.Symlink(outside, blobPath)
		case "truncate":
			_ = os.WriteFile(blobPath, nil, 0o644)
		case "hardlink-to-outside":
			_ = os.Link(outside, blobPath)
		}
		cwd := t.TempDir()
		dest := filepath.Join(cwd, "dl")
		path, _, gerr := e.reader().Get(e.ctx, artViewer, recs[0].ID, dest, cwd)
		if gerr == nil {
			got, _ := os.ReadFile(path)
			t.Errorf("%s: get succeeded with %q", mode, got)
		}
		if ents, _ := os.ReadDir(filepath.Join(dest, "out")); len(ents) > 0 {
			t.Errorf("%s: a file was left behind in the destination", mode)
		}
		if _, err := records.CommitAndVerify(e.ctx, e.st, e.blobs, "sub", 3, e.pol); err == nil {
			t.Errorf("%s: a second CommitAndVerify trusted the substituted blob", mode)
		}
	}
}

func TestB_Artifacts_StaleEpochAndPostCommitWrites(t *testing.T) {
	e := newLedgerEnv(t, "ep")
	for _, ep := range []int64{0, 1, 3, 99, -1} {
		if _, err := e.stage("ep", "out/x", "payload", ep); err == nil {
			t.Errorf("stage at foreign epoch %d accepted while the attempt runs at 2", ep)
		}
	}
	if _, err := e.stage("ep", "out/x", "payload-v1", 2); err != nil {
		t.Fatal(err)
	}
	recs, err := e.commit("ep")
	if err != nil || len(recs) != 1 {
		t.Fatalf("commit: %v", err)
	}
	first := recs[0]
	// a zombie holding the old running epoch writes after the commit
	if _, err := e.stage("ep", "out/x", "payload-v2-zombie", 2); err == nil {
		t.Error("stage at the old running epoch accepted after finalizing")
	}
	// the live epoch re-stages different bytes under the same name after the commit
	_, serr := e.stage("ep", "out/x", "payload-v3-overwrite", 3)
	_, cerr := records.CommitAndVerify(e.ctx, e.st, e.blobs, "ep", 3, e.pol)
	rec, ok, err := e.st.ArtifactGet(e.ctx, first.ID)
	if err != nil || !ok {
		t.Fatalf("record gone: %v", err)
	}
	if rec.SHA256 != first.SHA256 {
		t.Errorf("a committed record was overwritten (stage err=%v commit err=%v): %s -> %s", serr, cerr, first.SHA256[:12], rec.SHA256[:12])
	}
	// commit with the wrong epoch, and after the attempt ended
	if _, err := records.CommitAndVerify(e.ctx, e.st, e.blobs, "ep", 2, e.pol); err == nil {
		t.Log("commit at the old epoch: nothing staged, no-op")
	}
	if err := e.st.Transition(e.ctx, "ep", "finalizing", "passed", store.TransitionDetail{Epoch: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.stage("ep", "out/x", "payload-v4-after-end", 3); err == nil {
		t.Error("stage after the attempt ended accepted")
	}
}
