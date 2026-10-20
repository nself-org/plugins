package remote

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestCacheRetractUsingReopenedStoreDropsRealEntry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, err := range []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: "p", Project: "proj", Revision: "rev", SourceTrust: "owner", Status: "queued", Trigger: "local", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full"}),
		s.CreateJob(ctx, store.JobRow{ID: "j", PipelineID: "p", Name: "build", Kind: "test", Idempotent: true, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: "a", JobID: "j", N: 1}, ""),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	e := lease.New(s, nil, lease.Config{})
	epoch, err := e.Grant(ctx, "host", store.Demand{AttemptID: "a", LeaseID: "l", RunnerID: "node", CPU: 1, MemMB: 1, CapacityCPU: 2, CapacityMemMB: 2, TTL: time.Minute}, 1)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	facts := entries.Facts{Project: "proj", Trust: "owner", Protection: "protected"}
	ns, err := entries.NamespaceOf(facts)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Ledger: s, Entries: entries.New(s, blobs, entries.Options{DisableFloor: true}), Blobs: blobs}
	l, err := h.Bind(Binding{AttemptID: "a", LeaseID: "l", Epoch: epoch, Facts: facts, Plan: &protocol.LeaseCache{Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: "rw", Save: []protocol.CacheSave{{Spec: "go", Key: strings.Repeat("a", 64), Dirs: []string{"go/mod"}}}}, Fence: func(ctx context.Context) error { return e.Fence(ctx, "l", epoch) }, Ready: func(ctx context.Context) error {
		state, n, err := s.AttemptStateEpoch(ctx, "a")
		if err == nil && state == "leased" {
			return s.Transition(ctx, "a", "leased", "running", store.TransitionDetail{Epoch: n})
		}
		return err
	}})
	if err != nil {
		t.Fatal(err)
	}
	addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "payload", false))
	if err = e.Result(ctx, "l", epoch); err != nil {
		t.Fatal(err)
	}
	if err = l.Finalize(ctx, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.CacheEntries(ctx, ""); err != nil || len(rows) != 1 {
		t.Fatalf("before close entries=%v err=%v", rows, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	if err = l.RetractUsing(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if rows, err := fresh.CacheEntries(ctx, ""); err != nil || len(rows) != 0 {
		t.Fatalf("after reopen/retract entries=%v err=%v", rows, err)
	}
}

func TestCacheArchiveValidationObservesCancellation(t *testing.T) {
	l, _, _ := coordinatorFixture(t)
	addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "payload", true))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.validateRef(ctx, l.refs[l.b.Plan.Save[0].Key]); !errors.Is(err, context.Canceled) {
		t.Fatalf("validation=%v", err)
	}
}

func cacheArchive(t *testing.T, name string, compressed bool) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if !compressed {
		return raw.Bytes()
	}
	var packed bytes.Buffer
	gw := gzip.NewWriter(&packed)
	if _, err := gw.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return packed.Bytes()
}

func addRef(t *testing.T, l *Lease, key string, data []byte) {
	t.Helper()
	d, n, err := l.h.Blobs.Put(context.Background(), cacheRealm(l.b.Plan.Namespace), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	err = l.CacheRef(context.Background(), protocol.CacheRef{LeaseID: l.b.LeaseID, Key: key, Spec: "go", Blobs: []protocol.CacheBlob{{Path: "mod", SHA256: d, Size: n}}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCacheBindRejectsMalformedNamespace(t *testing.T) {
	l, _, _ := coordinatorFixture(t)
	l.b.Plan.Namespace = "invalid namespace"
	if got, err := l.h.Bind(l.b); err == nil || got != nil {
		t.Fatalf("lease=%v err=%v", got, err)
	}
}

func TestCacheRefRefusalAuditDeduplicated(t *testing.T) {
	l, s, _ := coordinatorFixture(t)
	for range 3 {
		_ = l.CacheRef(context.Background(), protocol.CacheRef{LeaseID: "l", Spec: "go", Key: strings.Repeat("b", 64)})
	}
	rows, err := s.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, row := range rows {
		if row.Action == "cache.remote.refused" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("refusal rows=%d, want 1", n)
	}
}

func TestCacheFinalizeValidatesEveryArchiveAndContinues(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(map[bool]string{false: "tar", true: "gzip"}[compressed], func(t *testing.T) {
			l, s, e := coordinatorFixture(t)
			good := strings.Repeat("b", 64)
			l.b.Plan.Save = append(l.b.Plan.Save, protocol.CacheSave{Spec: "go", Key: good, Dirs: []string{"go/mod"}})
			addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "../escape", compressed))
			addRef(t, l, good, cacheArchive(t, "payload", compressed))
			if err := e.Result(context.Background(), "l", l.b.Epoch); err != nil {
				t.Fatal(err)
			}
			var refused *entries.Refused
			if err := l.Finalize(context.Background(), time.Millisecond); !errors.As(err, &refused) || refused.Reason != entries.ReasonBlobInvalid {
				t.Fatalf("finalize=%v", err)
			}
			rows, err := s.CacheEntries(context.Background(), "")
			if err != nil || len(rows) != 1 || rows[0].Key != good {
				t.Fatalf("entries=%v err=%v", rows, err)
			}
			assertAudit(t, s, entries.ReasonBlobInvalid)
		})
	}
}

type failingLocality struct {
	Ledger
	err error
}

type failingCommits struct {
	entries.Backend
	seen []string
	errs map[string]error
}

func (f *failingCommits) CacheCommit(_ context.Context, r entries.CommitRequest) error {
	f.seen = append(f.seen, r.Manifest.Key)
	return f.errs[r.Manifest.Key]
}

func TestCacheFinalizeJoinsCommitErrorsAndContinues(t *testing.T) {
	l, s, e := coordinatorFixture(t)
	a, b := errors.New("first commit failed"), errors.New("second commit failed")
	keyA, keyB := l.b.Plan.Save[0].Key, strings.Repeat("b", 64)
	l.b.Plan.Save = append(l.b.Plan.Save, protocol.CacheSave{Spec: "go", Key: keyB, Dirs: []string{"go/mod"}})
	be := &failingCommits{Backend: s, errs: map[string]error{keyA: a, keyB: b}}
	l.h.Entries = entries.New(be, l.h.Blobs, entries.Options{DisableFloor: true})
	for _, save := range l.b.Plan.Save {
		addRef(t, l, save.Key, cacheArchive(t, "payload", false))
	}
	if err := e.Result(context.Background(), "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	err := l.Finalize(context.Background(), time.Millisecond)
	if !errors.Is(err, a) || !errors.Is(err, b) || len(be.seen) != 2 {
		t.Fatalf("commits=%v err=%v", be.seen, err)
	}
}

func (f failingLocality) CacheLocalityPut(context.Context, store.CacheLocalityRow) error {
	return f.err
}

func TestCacheFinalizeLocalityFailureDoesNotBlockSaves(t *testing.T) {
	l, s, e := coordinatorFixture(t)
	l.b.Node = "node"
	l.b.Plan.Save = append(l.b.Plan.Save, protocol.CacheSave{Spec: "go", Key: strings.Repeat("b", 64), Dirs: []string{"go/mod"}})
	failure := errors.New("locality unavailable")
	l.h.Ledger = failingLocality{l.h.Ledger, failure}
	warnings := 0
	l.h.Warn = func(err error) {
		if !errors.Is(err, failure) {
			t.Errorf("warning=%v", err)
		}
		warnings++
	}
	for _, save := range l.b.Plan.Save {
		addRef(t, l, save.Key, cacheArchive(t, "payload", false))
	}
	if err := e.Result(context.Background(), "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := l.Finalize(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	rows, err := s.CacheEntries(context.Background(), "")
	if err != nil || len(rows) != 2 || warnings != 2 {
		t.Fatalf("entries=%v warnings=%d err=%v", rows, warnings, err)
	}
}

func TestCacheFinalizeNoRefsUsesShortGrace(t *testing.T) {
	l, _, e := coordinatorFixture(t)
	if err := l.Declare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Result(context.Background(), "l", l.b.Epoch); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := l.Finalize(ctx, 30*time.Second); err != nil {
		t.Fatalf("ordered empty result waited full grace: %v", err)
	}
}

func TestCacheFinalizePreservesFullGraceForPartialOrUnordered(t *testing.T) {
	for _, unordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial", true: "unordered"}[unordered], func(t *testing.T) {
			l, _, e := coordinatorFixture(t)
			l.b.Unordered = unordered
			if unordered {
				if err := l.Declare(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				l.b.Plan.Save = append(l.b.Plan.Save, protocol.CacheSave{Spec: "go", Key: strings.Repeat("b", 64), Dirs: []string{"go/mod"}})
				addRef(t, l, l.b.Plan.Save[0].Key, cacheArchive(t, "payload", false))
			}
			if err := e.Result(context.Background(), "l", l.b.Epoch); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2200*time.Millisecond)
			defer cancel()
			if err := l.Finalize(ctx, 30*time.Second); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("full grace ended early: %v", err)
			}
		})
	}
}
