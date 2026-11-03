package economics

// Tests of the reference-counted process binding (D-1280): overlapping runs share one scope, the
// first release leaves the second bound, the last release unbinds, and a run with another store,
// coordinator or spend policy is refused. Fake store and policies only.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

func TestBindOverlappingRunsShareOneScope(t *testing.T) {
	clk := &clock{t: at("2026-11-03T10:00:00Z")}
	st := openStore(t, clk)
	p := spendPolicy(t, paidKV(nil))
	a, b := envFor(st, p, clk, "coord-1"), envFor(st, p, clk, "coord-1") // two runs: equal, separately built
	relA, err := Bind(a)
	if err != nil {
		t.Fatal(err)
	}
	relB, err := Bind(b)
	if err != nil {
		t.Fatalf("the same store, spend policy and coordinator share the scope: %v", err)
	}
	relA() // run A finishes while run B is still dispatching
	relA() // a repeated release must not drop B's hold
	if _, ok := envOf(context.Background()); !ok {
		t.Fatal("run A's release unbound economics under run B")
	}
	relB()
	if _, ok := envOf(context.Background()); ok {
		t.Fatal("the last release must unbind")
	}
	relC, err := Bind(a) // after a full release a new scope may bind anything
	if err != nil {
		t.Fatal(err)
	}
	relC()
}

func TestBindRefusesAnotherEnvAndHoldsNothing(t *testing.T) {
	clk := &clock{t: at("2026-11-03T10:00:00Z")}
	st, other := openStore(t, clk), openStore(t, clk)
	p := spendPolicy(t, paidKV(nil))
	rel, err := Bind(envFor(st, p, clk, "coord-1"))
	if err != nil {
		t.Fatal(err)
	}
	for name, e := range map[string]Env{
		"store":       envFor(other, p, clk, "coord-1"),
		"coordinator": envFor(st, p, clk, "coord-2"),
		"policy":      envFor(st, spendPolicy(t, paidKV(map[string]string{schedpolicy.KeyBudgetDay: `2000000`})), clk, "coord-1"),
	} {
		r, berr := Bind(e)
		if berr == nil {
			t.Fatalf("%s: a different env must be refused", name)
		}
		r() // the refused hold releases nothing
	}
	rel()
	if _, ok := envOf(context.Background()); ok {
		t.Fatal("refused holds must not keep the scope bound")
	}
}
