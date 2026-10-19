package entries_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

func refusedReason(t *testing.T, err error) string {
	t.Helper()
	var r *entries.Refused
	if !errors.As(err, &r) {
		t.Fatalf("want E711, got %v", err)
	}
	return r.Reason
}

func TestCommitThenLookupHit(t *testing.T) {
	e := newEnv(t, envOpts{})
	m := e.save("a1", trusted, "x", "module cache bytes", "build cache bytes")
	got, ok := e.lookup(trusted, m.Key)
	if !ok {
		t.Fatal("committed entry not found")
	}
	w := got.Writer
	if w.AttemptID != "a1" || w.Epoch != 3 || w.SourceTrust != "owner" || w.SecretsReceived || w.PrivateDeps || w.ScannedClean {
		t.Fatalf("writer facts must come from the store: %+v", w)
	}
	if len(got.Blobs) != 2 || got.Spec != "go" || got.CreatedAt == 0 || got.Bytes() != m.Bytes() {
		t.Fatalf("manifest %+v", got)
	}
	if string(got.Namespace) != string(e.ns(trusted).Canonical()) {
		t.Fatal("namespace not stored byte for byte")
	}
}

func TestCommitIgnoresCallerWriter(t *testing.T) {
	e := newEnv(t, envOpts{})
	key := e.key(untrust1, "w")
	fin := e.attempt("w1", untrust1, []string{key}, true)
	m := e.manifest(untrust1, key, "data")
	m.Writer = entries.Writer{AttemptID: "forged", Epoch: 99, SourceTrust: "owner", ScannedClean: true}
	if err := e.es.Commit(e.ctx, "w1", fin, e.ns(untrust1), m); err != nil {
		t.Fatal(err)
	}
	got, _ := e.lookup(untrust1, key)
	if got.Writer.AttemptID != "w1" || got.Writer.SourceTrust != "untrusted" || got.Writer.Epoch != fin {
		t.Fatalf("forged writer accepted: %+v", got.Writer)
	}
}

func TestCommitRefusalsLeaveNothingVisible(t *testing.T) {
	e := newEnv(t, envOpts{})
	// untrusted attempt into the trusted namespace, and PR 1 into PR 2's: E684
	key := e.key(untrust1, "d")
	fin := e.attempt("d1", untrust1, []string{key}, true)
	pr2 := untrust1
	pr2.PR = "2"
	for name, target := range map[string]entries.Facts{"trusted": trusted, "pr2": pr2} {
		m := e.manifest(target, e.key(target, "d"), "poison")
		err := e.es.Commit(e.ctx, "d1", fin, e.ns(target), m)
		var denied *authz.Denied
		if !errors.As(err, &denied) || denied.Reason != authz.ReasonNamespace {
			t.Fatalf("%s: want E684, got %v", name, err)
		}
		if _, ok := e.lookup(target, m.Key); ok {
			t.Fatalf("%s: a denied commit is visible", name)
		}
	}
	// path outside the entry
	m := e.manifest(untrust1, key, "ok")
	m.Blobs[0].Path = "../escape"
	if got := refusedReason(t, e.es.Commit(e.ctx, "d1", fin, e.ns(untrust1), m)); got != entries.ReasonPathOutside {
		t.Fatal(got)
	}
	// a blob that is not in cas, and one whose size lies
	m = e.manifest(untrust1, key, "ok")
	m.Blobs[0].SHA256 = strings.Repeat("0", 64)
	if err := e.es.Commit(e.ctx, "d1", fin, e.ns(untrust1), m); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing blob: %v", err)
	}
	m = e.manifest(untrust1, key, "ok")
	m.Blobs[0].Size++
	if err := e.es.Commit(e.ctx, "d1", fin, e.ns(untrust1), m); !errors.Is(err, entries.ErrManifest) {
		t.Fatalf("size lie: %v", err)
	}
	// an invalid manifest and an invalid attempt id
	bad := e.manifest(untrust1, key, "ok")
	bad.Key = "short"
	if err := e.es.Commit(e.ctx, "d1", fin, e.ns(untrust1), bad); !errors.Is(err, entries.ErrManifest) {
		t.Fatalf("bad manifest: %v", err)
	}
	if err := e.es.Commit(e.ctx, "bad id", fin, e.ns(untrust1), m); !errors.Is(err, entries.ErrManifest) {
		t.Fatalf("bad attempt id: %v", err)
	}
	if e.count() != 0 {
		t.Fatalf("%d entries visible after refusals", e.count())
	}
	// the same attempt can still save once the request is right
	if err := e.es.Commit(e.ctx, "d1", fin, e.ns(untrust1), e.manifest(untrust1, key, "good")); err != nil {
		t.Fatal(err)
	}
}

func TestCommitSizeCapAndDiskFloor(t *testing.T) {
	e := newEnv(t, envOpts{env: map[string]string{"NSELF_CI_CACHE_MAX_BYTES": "10"}})
	key := e.key(trusted, "s")
	fin := e.attempt("s1", trusted, []string{key}, true)
	if got := refusedReason(t, e.es.Commit(e.ctx, "s1", fin, e.ns(trusted), e.manifest(trusted, key, strings.Repeat("x", 11)))); got != entries.ReasonSizeCap {
		t.Fatal(got)
	}
	if err := e.es.Commit(e.ctx, "s1", fin, e.ns(trusted), e.manifest(trusted, key, strings.Repeat("x", 10))); err != nil {
		t.Fatalf("an entry of exactly the cap must fit: %v", err)
	}
	f := newEnv(t, envOpts{env: map[string]string{"NSELF_CI_STORE_MIN_FREE": "1000"}})
	key = f.key(trusted, "f")
	fin = f.attempt("f1", trusted, []string{key}, true)
	f.free.Store(1000 + 10)
	if got := refusedReason(t, f.es.Commit(f.ctx, "f1", fin, f.ns(trusted), f.manifest(trusted, key, strings.Repeat("y", 11)))); got != entries.ReasonDiskFloor {
		t.Fatal(got)
	}
	if err := f.es.Commit(f.ctx, "f1", fin, f.ns(trusted), f.manifest(trusted, key, strings.Repeat("y", 10))); err != nil {
		t.Fatalf("writing down to exactly the floor must work: %v", err)
	}
	f.free.Store(0)
	if _, ok := f.lookup(trusted, key); !ok {
		t.Fatal("reads are not blocked by the floor")
	}
}

func TestRecordPlanRejectsBadInput(t *testing.T) {
	e := newEnv(t, envOpts{})
	if err := e.es.RecordPlan(e.ctx, "bad id", 0, trusted, nil); !errors.Is(err, entries.ErrManifest) {
		t.Fatalf("attempt id: %v", err)
	}
	if err := e.es.RecordPlan(e.ctx, "ok", 0, trusted, []string{"not-a-key"}); !errors.Is(err, entries.ErrManifest) {
		t.Fatalf("planned key: %v", err)
	}
	if err := e.es.RecordPlan(e.ctx, "ok", 0, trusted, make([]string, 1025)); !errors.Is(err, entries.ErrManifest) {
		t.Fatalf("planned key count: %v", err)
	}
}
