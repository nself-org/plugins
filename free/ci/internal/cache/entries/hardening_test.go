package entries_test

import (
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// TestLookupNeverDropsAHealthyEntryDuringAnIdenticalRePut: re-Putting the same
// bytes (the refresh BlobMaxAge requires) replaces the blob file atomically; a
// concurrent Lookup may see cas's race guards fire and must report a miss at
// worst, never drop the entry or write a "corrupt" audit row.
func TestLookupNeverDropsAHealthyEntryDuringAnIdenticalRePut(t *testing.T) {
	e := newEnv(t, envOpts{})
	content := strings.Repeat("x", 4096)
	m := e.save("rp", trusted, "k", content)
	realm := cas.CacheRealm(string(m.Namespace))
	var stop atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for !stop.Load() {
			_, _, _ = e.blobs.Put(e.ctx, realm, strings.NewReader(content))
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, err := e.es.Lookup(e.ctx, e.ns(trusted), m.Key, authz.ModeRWOwn); err != nil {
			t.Error(err)
			break
		}
		if e.count() != 1 {
			break
		}
	}
	stop.Store(true)
	<-done
	if e.count() != 1 || len(e.audit("cache.entry.dropped")) != 0 {
		t.Fatalf("a healthy entry was dropped by a race: %d entries, audit %+v", e.count(), e.audit("cache.entry.dropped"))
	}
	if _, ok := e.lookup(trusted, m.Key); !ok {
		t.Fatal("the entry must be served once the re-Put is over")
	}
}

// TestBlobMaxAgeStaysInsideCasGrace pins cas's one hour sweep grace and checks that
// BlobMaxAge leaves a clear margin under it: a commit-accepted blob can never be
// old enough for a sweep that started before the commit.
func TestBlobMaxAgeStaysInsideCasGrace(t *testing.T) {
	e := newEnv(t, envOpts{})
	ns := e.ns(trusted)
	aged := func(d time.Duration, content string) entries.Blob {
		b := e.put(ns, "x", content)
		when := time.Now().Add(-d)
		if err := os.Chtimes(e.blobPath(entries.Manifest{Namespace: ns.Canonical(), Blobs: []entries.Blob{b}}, 0), when, when); err != nil {
			t.Fatal(err)
		}
		return b
	}
	young, old := aged(59*time.Minute, "59 minutes"), aged(61*time.Minute, "61 minutes")
	if _, err := e.es.Prune(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	realm := cas.CacheRealm(string(ns.Canonical()))
	if _, err := e.blobs.Stat(realm, young.SHA256); err != nil {
		t.Fatalf("cas swept a blob inside its grace: %v", err)
	}
	if _, err := e.blobs.Stat(realm, old.SHA256); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cas kept a blob past its grace (the grace this test pins changed): %v", err)
	}
	if entries.BlobMaxAge > 30*time.Minute {
		t.Fatalf("BlobMaxAge = %v: it must stay at or under half of cas's one hour grace", entries.BlobMaxAge)
	}
}

func TestFreeSpaceProbeDefaultsAndIsNeverSilentlyOff(t *testing.T) {
	e := newEnv(t, envOpts{})
	save := func(es *entries.Store, id string) error {
		key := e.key(trusted, id)
		fin := e.attempt(id, trusted, []string{key}, true)
		return es.Commit(e.ctx, id, fin, e.ns(trusted), e.manifest(trusted, key, "x"))
	}
	env := map[string]string{"NSELF_CI_HOME": e.home, "NSELF_CI_STORE_MIN_FREE": "0"}
	fromEnv := entries.New(e.st, e.blobs, entries.Options{Getenv: func(k string) string { return env[k] }})
	if err := save(fromEnv, "h1"); err != nil {
		t.Fatalf("Home must default to NSELF_CI_HOME: %v", err)
	}
	none := entries.New(e.st, e.blobs, entries.Options{Getenv: func(string) string { return "" }})
	if err := save(none, "h2"); err == nil || !strings.Contains(err.Error(), "no free-space probe") {
		t.Fatalf("with no Home and no Free the floor must not be skipped silently: %v", err)
	}
	if _, err := none.Prune(e.ctx, 0); err == nil {
		t.Fatal("Prune skipped the floor with no probe")
	}
	off := entries.New(e.st, e.blobs, entries.Options{Getenv: func(string) string { return "" }, DisableFloor: true})
	if err := save(off, "h3"); err != nil {
		t.Fatalf("an explicit opt-out works: %v", err)
	}
}

// TestSameKeyStringInAnotherNamespaceNeitherBlocksNorRevealsTheTrustedEntry: a
// planned key that is also a trusted key (a coordinator slip) commits into the
// untrusted attempt's own namespace and cannot squat or probe the trusted entry.
func TestSameKeyStringInAnotherNamespaceNeitherBlocksNorRevealsTheTrustedEntry(t *testing.T) {
	e := newEnv(t, envOpts{})
	trustedEntry := e.save("t1", trusted, "k", "trusted bytes")
	fin := e.attempt("u1", untrust1, []string{trustedEntry.Key}, true)
	mine := e.manifest(untrust1, trustedEntry.Key, "untrusted bytes")
	if err := e.es.Commit(e.ctx, "u1", fin, e.ns(untrust1), mine); err != nil {
		t.Fatalf("the commit must neither fail (an oracle) nor touch the trusted entry: %v", err)
	}
	got, ok := e.lookup(trusted, trustedEntry.Key)
	if !ok || got.Writer.AttemptID != "t1" {
		t.Fatalf("the trusted entry was replaced or lost: %+v %v", got, ok)
	}
	if theirs, ok := e.lookup(untrust1, trustedEntry.Key); !ok || theirs.Writer.AttemptID != "u1" {
		t.Fatalf("the untrusted attempt's own entry: %+v %v", theirs, ok)
	}
	// a later trusted commit of the same key is not blocked either
	fin2 := e.attempt("t2", trusted, []string{trustedEntry.Key}, true)
	if err := e.es.Commit(e.ctx, "t2", fin2, e.ns(trusted), e.manifest(trusted, trustedEntry.Key, "newer trusted bytes")); err != nil {
		t.Fatalf("a squatting entry blocked a trusted commit: %v", err)
	}
}

func TestBlobProblemsAtCommitAreRefusedWithAnAuditRow(t *testing.T) {
	e := newEnv(t, envOpts{})
	key := e.key(trusted, "bi")
	fin := e.attempt("bi1", trusted, []string{key}, true)
	missing := e.manifest(trusted, key, "x")
	missing.Blobs[0].SHA256 = strings.Repeat("0", 64)
	if got := refusedReason(t, e.es.Commit(e.ctx, "bi1", fin, e.ns(trusted), missing)); got != entries.ReasonBlobInvalid {
		t.Fatal(got)
	}
	lie := e.manifest(trusted, key, "x")
	lie.Blobs[0].Size++
	if got := refusedReason(t, e.es.Commit(e.ctx, "bi1", fin, e.ns(trusted), lie)); got != entries.ReasonBlobInvalid {
		t.Fatal(got)
	}
	if n := len(e.audit("cache.commit.refused")); n != 1 { // same attempt and reason: coalesced
		t.Fatalf("%d audit rows, want 1", n)
	}
}
