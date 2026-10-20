//go:build security

package redteam

// Purpose: shared fixture of the b_* cache attacks: a real store.Store (sqlite in a temp dir), a
// real cas.Store and a real entries.Store wired the way internal/cache/entries tests wire them.
// Inputs:  facts of an attempt. Outputs: attempts at known epochs and committable manifests.
// Constraints: temp dir only; the clock is fixed so blob freshness is deterministic.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

var (
	factsMain = entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}
	factsPR1  = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "1"}
	factsPR2  = entries.Facts{Project: "proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "2"}
)

type cacheEnv struct {
	t     *testing.T
	ctx   context.Context
	st    *store.Store
	blobs *cas.Store
	es    *entries.Store
	home  string
	audit []store.AuditRow
}

func newCacheEnv(t *testing.T) *cacheEnv {
	t.Helper()
	e := &cacheEnv{t: t, ctx: context.Background(), home: t.TempDir()}
	var err error
	if e.st, err = store.Open(filepath.Join(e.home, "state.db"), store.Options{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.st.Close() })
	e.st.SetAuditSealer(func(_ []byte, row store.AuditRow) (store.AuditChain, error) {
		e.audit = append(e.audit, row)
		return store.AuditChain{Head: []byte("h"), Signature: []byte("s")}, nil
	})
	if e.blobs, err = cas.New(e.home); err != nil {
		t.Fatal(err)
	}
	e.es = entries.New(e.st, e.blobs, entries.Options{Home: e.home, Warn: &strings.Builder{},
		Free: func() (int64, int64, error) { return 1 << 40, 1 << 41, nil }})
	return e
}

func (e *cacheEnv) ns(f entries.Facts) authz.Namespace {
	e.t.Helper()
	ns, err := entries.NamespaceOf(f)
	if err != nil {
		e.t.Fatal(err)
	}
	return ns
}

func (e *cacheEnv) key(f entries.Facts, extra string) string {
	e.t.Helper()
	k, err := entries.Key(entries.KeyInputs{Namespace: e.ns(f), SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "container", Extras: []string{extra}})
	if err != nil {
		e.t.Fatal(err)
	}
	return k
}

// start creates attempt id and moves it to running at epoch 1.
func (e *cacheEnv) start(id string, f entries.Facts) {
	e.t.Helper()
	ctx, s := e.ctx, e.st
	for i, err := range []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Revision: "rev", Project: f.Project, Trigger: "local", SourceTrust: f.Trust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}),
		s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""),
		storetest.Admit(ctx, s, id),
		s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}),
	} {
		if err != nil {
			e.t.Fatalf("start %s step %d: %v", id, i, err)
		}
	}
}

// finish plans keys at epoch 2, moves to finalizing and records the result at epoch 3.
func (e *cacheEnv) finish(id string, f entries.Facts, keys []string, passed bool) {
	e.t.Helper()
	for i, err := range []error{
		e.es.RecordPlan(e.ctx, id, 2, f, keys),
		e.st.Transition(e.ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}),
		e.es.RecordResult(e.ctx, id, 3, passed),
	} {
		if err != nil {
			e.t.Fatalf("finish %s step %d: %v", id, i, err)
		}
	}
}

// manifest builds a manifest for key, with blobs freshly Put in the realm of putIn.
func (e *cacheEnv) manifest(namespaceOf, putIn entries.Facts, key string, contents ...string) entries.Manifest {
	e.t.Helper()
	m := entries.Manifest{Namespace: e.ns(namespaceOf).Canonical(), Key: key, Spec: "go"}
	for i, c := range contents {
		digest, size, err := e.blobs.Put(e.ctx, cas.CacheRealm(string(e.ns(putIn).Canonical())), strings.NewReader(c))
		if err != nil {
			e.t.Fatal(err)
		}
		m.Blobs = append(m.Blobs, entries.Blob{Path: fmt.Sprintf("part%02d.tar.gz", i), SHA256: digest, Size: size})
	}
	return m
}

func (e *cacheEnv) lookup(f entries.Facts, key string, mode authz.Mode) bool {
	e.t.Helper()
	_, ok, err := e.es.Lookup(e.ctx, e.ns(f), key, mode)
	if err != nil {
		e.t.Fatal(err)
	}
	return ok
}

var _ = time.Second
