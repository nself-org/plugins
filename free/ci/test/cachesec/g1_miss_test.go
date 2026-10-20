//go:build cachesec

package cachesec

// Purpose: TM-12 read-side cases. A read that policy refuses is a MISS in production (entries.Lookup
// returns hit=false, err=nil; it carries no fabric code and, for an ineligible snapshot, writes no
// audit row), so it cannot be a matrix row (a refused row needs a code and an audit action). These
// are plain tests with the same discipline: a positive control on the same fixture, the production
// access decision asserted first, and the miss asserted second.
// Inputs: a real store and cas (newWorld) and a second entries.Store with the project switch.
// Outputs: TestCacheSecMiss_SecretBearingSnapshot, TestCacheSecMiss_TrustedReadsUntrusted.
// Constraints: production packages only. Run them with
//   go test -tags 'cachesec integration' -run TestCacheSecMiss ./test/cachesec/

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// g1Reader is an entries.Store over the world's backend with the project switch set.
func (w *world) g1Reader(noPrivateDeps bool) *entries.Store {
	return entries.New(w.St, w.Blobs, entries.Options{Home: w.Home, Getenv: func(string) string { return "" },
		Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: noPrivateDeps} }})
}

// g1Seed commits one entry through a passing attempt of f; scanned is the saver's ScannedClean.
func (w *world) g1Seed(es *entries.Store, id string, f entries.Facts, extra string, scanned bool) string {
	w.t.Helper()
	key := w.Key(f, extra)
	fin := w.Attempt(id, f, []string{key}, true)
	m := w.Manifest(f, key, "entry bytes of "+id)
	m.Writer.ScannedClean = scanned
	if err := es.Commit(w.ctx, id, fin, w.NS(f), m); err != nil {
		w.t.Fatalf("seed %s: %v", id, err)
	}
	return key
}

// g1ReaderJob is a running attempt of f, as the lookup caller derives its access.
func (w *world) g1ReaderJob(f entries.Facts) authz.Job {
	return authz.Job{Namespace: w.NS(f), State: authz.StateRunning, Capability: authz.Capability{Attempt: "rd", Epoch: 2}}
}

func (w *world) g1Hit(es *entries.Store, f entries.Facts, key string, mode authz.Mode) bool {
	w.t.Helper()
	_, ok, err := es.Lookup(w.ctx, w.NS(f), key, mode)
	if err != nil {
		w.t.Fatal(err)
	}
	return ok
}

func TestCacheSecMiss_SecretBearingSnapshot(t *testing.T) {
	w := newWorld(t)
	es := w.g1Reader(true)
	clean := w.g1Seed(es, "s0", g1Main, "clean", true)
	secret := g1Main
	secret.SecretsDeclared = true
	keySecret := w.g1Seed(es, "s1", secret, "secret", true)
	private := g1Main
	private.PrivateDeps = true
	keyPrivate := w.g1Seed(es, "s2", private, "private", true)
	keyUnscanned := w.g1Seed(es, "s3", g1Main, "unscanned", false)

	job := w.g1ReaderJob(g1PR1) // an untrusted PR attempt of the same project
	mode := authz.CacheAccess(job, w.NS(g1Main))
	if mode != authz.ModeROSnapshot {
		t.Fatalf("fixture: the PR job gets %q on the protected namespace, want ro-snapshot", mode)
	}
	if !w.g1Hit(es, g1Main, clean, mode) { // positive control: a clean snapshot is served
		t.Fatal("control: the clean protected snapshot was not served in ro-snapshot mode")
	}
	for name, key := range map[string]string{"secrets declared": keySecret, "private deps": keyPrivate, "not scanned clean": keyUnscanned} {
		if !w.g1Hit(es, g1Main, key, authz.ModeRWOwn) { // the entry exists and is intact for its owner
			t.Fatalf("%s: the entry is not readable by its own namespace; the fixture is broken", name)
		}
		if w.g1Hit(es, g1Main, key, mode) {
			t.Errorf("%s: the snapshot was served to an untrusted job", name)
		}
	}
	// a project flagged with private dependencies switches snapshots off even for the clean entry
	if w.g1Hit(w.g1Reader(false), g1Main, clean, mode) {
		t.Error("project with private deps: a snapshot was served")
	}
	// a job of another project gets no access at all
	foreign := g1PR1
	foreign.Project = "other"
	if got := authz.CacheAccess(w.g1ReaderJob(foreign), w.NS(g1Main)); got != authz.ModeNone {
		t.Fatalf("foreign project access is %q, want none", got)
	}
	if w.g1Hit(es, g1Main, clean, authz.ModeNone) {
		t.Error("foreign project: an entry was served with mode none")
	}
}

func TestCacheSecMiss_TrustedReadsUntrusted(t *testing.T) {
	w := newWorld(t)
	es := w.g1Reader(true)
	key := w.g1Seed(es, "u1", g1PR1, "pr", false)
	if !w.g1Hit(es, g1PR1, key, authz.ModeRWOwn) { // positive control: the PR reads its own entry
		t.Fatal("control: the PR cannot read its own entry")
	}
	trusted := w.g1ReaderJob(g1Main)
	mode := authz.CacheAccess(trusted, w.NS(g1PR1))
	if mode != authz.ModeNone {
		t.Fatalf("a trusted job gets %q on an untrusted namespace, want none", mode)
	}
	if w.g1Hit(es, g1PR1, key, mode) {
		t.Error("a trusted job read an untrusted PR's entry")
	}
	if w.g1Hit(es, g1PR1, key, authz.ModeROSnapshot) { // forcing the mode does not make an untrusted entry a snapshot
		t.Error("an untrusted entry was served as a snapshot")
	}
	// the same key string looked up in the trusted namespace is a miss with an audit row, never a fallback
	if w.g1Hit(es, g1Main, key, authz.ModeRWOwn) {
		t.Error("the trusted namespace answered with the untrusted entry")
	}
	if w.AuditOf("cache.lookup.namespace_mismatch") == "" {
		t.Error("no cache.lookup.namespace_mismatch audit row")
	}
}
