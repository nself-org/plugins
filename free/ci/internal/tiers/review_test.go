package tiers

// Purpose: the review fixes of P7-AUTO-05: the order key is operator scope only, a
// lower scope keeps the relative order, a self-reported or non-numeric rank never
// wins, and an order that names no tier is an error.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// TestOrderKeyIsOperatorScopeOnly: a revision, pipeline or job scope cannot set the
// order, even to a "stricter" list, so a head commit never moves paid VPS ahead.
func TestOrderKeyIsOperatorScopeOnly(t *testing.T) {
	spec, ok := policy.Default.Find(KeyOrder)
	if !ok || !spec.OperatorOnly || spec.Spend || spec.Kind != policy.TightenOrdered {
		t.Fatalf("%s must be an operator-only, ordered, non-Spend key: %+v", KeyOrder, spec)
	}
	reg := registry(t)
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourcePipeline, trust.SourceJob, trust.SourceProtectedBranch} {
		for _, raw := range []string{`["vps"]`, `["local","lan","vps","hosted"]`, `["hosted","vps","lan","local"]`} {
			eff, _, err := reg.Merge("", scope(trust.SourceUser, KeyOrder, `["lan","vps","local"]`), scope(kind, KeyOrder, raw))
			ign := eff.Ignored()
			if err != nil || len(ign) != 1 || ign[0].Key != KeyOrder || !strings.Contains(ign[0].Reason, "E671") {
				t.Fatalf("%s %s: want the key ignored with E671: %v %v", kind, raw, ign, err)
			}
			if got := Order(eff); strings.Join(got, ",") != "lan,vps,local" {
				t.Fatalf("%s %s moved the order: %v", kind, raw, got)
			}
		}
	}
}

// TestOrderNarrowingKeepsRelativeOrder: a lower operator scope may only drop entries;
// a permutation or an added tier is refused (E670), never "narrowing".
func TestOrderNarrowingKeepsRelativeOrder(t *testing.T) {
	reg := registry(t)
	user := scope(trust.SourceUser, KeyOrder, `["lan","vps","local"]`)
	for _, c := range []struct {
		raw string
		ok  bool
	}{
		{`["lan","vps","local"]`, true}, {`["lan","local"]`, true}, {`["vps"]`, true}, {`["lan","vps"]`, true},
		{`["vps","lan"]`, false}, {`["local","lan"]`, false}, {`["local","lan","vps"]`, false}, {`["lan","vps","local","hosted"]`, false},
	} {
		eff, _, err := reg.Merge("", user, scope(trust.SourceProject, KeyOrder, c.raw))
		switch {
		case c.ok && err != nil:
			t.Errorf("%s: %v", c.raw, err)
		case !c.ok && (err == nil || !strings.Contains(err.Error(), "E670")):
			t.Errorf("%s: want E670, got %v", c.raw, err)
		case c.ok:
			// the result is a subsequence of the operator order: nothing was reordered.
			full, got := []string{"lan", "vps", "local"}, Order(eff)
			i := 0
			for _, x := range full {
				if i < len(got) && got[i] == x {
					i++
				}
			}
			if i != len(got) {
				t.Errorf("%s: %v is not a subsequence of %v", c.raw, got, full)
			}
		}
	}
	// the first operator setting over the registered default may reorder it.
	eff, _, err := reg.Merge("", scope(trust.SourceUser, KeyOrder, `["local","hosted"]`))
	if err != nil || strings.Join(Order(eff), ",") != "local,hosted" {
		t.Fatalf("operator order: %v %v", Order(eff), err)
	}
}

// TestRankLowestNumberWins: rank=10 sorts before rank=2 as text; the number decides.
func TestRankLowestNumberWins(t *testing.T) {
	r := sched.Runner{ID: "a", Location: "lan", Class: "owned"}
	for _, labels := range [][]string{{"rank=10", "rank=2"}, {"rank=2", "rank=10"}, {"rank=x", "rank=10", "rank=-3", "rank=2"}} {
		r.Capability.Identity.Labels = labels
		if got := Rank(r, 5); got != 2 {
			t.Errorf("%v: rank %d want 2", labels, got)
		}
	}
	r.Capability.Identity.Labels = []string{"rank=x", "rank=-1"}
	if got := Rank(r, 5); got != 5 {
		t.Errorf("no valid rank label falls back to the position: %d", got)
	}
}

// TestEmptyOrderIsAnError: an order that names no known tier fails the world build
// instead of making every tier tier 0.
func TestEmptyOrderIsAnError(t *testing.T) {
	lax, err := trust.NewRegistry(trust.KeySpec{Key: KeyOrder, Owner: Owner, Kind: policy.TightenAny})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`[]`, `["cloud"]`, `["cloud","edge"]`} {
		eff, _, err := lax.Merge("", scope(trust.SourceProject, KeyOrder, raw))
		if err != nil {
			t.Fatal(err)
		}
		w := sched.World{Policy: sched.Policy{Preset: sched.AutoPreset, Trust: eff}, Runners: []sched.Runner{{ID: "a", Location: "lan", Class: "owned"}}}
		if err := contribute(context.Background(), &w); err == nil || !strings.Contains(err.Error(), KeyOrder) || w.Bias != nil {
			t.Errorf("%s: want an error naming the key and no bias, got %v %v", raw, err, w.Bias)
		}
	}
}
