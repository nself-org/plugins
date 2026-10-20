package gamma

import "testing"

func TestName(t *testing.T) {
	if Name() == "" {
		t.Fatal("empty name")
	}
}

func TestSum(t *testing.T) {
	if got := Sum(1, 2, 3); got != 6 {
		t.Fatalf("Sum = %d", got)
	}
}

func TestRepeat(t *testing.T) {
	if got := Repeat("a", 3); got != "a,a,a" {
		t.Fatalf("Repeat = %q", got)
	}
}
