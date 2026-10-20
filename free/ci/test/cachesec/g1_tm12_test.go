//go:build cachesec

package cachesec

// Purpose: TM-12 cases (cache poisoning across trust, PR or project, key collision included) on the
// local carrier: a commit into a namespace the attempt does not own, and key collisions through
// field boundaries. The read-side halves (secret-bearing snapshot, trusted job reading an
// untrusted PR's entry) are the miss cases C12-CAC-06 and C12-CAC-07 in g1_miss_test.go.
// Inputs: a real store and cas (newWorld). Outputs: registered cases C12-CAC-01..05.
// Constraints: production packages only. Each case commits an honest entry of the same attempt
// first (positive control); Landed is the entry bytes added by the attack, so the control does
// not count. A blob Put into the attacked realm before a refused commit is unreferenced (swept by
// cas.Sweep) and is not reachable through any entry.

import (
	"bytes"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// g1Poison runs the common TM-12 shape: attempt of f plans its own key and the target key, commits
// its own (control), then tries to commit content into the namespace of target under targetKey.
func g1Poison(t *testing.T, f, target entries.Facts) Observed {
	w := newWorld(t)
	own, tkey := w.Key(f, "own"), w.Key(target, "own")
	if own == tkey {
		t.Fatal("fixture: the two namespaces produced one key")
	}
	fin := w.Attempt("a1", f, []string{own, tkey}, true)
	if err := w.g1Commit("a1", fin, f, own, "honest bytes"); err != nil { // positive control
		t.Fatalf("control: the honest commit into the own namespace was refused: %v", err)
	}
	base := w.EntryBytes()
	err := w.Es.Commit(w.ctx, "a1", fin, w.NS(target), w.Manifest(target, tkey, "poisoned bytes"))
	got := g1Denied(t, err)
	if w.g1Visible(tkey) != 0 {
		t.Fatal("a denied commit is visible under the target key")
	}
	got.Audit = w.AuditOf("cache.commit.denied")
	got.Landed = w.EntryBytes() - base
	return got
}

func init() {
	register("C12-CAC-01", func(t *testing.T) Observed { return g1Poison(t, g1PR1, g1Main) })
	register("C12-CAC-02", func(t *testing.T) Observed { return g1Poison(t, g1PR1, g1PR2) })
	register("C12-CAC-03", func(t *testing.T) Observed {
		other := g1PR1
		other.HeadRepo = "other/fork" // same PR number, different head repo
		return g1Poison(t, g1PR1, other)
	})

	// Key collision through field boundaries: a byte moved across a boundary changes the key, and an
	// attempt that planned key K' cannot commit key K.
	register("C12-CAC-04", func(t *testing.T) Observed {
		w := newWorld(t)
		ns := w.NS(g1PR1)
		in := func(spec, ver string, extras ...string) string {
			k, err := entries.Key(entries.KeyInputs{Namespace: ns, SpecID: spec, SpecVersion: ver, Platform: "linux/amd64", Isolation: "container", Extras: extras})
			if err != nil {
				t.Fatal(err)
			}
			return k
		}
		pairs := [][2]string{
			{in("ab", "c"), in("a", "bc")},                         // scalar boundary
			{in("s", "1", "ab", "c"), in("s", "1", "a", "bc")},     // list item boundary
			{in("s", "1", "ab"), in("s", "1", "a", "b")},           // list count
			{in("s", "1", "a", "b", "c"), in("s", "1", "ab", "c")}, // item merged
		}
		for i, p := range pairs {
			if p[0] == p[1] {
				t.Fatalf("pair %d: a byte shifted across a field boundary gave the same key", i)
			}
		}
		if in("ab", "c") != in("ab", "c") {
			t.Fatal("control: the key is not deterministic")
		}
		a := authz.Namespace{Project: "proj", Trust: "untrusted", Protection: authz.Unprotected, HeadRepo: "ab", PR: "c"}
		b := a
		b.HeadRepo, b.PR = "a", "bc"
		if !a.Valid() || !b.Valid() || bytes.Equal(a.Canonical(), b.Canonical()) {
			t.Fatal("namespaces shifting a byte between head repo and PR share an encoding")
		}
		keyOf := func(n authz.Namespace) string {
			k, err := entries.Key(entries.KeyInputs{Namespace: n, SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "container"})
			if err != nil {
				t.Fatal(err)
			}
			return k
		}
		if keyOf(a) == keyOf(b) {
			t.Fatal("namespaces shifting a byte between head repo and PR share a key")
		}
		kPlanned, kOther := pairs[0][1], pairs[0][0] // K' is planned, K is not
		fin := w.Attempt("k1", g1PR1, []string{kPlanned}, true)
		if err := w.g1Commit("k1", fin, g1PR1, kPlanned, "honest bytes"); err != nil { // positive control
			t.Fatalf("control: the planned key was refused: %v", err)
		}
		base := w.EntryBytes()
		got := refusal(t, w.g1Commit("k1", fin, g1PR1, kOther, "collision bytes"), g1ReasonE711(t))
		got.Audit = w.AuditOf("cache.commit.refused")
		if w.g1Visible(kOther) != 0 {
			t.Fatal("the unplanned key is visible")
		}
		got.Landed = w.EntryBytes() - base
		return got
	})

	// A manifest that names the trusted namespace while the commit claims the attempt's own.
	register("C12-CAC-05", func(t *testing.T) Observed {
		w := newWorld(t)
		own, ctl := w.Key(g1PR1, "own"), w.Key(g1PR1, "ctl")
		fin := w.Attempt("m1", g1PR1, []string{ctl, own}, true)
		if err := w.g1Commit("m1", fin, g1PR1, ctl, "honest bytes"); err != nil { // positive control
			t.Fatalf("control: %v", err)
		}
		base := w.EntryBytes()
		m := w.Manifest(g1PR1, own, "smuggled bytes")
		m.Namespace = w.NS(g1Main).Canonical()
		got := refusal(t, w.Es.Commit(w.ctx, "m1", fin, w.NS(g1PR1), m), g1ReasonE711(t))
		got.Audit = w.AuditOf("cache.commit.refused")
		got.Landed = w.EntryBytes() - base
		if w.g1Visible(own) != 0 {
			t.Fatal("the smuggled manifest is visible")
		}
		return got
	})
}
