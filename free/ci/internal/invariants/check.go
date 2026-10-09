package invariants

import "testing"

type PlaceFn func(World) error
type DispatchFn func(World) error

func Check(t *testing.T, place PlaceFn) {
	t.Helper()
	id := t.Name()
	if len(id) > len("TestInvariant_") {
		id = id[len("TestInvariant_"):]
	}
	violations := 0
	first := ""
	visit := func(w World) {
		if err := place(w); err != nil {
			violations++
			if first == "" {
				first = err.Error()
			}
		}
	}
	n := Enumerate(visit)
	seed := Seed()
	Generate(seed, 10000, visit)
	t.Logf("INVARIANT id=%s enumerated=%d generated=10000 seed=%d violations=%d", id, n, seed, violations)
	if violations != 0 {
		t.Fatalf("%d violations: %s", violations, first)
	}
}
func CheckDispatcher(t *testing.T, dispatch DispatchFn) { Check(t, PlaceFn(dispatch)) }
