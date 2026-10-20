//go:build security

package redteam

// Purpose: red team attacks on cache poisoning (TM-12) and on stale or zombie writes (TM-13):
// namespace and key collisions, forging the manifest namespace, restore-key style fallback,
// writes with the wrong attempt state, epoch or result.
// Inputs:  the real entries.Store over a real sqlite store in a temp dir.
// Outputs: pass or fail; a pass means the attack failed.
// Constraints: local only; blobs are small synthetic strings.

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestB_Cache_NamespaceCollisionMatrix(t *testing.T) {
	type tuple struct {
		class     model.TrustClass
		prot      authz.Protection
		project   string
		head, pr  string
		wantValid bool
	}
	nfc, nfd := "café", "café"
	tuples := []tuple{
		{"owner", authz.Protected, "proj", "", "", true},
		{"owner", authz.Unprotected, "proj", "", "", true},
		{"internal", authz.Protected, "proj", "", "", true},
		{"untrusted", authz.Unprotected, "proj", "fork/x", "1", true},
		{"untrusted", authz.Unprotected, "proj", "fork/x", "11", true},
		{"untrusted", authz.Unprotected, "proj", "fork/x1", "1", true},
		{"untrusted", authz.Unprotected, "proj", "fork/", "x1", true},
		{"untrusted", authz.Unprotected, "proj", "fork/x\x001", "", false},
		{"untrusted", authz.Unprotected, "pr", "oj", "1", true},
		{"untrusted", authz.Unprotected, "pro", "j", "1", true},
		{"owner", authz.Protected, "proj|protected", "", "", true},
		{"owner", authz.Protected, "proj/..", "", "", true},
		{"owner", authz.Protected, "../proj", "", "", true},
		{"owner", authz.Protected, "Proj", "", "", true},
		{"owner", authz.Protected, "proj ", "", "", true},
		{"owner", authz.Protected, "proj.", "", "", true},
		{"owner", authz.Protected, nfc, "", "", true},
		{"owner", authz.Protected, nfd, "", "", true},
		{"untrusted", authz.Protected, "proj", "fork/x", "1", true}, // protection ignored for untrusted
	}
	seen := map[string]int{}
	for i, tp := range tuples {
		ns, err := authz.CacheNamespace(tp.project, tp.class, tp.prot, tp.head, tp.pr)
		if err != nil {
			continue
		}
		c := string(ns.Canonical())
		if j, dup := seen[c]; dup && !(i == len(tuples)-1 && j == 3) {
			t.Errorf("tuples %d and %d share one canonical namespace", j, i)
		}
		seen[c] = i
	}
	// the last tuple is the same namespace as tuple 3 on purpose (protection is ignored); it must not
	// have become protected.
	ns, _ := authz.CacheNamespace("proj", "untrusted", authz.Protected, "fork/x", "1")
	if ns.Protection != authz.Unprotected {
		t.Errorf("an untrusted namespace was built protected: %+v", ns)
	}
}

func TestB_Cache_TrustSpellingNeverReachesTrustedNamespace(t *testing.T) {
	// positive control: the canonical spelling builds, and owner is a snapshot source (so the checks below can fail)
	if own, err := authz.CacheNamespace("proj", "owner", authz.Protected, "", ""); err != nil || !authz.IsSnapshotSource(own) {
		t.Fatalf("control: canonical owner namespace err=%v snapshot source=%v", err, err == nil && authz.IsSnapshotSource(own))
	}
	for _, spell := range []string{"", " owner", "owner ", "Owner", "OWNER", "owner\x00", "ownеr", "internal ", "Internal", "root", "trusted", "member", "admin"} {
		ns, err := authz.CacheNamespace("proj", model.TrustClass(spell), authz.Protected, "", "")
		if err != nil {
			continue // refused: fine
		}
		real, _ := authz.CacheNamespace("proj", "owner", authz.Protected, "", "")
		if ns.Equal(real) && spell != "owner" {
			t.Errorf("spelling %q lands in the owner protected namespace", spell)
		}
		if authz.IsSnapshotSource(ns) && spell != "owner" && spell != "internal" {
			t.Errorf("spelling %q (trust %q) became a snapshot source", spell, ns.Trust)
		}
	}
	// a Namespace literal with a mis-cased class must not validate as a snapshot source either.
	lit := authz.Namespace{Project: "proj", Trust: "OWNER", Protection: authz.Protected}
	if authz.IsSnapshotSource(lit) {
		t.Errorf("literal namespace with trust OWNER is a snapshot source")
	}
}

func TestB_Cache_KeyFieldShiftingNeverCollides(t *testing.T) {
	e := newCacheEnv(t)
	ns := e.ns(factsMain)
	base := entries.KeyInputs{Namespace: ns, SpecID: "go", SpecVersion: "1", Platform: "p", Isolation: "container"}
	variants := []entries.KeyInputs{base}
	add := func(mod func(k *entries.KeyInputs)) {
		k := base
		mod(&k)
		variants = append(variants, k)
	}
	add(func(k *entries.KeyInputs) { k.Extras = []string{"x"} })
	add(func(k *entries.KeyInputs) { k.Paths = []string{"x"} })
	add(func(k *entries.KeyInputs) { k.Lockfiles = []string{"x"} })
	add(func(k *entries.KeyInputs) { k.Toolchain = []string{"x"} })
	add(func(k *entries.KeyInputs) { k.EnvDigest = "x" })
	add(func(k *entries.KeyInputs) { k.Paths = []string{"a", "b"} })
	add(func(k *entries.KeyInputs) { k.Paths = []string{"ab"} })
	add(func(k *entries.KeyInputs) { k.Paths = []string{"a"}; k.Lockfiles = []string{"b"} })
	add(func(k *entries.KeyInputs) { k.SpecID = "go1"; k.SpecVersion = "" })
	add(func(k *entries.KeyInputs) { k.SpecID = "g"; k.SpecVersion = "o1" })
	add(func(k *entries.KeyInputs) { k.Isolation = "Container" })
	add(func(k *entries.KeyInputs) { k.Namespace = e.ns(factsPR1) })
	add(func(k *entries.KeyInputs) { k.Extras = []string{""} })
	add(func(k *entries.KeyInputs) { k.Extras = []string{"", ""} })
	add(func(k *entries.KeyInputs) { k.Extras = []string{"x\x00"} })
	seen := map[string]int{}
	for i, v := range variants {
		k, err := entries.Key(v)
		if err != nil {
			t.Fatal(err)
		}
		if j, dup := seen[k]; dup {
			t.Errorf("key inputs %d and %d collide: %+v / %+v", j, i, variants[j], v)
		}
		seen[k] = i
	}
}

// An untrusted attempt that finished a pass claims the trusted namespace, or writes a manifest
// stamped with the trusted namespace: neither may land under main's key.
func TestB_Cache_UntrustedCannotWriteMainNamespace(t *testing.T) {
	e := newCacheEnv(t)
	mainKey := e.key(factsMain, "k")
	e.start("evil", factsPR1)
	e.finish("evil", factsPR1, []string{mainKey}, true)

	// (a) claim main's namespace
	m := e.manifest(factsMain, factsMain, mainKey, "poison-a")
	if err := e.es.Commit(e.ctx, "evil", 3, e.ns(factsMain), m); err == nil {
		t.Fatal("untrusted attempt committed with a claimed trusted namespace")
	}
	// (b) claim own namespace but stamp the manifest with main's
	if err := e.es.Commit(e.ctx, "evil", 3, e.ns(factsPR1), m); err == nil {
		t.Fatal("manifest stamped with the trusted namespace was accepted under an untrusted claim")
	}
	if e.lookup(factsMain, mainKey, authz.ModeRWOwn) {
		t.Fatal("main's namespace holds an entry after an untrusted write")
	}
	// (c) honest write into the PR namespace under main's key string stays invisible to main.
	m2 := e.manifest(factsPR1, factsPR1, mainKey, "poison-c")
	if err := e.es.Commit(e.ctx, "evil", 3, e.ns(factsPR1), m2); err != nil {
		t.Fatalf("honest PR write refused: %v", err)
	}
	if e.lookup(factsMain, mainKey, authz.ModeRWOwn) {
		t.Fatal("PR entry visible to main under the same key string")
	}
	if e.lookup(factsPR2, mainKey, authz.ModeRWOwn) {
		t.Fatal("PR 1 entry visible to PR 2 under the same key string")
	}
}

// Restore-key style fallback: prefixes, truncations, case changes and padding of a key.
func TestB_Cache_NoPrefixOrFuzzyKeyFallback(t *testing.T) {
	e := newCacheEnv(t)
	k := e.key(factsMain, "k")
	e.start("good", factsMain)
	e.finish("good", factsMain, []string{k}, true)
	if err := e.es.Commit(e.ctx, "good", 3, e.ns(factsMain), e.manifest(factsMain, factsMain, k, "data")); err != nil {
		t.Fatal(err)
	}
	if !e.lookup(factsMain, k, authz.ModeRWOwn) {
		t.Fatal("control: exact lookup misses")
	}
	for name, probe := range map[string]string{
		"prefix": k[:32], "upper": strings.ToUpper(k), "padded": k + "0", "space": " " + k, "empty": "", "wildcard": "%", "like": k[:10] + "%",
		"dotdot": "../" + k, "nul": k[:63] + "\x00",
	} {
		if e.lookup(factsMain, probe, authz.ModeRWOwn) {
			t.Errorf("%s probe %q hit an entry", name, probe)
		}
	}
	// the invalid modes never hit
	for _, mode := range []authz.Mode{authz.ModeNone, "", "rw", "RW-OWN"} {
		if e.lookup(factsMain, k, mode) {
			t.Errorf("mode %q read the entry", mode)
		}
	}
	// a PR reading main in snapshot mode is allowed only when the writer facts allow it; a writer
	// that received secrets is never served.
	if e.lookup(factsPR1, k, authz.ModeROSnapshot) {
		t.Errorf("ro-snapshot served an entry although no project facts allow snapshots")
	}
}

func TestB_Cache_StaleAndZombieWrites(t *testing.T) {
	e := newCacheEnv(t)
	k := e.key(factsMain, "z")
	e.start("zomb", factsMain)
	// before finalizing: a commit while running is refused
	if err := e.es.Commit(e.ctx, "zomb", 1, e.ns(factsMain), e.manifest(factsMain, factsMain, k, "early")); err == nil {
		t.Fatal("commit while running accepted")
	}
	e.finish("zomb", factsMain, []string{k}, true)
	m := e.manifest(factsMain, factsMain, k, "late")
	for _, ep := range []int64{0, 1, 2, 4, -1, 1 << 40} {
		err := e.es.Commit(e.ctx, "zomb", ep, e.ns(factsMain), m)
		if err == nil {
			t.Fatalf("commit at stale or foreign epoch %d accepted", ep)
		}
		var r *entries.Refused
		var d *authz.Denied
		if !errors.As(err, &r) && !errors.As(err, &d) {
			t.Errorf("epoch %d refusal is not E711/E684: %v", ep, err)
		}
	}
	// the attempt ends (lost): a late write from the old process is refused even at its old epoch.
	e2 := newCacheEnv(t)
	e2.start("lostone", factsMain)
	if err := e2.es.RecordPlan(e2.ctx, "lostone", 2, factsMain, []string{k}); err != nil {
		t.Fatal(err)
	}
	if err := e2.st.Transition(e2.ctx, "lostone", "running", "lost", store.TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	if err := e2.es.RecordResult(e2.ctx, "lostone", 3, true); err == nil {
		t.Error("a result was recorded for a lost attempt")
	}
	if err := e2.es.Commit(e2.ctx, "lostone", 3, e2.ns(factsMain), e2.manifest(factsMain, factsMain, k, "zombie")); err == nil {
		t.Error("a lost attempt committed")
	}
	if e2.lookup(factsMain, k, authz.ModeRWOwn) {
		t.Error("zombie write became visible")
	}
}

func TestB_Cache_FailedOrFlippedResultBlocksWrite(t *testing.T) {
	e := newCacheEnv(t)
	k := e.key(factsMain, "f")
	e.start("flip", factsMain)
	e.finish("flip", factsMain, []string{k}, false)
	if err := e.es.RecordResult(e.ctx, "flip", 3, true); err == nil {
		t.Error("a recorded failure was flipped to a pass at the same epoch")
	}
	if err := e.es.Commit(e.ctx, "flip", 3, e.ns(factsMain), e.manifest(factsMain, factsMain, k, "x")); err == nil {
		t.Error("a failed attempt committed")
	}
	// unplanned key
	e.start("plan", factsMain)
	e.finish("plan", factsMain, []string{k}, true)
	other := e.key(factsMain, "other")
	if err := e.es.Commit(e.ctx, "plan", 3, e.ns(factsMain), e.manifest(factsMain, factsMain, other, "y")); err == nil {
		t.Error("an unplanned key was committed")
	}
	// plan facts swapped to another project / trust after the fact
	if err := e.es.RecordPlan(e.ctx, "plan", 2, factsPR1, []string{k}); err == nil {
		t.Error("plan facts were replaced with an untrusted PR's after recording")
	}
	// blobs from another realm (poisoned bytes referenced by digest only)
	e.start("realm", factsMain)
	rk := e.key(factsMain, "realm")
	e.finish("realm", factsMain, []string{rk}, true)
	foreign := e.manifest(factsMain, factsPR1, rk, "bytes-from-the-pr-realm")
	if err := e.es.Commit(e.ctx, "realm", 3, e.ns(factsMain), foreign); err == nil {
		t.Error("an entry whose blobs live only in another namespace's realm was committed")
	}
	_ = bytes.MinRead
}
