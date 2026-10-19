package entries_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

func TestPruneByteCapEvictsLeastRecentlyUsed(t *testing.T) {
	e := newEnv(t, envOpts{env: map[string]string{"NSELF_CI_CACHE_MAX_BYTES": "25"}})
	a := e.save("a", trusted, "a", strings.Repeat("a", 10))
	e.advance(time.Minute)
	b := e.save("b", trusted, "b", strings.Repeat("b", 10))
	e.advance(time.Minute)
	c := e.save("c", trusted, "c", strings.Repeat("c", 10))
	e.advance(time.Minute)
	e.lookup(trusted, a.Key) // a is now the most recently used
	e.advance(time.Minute)
	res, err := e.es.Prune(e.ctx, 0)
	if err != nil || res.Entries != 1 || res.Bytes != 10 || res.More {
		t.Fatalf("prune: %+v %v", res, err)
	}
	if _, ok := e.lookup(trusted, b.Key); ok {
		t.Fatal("the least recently used entry survived")
	}
	for _, m := range []entries.Manifest{a, c} {
		if _, ok := e.lookup(trusted, m.Key); !ok {
			t.Fatal("a more recently used entry was evicted")
		}
	}
	// the evicted blob stays through the one hour grace, then is swept
	if e.blobFiles() != 3 {
		t.Fatalf("blob swept inside the grace window: %d files", e.blobFiles())
	}
	e.advance(2 * time.Hour)
	if _, err = e.es.Prune(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	if e.blobFiles() != 2 {
		t.Fatalf("unreferenced blob not swept: %d files", e.blobFiles())
	}
	for _, m := range []entries.Manifest{a, c} {
		if _, ok := e.lookup(trusted, m.Key); !ok {
			t.Fatal("a referenced blob was swept")
		}
	}
}

func TestPruneAgeCap(t *testing.T) {
	e := newEnv(t, envOpts{env: map[string]string{"NSELF_CI_CACHE_MAX_AGE": "1h"}})
	old := e.save("old", trusted, "old", "old data")
	e.advance(30 * time.Minute)
	young := e.save("young", trusted, "young", "young data")
	e.advance(40 * time.Minute)
	res, err := e.es.Prune(e.ctx, 0)
	if err != nil || res.Entries != 1 {
		t.Fatalf("prune: %+v %v", res, err)
	}
	if _, ok := e.lookup(trusted, old.Key); ok {
		t.Fatal("an entry past the age cap survived")
	}
	if _, ok := e.lookup(trusted, young.Key); !ok {
		t.Fatal("an entry inside the age cap was dropped")
	}
	// an entry used exactly now is never aged out
	if res, err = e.es.Prune(e.ctx, 0); err != nil || res.Entries != 0 {
		t.Fatalf("second prune: %+v %v", res, err)
	}
}

func TestPruneKeepsOrphanBlobsOfOtherKindsAndGraceWindow(t *testing.T) {
	e := newEnv(t, envOpts{})
	ns := e.ns(trusted)
	orphan := e.put(ns, "x", "never committed")
	art, _, err := e.blobs.Put(e.ctx, cas.ArtifactRealm("proj"), strings.NewReader("artifact bytes"))
	if err != nil {
		t.Fatal(err)
	}
	e.advance(2 * time.Hour)
	if _, err = e.es.Prune(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = e.blobs.Stat(cas.CacheRealm(string(ns.Canonical())), orphan.SHA256); err == nil {
		t.Fatal("an old unreferenced cache blob survived the sweep")
	}
	if _, err = e.blobs.Stat(cas.ArtifactRealm("proj"), art); err != nil {
		t.Fatalf("the cache sweep touched an artifact blob: %v", err)
	}
}

func TestPruneExpiresUsageRows(t *testing.T) {
	e := newEnv(t, envOpts{})
	use := entries.Use{Attempt: "u1", Spec: "go", Key: strings.Repeat("a", 64), Result: "hit", Bytes: 5, MS: 3}
	if err := e.es.RecordUse(e.ctx, use); err != nil {
		t.Fatal(err)
	}
	e.advance(29 * 24 * time.Hour)
	if _, err := e.es.Prune(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	if rows, _ := e.es.Usage(e.ctx, "u1"); len(rows) != 1 {
		t.Fatalf("usage row expired early: %d", len(rows))
	}
	e.advance(2 * 24 * time.Hour)
	if _, err := e.es.Prune(e.ctx, 0); err != nil {
		t.Fatal(err)
	}
	if rows, _ := e.es.Usage(e.ctx, "u1"); len(rows) != 0 {
		t.Fatalf("usage row older than 30 days kept: %d", len(rows))
	}
}

func TestPruneEnforcesTheFreeSpaceFloor(t *testing.T) {
	e := newEnv(t, envOpts{env: map[string]string{"NSELF_CI_STORE_MIN_FREE": "100"}})
	var all []entries.Manifest
	for i := 0; i < 8; i++ {
		all = append(all, e.save("f"+string(rune('a'+i)), trusted, string(rune('a'+i)), strings.Repeat("z", 10)))
		e.advance(time.Minute)
	}
	// free space grows 10 bytes per evicted entry; the floor needs 6 evictions
	e.freeFn = func() int64 { return 40 + 10*int64(8-e.count()) }
	e.advance(2 * time.Hour)
	res, err := e.es.Prune(e.ctx, 0)
	if err != nil || res.Entries != 6 || res.Bytes != 60 {
		t.Fatalf("prune: %+v %v", res, err)
	}
	for i, m := range all {
		if _, ok := e.lookup(trusted, m.Key); ok != (i >= 6) {
			t.Errorf("entry %d present=%v: eviction was not least recently used first", i, ok)
		}
	}
	// when evicting everything cannot reach the floor, Prune says so
	e.freeFn = func() int64 { return 0 }
	_, err = e.es.Prune(e.ctx, 0)
	var r *entries.Refused
	if !errors.As(err, &r) || r.Reason != entries.ReasonDiskFloor || e.count() != 0 {
		t.Fatalf("floor error: %v (%d entries left)", err, e.count())
	}
	if len(e.audit("cache.prune.refused")) != 1 {
		t.Fatal("no audit row for the unmet floor")
	}
}

func TestStatfsFloorWithoutHook(t *testing.T) {
	e := newEnv(t, envOpts{})
	ns := e.ns(trusted)
	for floor, wantReason := range map[string]string{"0": "", "9223372036854775807": entries.ReasonDiskFloor} {
		es := entries.New(e.st, e.blobs, entries.Options{Home: e.home, Getenv: func(k string) string {
			if k == "NSELF_CI_STORE_MIN_FREE" {
				return floor
			}
			return ""
		}})
		id := "sf" + floor[:1] + string(rune('a'+len(floor)))
		key := e.key(trusted, id)
		fin := e.attempt(id, trusted, []string{key}, true)
		err := es.Commit(e.ctx, id, fin, ns, e.manifest(trusted, key, "x"))
		if wantReason == "" && err != nil {
			t.Fatalf("floor %s: %v", floor, err)
		}
		if wantReason != "" && refusedReason(t, err) != wantReason {
			t.Fatalf("floor %s: %v", floor, err)
		}
	}
	// the default floor is derived from the volume and never disables the check
	def := entries.New(e.st, e.blobs, entries.Options{Home: e.home, Getenv: func(string) string { return "" }})
	if _, err := def.Prune(e.ctx, 0); err != nil && !errors.As(err, new(*entries.Refused)) {
		t.Fatal(err)
	}
	if _, err := entries.New(e.st, e.blobs, entries.Options{Home: "/nonexistent-nself-home"}).Prune(e.ctx, 0); err == nil {
		t.Fatal("a failing free-space probe must surface")
	}
	// no Home and no hook: the check is off
	off := entries.New(e.st, e.blobs, entries.Options{Getenv: func(string) string { return "" }})
	key := e.key(trusted, "off")
	fin := e.attempt("off", trusted, []string{key}, true)
	if err := off.Commit(e.ctx, "off", fin, ns, e.manifest(trusted, key, "x")); err != nil {
		t.Fatal(err)
	}
}

func TestEnvCaps(t *testing.T) {
	e := newEnv(t, envOpts{})
	if b, a := e.es.Caps(); b != 10<<30 || a != 336*time.Hour {
		t.Fatalf("defaults: %d %v", b, a)
	}
	e = newEnv(t, envOpts{env: map[string]string{"NSELF_CI_CACHE_MAX_BYTES": "4096", "NSELF_CI_CACHE_MAX_AGE": "90m"}})
	if b, a := e.es.Caps(); b != 4096 || a != 90*time.Minute || e.warn.Len() != 0 {
		t.Fatalf("parsed: %d %v warn=%q", b, a, e.warn.String())
	}
	for _, bad := range []map[string]string{
		{"NSELF_CI_CACHE_MAX_BYTES": "ten"}, {"NSELF_CI_CACHE_MAX_BYTES": "0"}, {"NSELF_CI_CACHE_MAX_BYTES": "-5"},
		{"NSELF_CI_CACHE_MAX_AGE": "soon"}, {"NSELF_CI_CACHE_MAX_AGE": "0s"}, {"NSELF_CI_CACHE_MAX_AGE": "-1h"},
		{"NSELF_CI_STORE_MIN_FREE": "lots"}, {"NSELF_CI_STORE_MIN_FREE": "-1"},
	} {
		e = newEnv(t, envOpts{env: bad})
		b, a := e.es.Caps()
		if b != 10<<30 || a != 336*time.Hour {
			t.Errorf("%v: unparsable value changed the default (%d %v)", bad, b, a)
		}
		if strings.Count(e.warn.String(), "\n") != 1 {
			t.Errorf("%v: want exactly one warning, got %q", bad, e.warn.String())
		}
	}
}

// commitWithFree reports whether a save is accepted when free bytes are free.
func commitWithFree(t *testing.T, e *env, id string, free int64) bool {
	t.Helper()
	key := e.key(trusted, id)
	fin := e.attempt(id, trusted, []string{key}, true)
	e.free.Store(free)
	err := e.es.Commit(e.ctx, id, fin, e.ns(trusted), e.manifest(trusted, key, strings.Repeat("q", 10)))
	if err != nil && refusedReason(t, err) != entries.ReasonDiskFloor {
		t.Fatal(err)
	}
	return err == nil
}

func TestDefaultFloorIsTheLargerOfFiveGiBAndATenthOfTheVolume(t *testing.T) {
	const gib = int64(1) << 30
	e := newEnv(t, envOpts{})
	e.total.Store(100 * gib) // a tenth is 10 GiB
	if commitWithFree(t, e, "d1", 10*gib-1) || !commitWithFree(t, e, "d2", 10*gib) {
		t.Fatal("on a 100 GiB volume the floor is 10 GiB")
	}
	e.total.Store(10 * gib) // a tenth is 1 GiB, so 5 GiB wins
	if commitWithFree(t, e, "d3", 5*gib-1) || !commitWithFree(t, e, "d4", 5*gib) {
		t.Fatal("on a small volume the floor is 5 GiB")
	}
}

func TestExplicitZeroFloorIsAccepted(t *testing.T) {
	e := newEnv(t, envOpts{env: map[string]string{"NSELF_CI_STORE_MIN_FREE": "0"}})
	if !commitWithFree(t, e, "z1", 0) {
		t.Fatal("a zero floor never refuses")
	}
	if e.warn.Len() != 0 {
		t.Fatalf("0 is a valid floor but warned: %q", e.warn.String())
	}
}

func TestCommitAcceptsExactlyTheMaximumPlannedKeys(t *testing.T) {
	e := newEnv(t, envOpts{})
	keys := make([]string, 1024)
	for i := range keys {
		keys[i] = e.key(trusted, "k"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+strings.Repeat("y", i/26))
	}
	if err := e.es.RecordPlan(e.ctx, "nope", 0, trusted, keys); refusedReason(t, err) != entries.ReasonAttemptState {
		t.Fatalf("1024 keys must pass validation and reach the backend: %v", err)
	}
}

func TestPruneBudgetBoundsTheWholeOperation(t *testing.T) {
	e := newEnv(t, envOpts{})
	for i := 0; i < 3; i++ {
		e.save("b"+string(rune('a'+i)), trusted, string(rune('a'+i)), "data")
	}
	e.advance(2 * time.Hour)
	res, err := e.es.Prune(e.ctx, time.Nanosecond) // already over budget when the first stage starts
	if err != nil || !res.More {
		t.Fatalf("an exhausted budget is not an error: %+v %v", res, err)
	}
	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	if _, err = e.es.Prune(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled caller context is an error: %v", err)
	}
	if res, err = e.es.Prune(e.ctx, time.Minute); err != nil || res.More {
		t.Fatalf("a roomy budget completes: %+v %v", res, err)
	}
}

func TestPruneReportsTheCallersOwnDeadlineAsAnError(t *testing.T) {
	e := newEnv(t, envOpts{})
	ctx, cancel := context.WithDeadline(e.ctx, time.Now().Add(-time.Second))
	defer cancel()
	if _, err := e.es.Prune(ctx, time.Hour); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the caller's own expired deadline is an error, not More: %v", err)
	}
}

func TestKeyRefusesToExpandOversizedInputs(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	refs := make([]string, 1<<16) // 65 536 references to one 1 MiB string: 64 GiB if expanded
	for i := range refs {
		refs[i] = big
	}
	ns, err := authz.CacheNamespace("proj", "owner", authz.Protected, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entries.Key(entries.KeyInputs{Namespace: ns, Paths: refs}); err == nil {
		t.Fatal("inputs far over the limit were accepted")
	}
}
