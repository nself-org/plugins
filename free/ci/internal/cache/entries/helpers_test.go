package entries_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var (
	trusted  = entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}
	untrust1 = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "1"}
)

type env struct {
	t     *testing.T
	ctx   context.Context
	st    *store.Store
	blobs *cas.Store
	es    *entries.Store
	home  string
	clock atomic.Int64 // unix nanoseconds
	free  atomic.Int64 // bytes the Free hook reports; <0 means plenty
	total atomic.Int64 // volume size the Free hook reports
	// freeFn, when set, replaces free (it may read the store).
	freeFn func() int64
	warn   strings.Builder
	mu     sync.Mutex
	audits []store.AuditRow
}

type envOpts struct {
	env     map[string]string
	project func(string) authz.ProjectFacts
}

func newEnv(t *testing.T, o envOpts) *env {
	t.Helper()
	e := &env{t: t, ctx: context.Background(), home: t.TempDir()}
	e.clock.Store(time.Now().UnixNano())
	e.free.Store(-1)
	e.total.Store(100 << 30)
	now := func() time.Time { return time.Unix(0, e.clock.Load()) }
	var err error
	if e.st, err = store.Open(filepath.Join(e.home, "state.db"), store.Options{Clock: now}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.st.Close() })
	e.st.SetAuditSealer(func(_ []byte, row store.AuditRow) (store.AuditChain, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.audits = append(e.audits, row)
		return store.AuditChain{Head: []byte("h"), Signature: []byte("s")}, nil
	})
	if e.blobs, err = cas.New(e.home); err != nil {
		t.Fatal(err)
	}
	e.es = entries.New(e.st, e.blobs, entries.Options{
		Home: e.home, Clock: now, Warn: &e.warn, Project: o.project,
		Getenv: func(k string) string { return o.env[k] },
		Free: func() (int64, int64, error) {
			if e.freeFn != nil {
				return e.freeFn(), e.total.Load(), nil
			}
			if f := e.free.Load(); f >= 0 {
				return f, e.total.Load(), nil
			}
			return 1 << 40, e.total.Load(), nil
		},
	})
	return e
}

func (e *env) advance(d time.Duration) { e.clock.Add(int64(d)) }

func (e *env) ns(f entries.Facts) authz.Namespace {
	e.t.Helper()
	ns, err := entries.NamespaceOf(f)
	if err != nil {
		e.t.Fatal(err)
	}
	return ns
}

func (e *env) key(f entries.Facts, extra string) string {
	e.t.Helper()
	k, err := entries.Key(entries.KeyInputs{Namespace: e.ns(f), SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "container", Extras: []string{extra}})
	if err != nil {
		e.t.Fatal(err)
	}
	return k
}

// attempt creates attempt id, records the plan while running, moves it to
// finalizing and records the result. It returns the finalizing epoch.
func (e *env) attempt(id string, f entries.Facts, keys []string, passed bool) int64 {
	e.t.Helper()
	ctx, s := e.ctx, e.st
	steps := []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Revision: "rev", Project: f.Project, Trigger: "local", SourceTrust: f.Trust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}),
		s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""),
		s.Transition(ctx, id, "queued", "leased", store.TransitionDetail{Epoch: 0}),
		s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}),
		e.es.RecordPlan(ctx, id, 2, f, keys),
		s.Transition(ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}),
		e.es.RecordResult(ctx, id, 3, passed),
	}
	for i, err := range steps {
		if err != nil {
			e.t.Fatalf("attempt %s step %d: %v", id, i, err)
		}
	}
	return 3
}

// put stores content in the cache realm of ns and returns its manifest blob.
func (e *env) put(ns authz.Namespace, path, content string) entries.Blob {
	e.t.Helper()
	digest, size, err := e.blobs.Put(e.ctx, cas.CacheRealm(string(ns.Canonical())), strings.NewReader(content))
	if err != nil {
		e.t.Fatal(err)
	}
	return entries.Blob{Path: path, SHA256: digest, Size: size}
}

// manifest builds a manifest for key from freshly Put blobs.
func (e *env) manifest(f entries.Facts, key string, contents ...string) entries.Manifest {
	e.t.Helper()
	ns := e.ns(f)
	m := entries.Manifest{Namespace: ns.Canonical(), Key: key, Spec: "go"}
	for i, c := range contents {
		m.Blobs = append(m.Blobs, e.put(ns, fmt.Sprintf("part%02d.tar.gz", i), c))
	}
	return m
}

// save runs a whole passing attempt that commits one entry; it returns the manifest.
func (e *env) save(id string, f entries.Facts, extra string, contents ...string) entries.Manifest {
	e.t.Helper()
	key := e.key(f, extra)
	fin := e.attempt(id, f, []string{key}, true)
	m := e.manifest(f, key, contents...)
	if err := e.es.Commit(e.ctx, id, fin, e.ns(f), m); err != nil {
		e.t.Fatalf("commit %s: %v", id, err)
	}
	return m
}

func (e *env) lookup(f entries.Facts, key string) (entries.Manifest, bool) {
	e.t.Helper()
	ns := e.ns(f)
	m, ok, err := e.es.Lookup(e.ctx, ns, key, authz.ModeRWOwn)
	if err != nil {
		e.t.Fatal(err)
	}
	return m, ok
}

func (e *env) blobPath(m entries.Manifest, i int) string {
	d := m.Blobs[i].SHA256
	return filepath.Join(e.home, "blobs", string(cas.CacheRealm(string(m.Namespace))), "sha256", d[:2], d)
}

func (e *env) count() int {
	e.t.Helper()
	list, err := e.st.CacheEntries(e.ctx, "")
	if err != nil {
		e.t.Fatal(err)
	}
	return len(list)
}

// blobFiles lists the cache blob files under home.
func (e *env) blobFiles() (n int) {
	_ = filepath.WalkDir(filepath.Join(e.home, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && strings.Contains(p, string(filepath.Separator)+"sha256"+string(filepath.Separator)) {
			n++
		}
		return nil
	})
	return n
}

// audit returns the audit rows with the given action.
func (e *env) audit(action string) (out []store.AuditRow) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.audits {
		if r.Action == action {
			out = append(out, r)
		}
	}
	return out
}
