package bench

import "testing"

func TestWorstOf(t *testing.T) {
	a := NewResult("", 20, []Metric{{Name: "x", Unit: "ms", P50: 1, P95: 5, Max: 6, N: 20}, {Name: "y", Unit: "ms", P95: 2, N: 20}})
	b := NewResult("", 20, []Metric{{Name: "x", Unit: "ms", P50: 2, P95: 4, Max: 9, N: 20}, {Name: "y", Unit: "ms", P95: 3, N: 10}})
	got, err := WorstOf([]Result{a, b})
	if err != nil {
		t.Fatal(err)
	}
	x, _ := got.Find("x")
	y, _ := got.Find("y")
	if x.P50 != 2 || x.P95 != 5 || x.Max != 9 || y.P95 != 3 || y.N != 10 {
		t.Fatalf("%+v %+v", x, y)
	}
	if a.Metrics[0].P50 != 1 {
		t.Fatal("WorstOf changed its input")
	}
	other := b
	other.GOARCH = "riscv64"
	if _, err := WorstOf([]Result{a, other}); err == nil {
		t.Fatal("runs from two platforms were folded")
	}
	if _, err := WorstOf(nil); err == nil {
		t.Fatal("no runs accepted")
	}
}
