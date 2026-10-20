//go:build cachesec

package cachesec

// Purpose: TM-12 read-side cases C12-CAC-06 and C12-CAC-07. A read that policy refuses is a MISS in
// production (entries.Lookup returns hit=false, err=nil, no fabric code), so these cases report
// Outcome Miss. Each starts with a positive control on the same fixture (the entry's owner, or a
// clean snapshot, DOES hit), asserts the production access decision (authz.CacheAccess) before the
// lookup, and counts as Landed the entry bytes any attacked lookup returned (0 for a miss).
// Inputs: a real store and cas (newWorld) and a second entries.Store carrying the project switch.
// Outputs: registered cases C12-CAC-06 and C12-CAC-07.
// Constraints: production packages only. The ineligible-snapshot miss appends no audit row (the case
// asserts there is none, so the matrix row leaves audit empty); a key held only by another
// namespace appends cache.lookup.namespace_mismatch, which C12-CAC-07 reports.

import (
	"strings"
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

// g1Lookup reads key in the namespace of f under mode and returns the bytes it served (0 on a miss).
func (w *world) g1Lookup(es *entries.Store, f entries.Facts, key string, mode authz.Mode) (served int64, hit bool) {
	w.t.Helper()
	m, ok, err := es.Lookup(w.ctx, w.NS(f), key, mode)
	if err != nil {
		w.t.Fatal(err)
	}
	return m.Bytes(), ok
}

// g1LookupAudits counts the cache.lookup.* audit rows seen so far.
func (w *world) g1LookupAudits() (n int) {
	for _, r := range w.Audit("") {
		if strings.HasPrefix(r.Action, "cache.lookup.") {
			n++
		}
	}
	return n
}

func init() {
	register("C12-CAC-06", func(t *testing.T) Observed {
		w := newWorld(t)
		es := w.g1Reader(true)
		clean := w.g1Seed(es, "s0", g1Main, "clean", true)
		secret := g1Main
		secret.SecretsDeclared = true
		private := g1Main
		private.PrivateDeps = true
		attacked := map[string]string{
			"secrets declared":  w.g1Seed(es, "s1", secret, "secret", true),
			"private deps":      w.g1Seed(es, "s2", private, "private", true),
			"not scanned clean": w.g1Seed(es, "s3", g1Main, "unscanned", false),
		}

		mode := authz.CacheAccess(w.g1ReaderJob(g1PR1), w.NS(g1Main)) // an untrusted PR job of the same project
		if mode != authz.ModeROSnapshot {
			t.Fatalf("fixture: the PR job gets %q on the protected namespace, want ro-snapshot", mode)
		}
		if _, hit := w.g1Lookup(es, g1Main, clean, mode); !hit { // positive control: a clean snapshot is served
			t.Fatal("control: the clean protected snapshot was not served in ro-snapshot mode")
		}
		var served int64
		for name, key := range attacked {
			if _, hit := w.g1Lookup(es, g1Main, key, authz.ModeRWOwn); !hit { // control: the owner reads its own entry
				t.Fatalf("%s: the entry is not readable by its own namespace; the fixture is broken", name)
			}
			n, hit := w.g1Lookup(es, g1Main, key, mode)
			served += n
			if hit {
				t.Fatalf("%s: the snapshot was served to an untrusted job", name)
			}
		}
		// a project flagged with private dependencies switches snapshots off even for the clean entry
		n, hit := w.g1Lookup(w.g1Reader(false), g1Main, clean, mode)
		served += n
		if hit {
			t.Fatal("project with private deps: a snapshot was served")
		}
		// a job of another project gets no access at all
		foreign := g1PR1
		foreign.Project = "other"
		if got := authz.CacheAccess(w.g1ReaderJob(foreign), w.NS(g1Main)); got != authz.ModeNone {
			t.Fatalf("foreign project access is %q, want none", got)
		}
		n, hit = w.g1Lookup(es, g1Main, clean, authz.ModeNone)
		served += n
		if hit {
			t.Fatal("foreign project: an entry was served with mode none")
		}
		if a := w.g1LookupAudits(); a != 0 {
			t.Fatalf("%d cache.lookup audit rows for ineligible snapshots; production appends none and the row leaves audit empty", a)
		}
		return Observed{Outcome: Miss, Reason: "secret_bearing_snapshot", Landed: served}
	})

	register("C12-CAC-07", func(t *testing.T) Observed {
		w := newWorld(t)
		es := w.g1Reader(true)
		key := w.g1Seed(es, "u1", g1PR1, "pr", false)
		if _, hit := w.g1Lookup(es, g1PR1, key, authz.ModeRWOwn); !hit { // positive control: the PR reads its own entry
			t.Fatal("control: the PR cannot read its own entry")
		}
		mode := authz.CacheAccess(w.g1ReaderJob(g1Main), w.NS(g1PR1)) // a trusted job
		if mode != authz.ModeNone {
			t.Fatalf("a trusted job gets %q on an untrusted namespace, want none", mode)
		}
		var served int64
		for name, f := range map[string]struct {
			f    entries.Facts
			mode authz.Mode
		}{
			"mode none":                 {g1PR1, mode},
			"forced ro-snapshot":        {g1PR1, authz.ModeROSnapshot}, // forcing the mode does not make an untrusted entry a snapshot
			"same key, own namespace":   {g1Main, authz.ModeRWOwn},     // never a fallback to the other namespace
			"same key, forced snapshot": {g1Main, authz.ModeROSnapshot},
		} {
			n, hit := w.g1Lookup(es, f.f, key, f.mode)
			served += n
			if hit {
				t.Fatalf("%s: a trusted job read an untrusted PR's entry", name)
			}
		}
		return Observed{Outcome: Miss, Reason: "untrusted_entry", Audit: w.AuditOf("cache.lookup.namespace_mismatch"), Landed: served}
	})
}
