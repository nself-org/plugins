//go:build cachesec

package cachesec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

type rig struct {
	t       *testing.T
	ctx     context.Context
	home    string
	st      *store.Store
	blobs   *cas.Store
	entries *entries.Store
	engine  *specs.Engine
	red     *redact.Redactor
	audits  []store.AuditRow
	note    string // set by a runner when the admitted action was a clean miss rather than a restore
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, ctx: context.Background(), home: t.TempDir()}
	var err error
	r.st, err = store.Open(filepath.Join(r.home, "state.db"), store.Options{})
	must(t, err)
	t.Cleanup(func() { must(t, r.st.Close()) })
	r.st.SetAuditSealer(func(_ []byte, row store.AuditRow) (store.AuditChain, error) {
		r.audits = append(r.audits, row)
		return store.AuditChain{Head: []byte("test-head"), Signature: []byte("test-seal")}, nil
	})
	r.blobs, err = cas.New(r.home)
	must(t, err)
	r.entries = entries.New(r.st, r.blobs, entries.Options{DisableFloor: true, Getenv: func(string) string { return "" }, Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: true} }})
	r.red = redact.New()
	r.engine = &specs.Engine{Entries: r.entries, Blobs: r.blobs, Backend: r.st, Scanner: r.red}
	return r
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func owner() entries.Facts {
	return entries.Facts{Project: "project", Trust: "owner", Protection: "protected"}
}
func fork(pr string) entries.Facts {
	return entries.Facts{Project: "project", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/project", PR: pr}
}

func namespace(t *testing.T, f entries.Facts) authz.Namespace {
	t.Helper()
	ns, err := entries.NamespaceOf(f)
	must(t, err)
	return ns
}

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func key(t *testing.T, f entries.Facts) string {
	t.Helper()
	k, err := entries.Key(entries.KeyInputs{Namespace: namespace(t, f), SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "container"})
	must(t, err)
	return k
}

func (r *rig) running(id string, f entries.Facts) {
	t, ctx, s := r.t, r.ctx, r.st
	must(t, s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Revision: strings.Repeat("a", 40), Project: f.Project, Trigger: "local", SourceTrust: f.Trust, PrivacyZone: "local-only", PolicyDigest: "policy", InputDigest: "input", SelectionMode: "full", Status: "queued"}))
	must(t, s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "input"}))
	must(t, s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""))
	must(t, storetest.Admit(ctx, s, id))
	must(t, s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}))
}

func (r *rig) finalizing(id string, f entries.Facts, k string) {
	r.running(id, f)
	must(r.t, r.entries.RecordPlan(r.ctx, id, 2, f, []string{k}))
	must(r.t, r.st.Transition(r.ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}))
	must(r.t, r.entries.RecordResult(r.ctx, id, 3, true))
}

func (r *rig) manifest(f entries.Facts, k string, b []byte) entries.Manifest {
	ns := namespace(r.t, f)
	d, n, err := r.blobs.Put(r.ctx, cas.CacheRealm(string(ns.Canonical())), strings.NewReader(string(b)))
	must(r.t, err)
	return entries.Manifest{Namespace: ns.Canonical(), Key: k, Spec: "go", Blobs: []entries.Blob{{Path: "data", SHA256: d, Size: n}}, Writer: entries.Writer{ScannedClean: true}}
}

func (r *rig) audit(action, reason string) bool {
	rows, err := r.st.AuditEntries(r.ctx, 0, 1000)
	must(r.t, err)
	for _, row := range rows {
		if row.Action == action && strings.Contains(row.Detail, "reason="+reason) {
			return true
		}
	}
	return false
}

func cacheRealm(ns []byte) cas.Realm { return cas.CacheRealm(string(ns)) }

func (r *rig) noEntries() {
	r.t.Helper()
	rows, err := r.st.CacheEntries(r.ctx, "")
	must(r.t, err)
	if len(rows) != 0 {
		r.t.Errorf("refusal left %d cache entries", len(rows))
	}
}

func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = digest(b)
		return nil
	})
	if os.IsNotExist(err) {
		return out
	}
	must(t, err)
	return out
}
