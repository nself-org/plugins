package entries_test

// Purpose: D-1012 regression tests. The blob sweep and Verify must not load every
// entry with all its blobs at once, and a replayed refusal or namespace probe
// must not add an audit row per frame.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// wholeListSpy fails the test when anything asks for every entry (key "") in one
// call: that read holds all blob lists in memory at once.
type wholeListSpy struct {
	entries.Backend
	t *testing.T
}

func (s wholeListSpy) CacheEntries(ctx context.Context, key string) ([]entries.Manifest, error) {
	if key == "" {
		s.t.Error("CacheEntries(\"\") loaded every entry and all its blobs into memory")
	}
	return s.Backend.CacheEntries(ctx, key)
}

// spied builds a second entries.Store over e's database and blobs whose backend is the spy.
func spied(e *env) *entries.Store {
	return entries.New(wholeListSpy{e.st, e.t}, e.blobs, entries.Options{
		Clock:        func() time.Time { return time.Unix(0, e.clock.Load()) },
		Getenv:       func(string) string { return "" },
		DisableFloor: true,
	})
}

func TestSweepStreamsTheReferenceSetInsteadOfLoadingEveryEntry(t *testing.T) {
	e := newEnv(t, envOpts{})
	keep := e.save("s1", trusted, "a", "aaaa", "bbbb")
	orphan := e.put(e.ns(trusted), "x", "never committed")
	e.advance(2 * time.Hour)
	if _, err := spied(e).Prune(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := e.blobs.Stat(cas.CacheRealm(string(keep.Namespace)), orphan.SHA256); err == nil {
		t.Fatal("the unreferenced blob survived the sweep")
	}
	for i := range keep.Blobs {
		if _, err := os.Stat(e.blobPath(keep, i)); err != nil {
			t.Fatalf("a referenced blob was swept: %v", err)
		}
	}
}

func TestVerifyReadsOneKeyAtATime(t *testing.T) {
	e := newEnv(t, envOpts{})
	a := e.save("v1", trusted, "a", "aaaa")
	e.save("v2", trusted, "b", "bbbb")
	if err := os.WriteFile(e.blobPath(a, 0), []byte("zzzz"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := spied(e).Verify(e.ctx)
	if err != nil || rep.Checked != 2 || rep.Dropped != 1 || e.count() != 1 {
		t.Fatalf("verify: %+v %v (%d left)", rep, err, e.count())
	}
}

func TestReplayedRefusalsLeaveOneAuditRowPerAttemptAndReason(t *testing.T) {
	e := newEnv(t, envOpts{})
	key := e.key(trusted, "r")
	fin := e.attempt("r1", trusted, []string{key}, true)
	m := e.manifest(trusted, key, "data")
	for range 5 { // a replaying caller with a stale epoch
		if got := refusedReason(t, e.es.Commit(e.ctx, "r1", fin-1, e.ns(trusted), m)); got != entries.ReasonEpoch {
			t.Fatal(got)
		}
	}
	if n := len(e.audit("cache.commit.refused")); n != 1 {
		t.Fatalf("%d audit rows for one replayed (attempt, reason), want 1", n)
	}
	if got := refusedReason(t, e.es.Commit(e.ctx, "nobody", fin, e.ns(trusted), m)); got != entries.ReasonAttemptState {
		t.Fatal(got)
	}
	if got := refusedReason(t, e.es.Commit(e.ctx, "nobody", fin, e.ns(trusted), m)); got != entries.ReasonAttemptState {
		t.Fatal(got)
	}
	if n := len(e.audit("cache.commit.refused")); n != 2 {
		t.Fatalf("%d audit rows, want one per (attempt, reason) = 2", n)
	}
	// the window ends with usage retention: pruning forgets it and the next refusal is recorded again
	e.advance(entries.UsageRetention + time.Hour)
	if _, err := e.es.Prune(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	refusedReason(t, e.es.Commit(e.ctx, "r1", fin-1, e.ns(trusted), m))
	if n := len(e.audit("cache.commit.refused")); n != 3 {
		t.Fatalf("%d audit rows after the window, want 3", n)
	}
}

func TestReplayedNamespaceProbesLeaveOneAuditRowPerKey(t *testing.T) {
	e := newEnv(t, envOpts{})
	m := e.save("p1", trusted, "p", "aaaa")
	other := e.save("p2", trusted, "q", "bbbb")
	ns := e.ns(untrust1)
	for range 4 {
		for _, k := range []string{m.Key, other.Key} {
			if _, ok, err := e.es.Lookup(e.ctx, ns, k, authz.ModeRWOwn); ok || err != nil {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
		}
	}
	got := e.audit("cache.lookup.namespace_mismatch")
	if len(got) != 2 || got[0].Target == got[1].Target || !strings.Contains(got[0].Target+got[1].Target, m.Key) {
		t.Fatalf("want one audit row per probed key: %+v", got)
	}
}
