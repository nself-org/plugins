//go:build cachesec

package cachesec

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func artifactPolicy() records.Policy {
	return records.Policy{MaxFile: 32 << 20, MaxTotal: 64 << 20, Statfs: func(string) (uint64, uint64, error) { return 1 << 40, 2 << 40, nil }}
}

func (r *rig) secret(id string) error {
	// This generated marker is registered only in this isolated test redactor.
	marker := []byte("fixture-" + digest([]byte(r.home)))
	must(r.t, r.red.Register(marker))
	payload := marker
	if strings.HasSuffix(id, "base64") {
		payload = []byte(base64.StdEncoding.EncodeToString(marker))
	}
	ws := r.t.TempDir()
	must(r.t, os.Mkdir(filepath.Join(ws, "data"), 0o700))
	must(r.t, os.WriteFile(filepath.Join(ws, "data", "payload"), payload, 0o600))
	var err error
	if strings.HasPrefix(id, "cache-") {
		base, e := os.OpenRoot(ws)
		must(r.t, e)
		defer base.Close()
		_, err = r.engine.Save(r.ctx, specs.SaveRequest{Base: base, Attempt: "writer", Epoch: 3, Namespace: namespace(r.t, owner()), Key: key(r.t, owner()), Spec: specs.Spec{ID: "go"}, Dirs: specs.Dirs{"data": "data"}})
	} else {
		c := records.Collector{Blobs: r.blobs, Scanner: r.red, Policy: artifactPolicy()}
		_, err = c.Collect(r.ctx, "project", ws, []records.Decl{{Path: "data/payload", Kind: records.KindBinary, Retention: "30d", Visibility: authz.VisibilityProject}})
	}
	r.noEntries()
	if len(tree(r.t, filepath.Join(r.home, "blobs"))) != 0 {
		r.t.Error("secret scan published blob bytes")
	}
	if err != nil && (strings.Contains(err.Error(), string(marker)) || strings.Contains(err.Error(), string(payload))) {
		r.t.Fatal("refusal exposed the registered marker")
	}
	return err
}

func (r *rig) artifactStage(id string) records.Record {
	r.running(id, owner())
	d := records.Decl{Path: "app", Kind: records.KindBinary, Retention: "30d", Visibility: authz.VisibilityProject}
	must(r.t, r.st.ArtifactDeclare(r.ctx, id, 2, []records.Decl{d}))
	f, err := r.st.ArtifactFacts(r.ctx, id)
	must(r.t, err)
	recs := records.ToRecords(id, f, 2, []records.Staged{{Decl: d, Name: "app", Rel: "app", SHA256: digest([]byte(id)), Size: int64(len(id))}}, time.Now())
	must(r.t, r.st.ArtifactStage(r.ctx, id, 2, recs))
	must(r.t, r.st.Transition(r.ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}))
	return recs[0]
}

func (r *rig) artifactMutation(id string) error {
	a := r.artifactStage("attempt-a")
	b := r.artifactStage("attempt-b")
	if a.ID == b.ID {
		r.t.Fatal("attempts share artifact identity")
	}
	before, err := r.st.ArtifactStaged(r.ctx, "attempt-a")
	must(r.t, err)
	changed := a
	if id == "artifact-overwrite" {
		changed.SHA256 = digest([]byte("replacement"))
	} else {
		changed = b
	}
	_, err = r.st.ArtifactCommit(r.ctx, "attempt-a", 3, []records.Record{changed}, artifactPolicy())
	after, e := r.st.ArtifactStaged(r.ctx, "attempt-a")
	must(r.t, e)
	if !reflect.DeepEqual(before, after) {
		r.t.Error("refused commit mutated staged records")
	}
	rows, e := r.st.ArtifactList(r.ctx, records.Filter{Project: "project"})
	must(r.t, e)
	for _, row := range rows {
		if row.State == records.StateCommitted {
			r.t.Error("substitution committed an artifact")
		}
	}
	return err
}

func (r *rig) getDest(id string) error {
	cwd := r.t.TempDir()
	dest := "."
	if id == "get-nonempty" {
		dest = "out"
		must(r.t, os.Mkdir(filepath.Join(cwd, dest), 0o700))
		must(r.t, os.WriteFile(filepath.Join(cwd, dest, "sentinel"), []byte("keep"), 0o600))
	}
	if id == "get-hooks" {
		dest = ".git/hooks"
		must(r.t, os.MkdirAll(filepath.Join(cwd, dest), 0o700))
	}
	before := tree(r.t, cwd)
	rec := r.artifactStage("download")
	_, err := r.st.ArtifactCommit(r.ctx, "download", 3, []records.Record{rec}, artifactPolicy())
	must(r.t, err)
	// A visible committed record ensures the real download reaches the destination check.
	reader := records.Reader{Ledger: r.st, Blobs: r.blobs}
	_, _, err = reader.Get(r.ctx, authz.Viewer{Project: "project", Role: authz.RoleMember}, rec.ID, dest, cwd)
	if !reflect.DeepEqual(before, tree(r.t, cwd)) {
		r.t.Error("refused download changed the destination")
	}
	return err
}
