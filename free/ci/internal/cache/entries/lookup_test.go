package entries_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

func TestLookupMissesAndNeverFallsBack(t *testing.T) {
	e := newEnv(t, envOpts{})
	m := e.save("l1", trusted, "k", "payload")
	trustedNS := e.ns(trusted)
	for name, c := range map[string]struct {
		ns     authz.Namespace
		key    string
		access authz.Mode
	}{
		"access none":              {trustedNS, m.Key, authz.ModeNone},
		"unknown access":           {trustedNS, m.Key, authz.Mode("rw")},
		"unknown key":              {trustedNS, strings.Repeat("0", 64), authz.ModeRWOwn},
		"near-miss key (prefix)":   {trustedNS, m.Key[:63] + "0", authz.ModeRWOwn},
		"truncated key":            {trustedNS, m.Key[:32], authz.ModeRWOwn},
		"upper case key":           {trustedNS, strings.ToUpper(m.Key), authz.ModeRWOwn},
		"invalid namespace":        {authz.Namespace{}, m.Key, authz.ModeRWOwn},
		"empty key":                {trustedNS, "", authz.ModeRWOwn},
		"snapshot by wrong access": {e.ns(untrust1), m.Key, authz.ModeRWOwn},
	} {
		if _, ok, err := e.es.Lookup(e.ctx, c.ns, c.key, c.access); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
	}
	if got := e.audit("cache.lookup.namespace_mismatch"); len(got) != 1 || got[0].Target != m.Key {
		t.Fatalf("a namespace mismatch must leave one audit row: %+v", got)
	}
	if _, ok := e.lookup(trusted, m.Key); !ok || e.count() != 1 {
		t.Fatal("misses must not disturb the entry")
	}
}

func TestLookupDropsCorruptMissingAndTruncated(t *testing.T) {
	e := newEnv(t, envOpts{})
	corrupt := e.save("c1", trusted, "corrupt", "0123456789")
	gone := e.save("c2", trusted, "gone", "abcdefghij")
	short := e.save("c3", trusted, "short", "klmnopqrst")
	keep := e.save("c4", trusted, "keep", "uvwxyz")
	if err := os.WriteFile(e.blobPath(corrupt, 0), []byte("9876543210"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(e.blobPath(gone, 0)); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(e.blobPath(short, 0), 3); err != nil {
		t.Fatal(err)
	}
	for name, m := range map[string]entries.Manifest{"corrupt": corrupt, "missing": gone, "truncated": short} {
		if _, ok := e.lookup(trusted, m.Key); ok {
			t.Fatalf("%s blob served", name)
		}
	}
	if e.count() != 1 {
		t.Fatalf("%d entries left, want only the intact one", e.count())
	}
	if _, ok := e.lookup(trusted, keep.Key); !ok {
		t.Fatal("the intact entry was lost")
	}
	reasons := map[string]bool{}
	for _, r := range e.audit("cache.entry.dropped") {
		reasons[r.Detail] = true
	}
	for _, want := range []string{"reason=corrupt", "reason=missing", "reason=size_mismatch"} {
		if !reasons[want] {
			t.Errorf("no audit row %q (have %v)", want, reasons)
		}
	}
}

func TestLookupTouchesLastUsed(t *testing.T) {
	e := newEnv(t, envOpts{})
	m := e.save("t1", trusted, "k", "x")
	before, _ := e.lookup(trusted, m.Key)
	e.advance(time.Hour)
	e.lookup(trusted, m.Key) // touches at +1h; a lookup returns the row as it was read
	after, _ := e.lookup(trusted, m.Key)
	if after.LastUsedAt-before.LastUsedAt < int64(time.Hour/2) {
		t.Fatalf("last_used_at did not move: %d -> %d", before.LastUsedAt, after.LastUsedAt)
	}
}

func TestSnapshotLookupServesOnlyEligibleEntries(t *testing.T) {
	yes := func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: true} }
	e := newEnv(t, envOpts{project: yes})
	reader := e.ns(untrust1)
	job := authz.Job{Namespace: reader, State: authz.StateRunning}
	mode := authz.CacheAccess(job, e.ns(trusted))
	if mode != authz.ModeROSnapshot {
		t.Fatalf("setup: untrusted reader of the protected owner namespace gets %s", mode)
	}
	save := func(id string, f entries.Facts, scanned bool) entries.Manifest {
		key := e.key(f, id)
		fin := e.attempt(id, f, []string{key}, true)
		m := e.manifest(f, key, id+"-data")
		m.Writer.ScannedClean = scanned
		if err := e.es.Commit(e.ctx, id, fin, e.ns(f), m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	snap := func(m entries.Manifest) bool {
		t.Helper()
		_, ok, err := e.es.Lookup(e.ctx, e.ns(trusted), m.Key, authz.ModeROSnapshot)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	clean := save("s-clean", trusted, true)
	secrets := trusted
	secrets.SecretsDeclared = true
	withSecrets := save("s-secret", secrets, true)
	priv := trusted
	priv.PrivateDeps = true
	withPrivate := save("s-priv", priv, true)
	unscanned := save("s-scan", trusted, false)
	if !snap(clean) {
		t.Fatal("an eligible entry is not served read-only")
	}
	for name, m := range map[string]entries.Manifest{"secrets declared": withSecrets, "private deps": withPrivate, "not scanned clean": unscanned} {
		if snap(m) {
			t.Errorf("%s: served as a snapshot", name)
		}
		if _, ok := e.lookup(secrets, m.Key); name == "secrets declared" && !ok {
			t.Errorf("the owner namespace must still read its own entry")
		}
	}
	// a project flagged with private dependencies switches snapshots off
	off := newEnv(t, envOpts{project: func(string) authz.ProjectFacts { return authz.ProjectFacts{} }})
	m := off.save("o1", trusted, "k", "data")
	if _, ok, _ := off.es.Lookup(off.ctx, off.ns(trusted), m.Key, authz.ModeROSnapshot); ok {
		t.Fatal("snapshot served for a project with private dependencies")
	}
	// no project hook at all fails closed
	none := newEnv(t, envOpts{})
	m = none.save("n1", trusted, "k", "data")
	if _, ok, _ := none.es.Lookup(none.ctx, none.ns(trusted), m.Key, authz.ModeROSnapshot); ok {
		t.Fatal("snapshot served with no project facts")
	}
	// an unprotected namespace is never a snapshot source
	unprot := entries.Facts{Project: "proj", Trust: "owner", Protection: "unprotected"}
	m = e.save("u1", unprot, "k", "data")
	if _, ok, _ := e.es.Lookup(e.ctx, e.ns(unprot), m.Key, authz.ModeROSnapshot); ok {
		t.Fatal("unprotected namespace served as a snapshot")
	}
}

func TestVerifyDropsUnusableEntries(t *testing.T) {
	e := newEnv(t, envOpts{})
	a := e.save("v1", trusted, "a", "aaaa")
	b := e.save("v2", trusted, "b", "bbbb")
	e.save("v3", trusted, "c", "cccc")
	if err := os.WriteFile(e.blobPath(a, 0), []byte("zzzz"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(e.blobPath(b, 0)); err != nil {
		t.Fatal(err)
	}
	rep, err := e.es.Verify(e.ctx)
	if err != nil || rep.Checked != 3 || rep.Dropped != 2 || e.count() != 1 {
		t.Fatalf("verify: %+v %v (%d left)", rep, err, e.count())
	}
}

// TestCasReportsACorruptBlobAsE710: the integrity failure Lookup turns into a drop.
func TestCasReportsACorruptBlobAsE710(t *testing.T) {
	e := newEnv(t, envOpts{})
	m := e.save("e710", trusted, "k", "0123456789")
	if err := os.WriteFile(e.blobPath(m, 0), []byte("9876543210"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := e.blobs.Open(cas.CacheRealm(string(m.Namespace)), m.Blobs[0].SHA256)
	var ce *cas.Error
	if !errors.Is(err, cas.ErrDigestMismatch) || !errors.As(err, &ce) || ce.Code() != "E710" {
		t.Fatalf("want E710 digest_mismatch, got %v", err)
	}
}
