package internal

import (
	"testing"
)

// TestJoinStrings_Empty verifies that joining an empty slice returns an empty string.
func TestJoinStrings_Empty(t *testing.T) {
	got := joinStrings(nil, ",")
	if got != "" {
		t.Errorf("joinStrings(nil) = %q, want empty", got)
	}
}

// TestJoinStrings_Single verifies that a single-element slice returns the element unchanged.
func TestJoinStrings_Single(t *testing.T) {
	got := joinStrings([]string{"only"}, ",")
	if got != "only" {
		t.Errorf("joinStrings([only]) = %q, want %q", got, "only")
	}
}

// TestJoinStrings_Multiple verifies that multiple elements are joined by the separator.
func TestJoinStrings_Multiple(t *testing.T) {
	got := joinStrings([]string{"a", "b", "c"}, "-")
	if got != "a-b-c" {
		t.Errorf("joinStrings([a,b,c], -) = %q, want %q", got, "a-b-c")
	}
}

// TestJoinStrings_PipeSeparator verifies pipe-separated joining.
func TestJoinStrings_PipeSeparator(t *testing.T) {
	got := joinStrings([]string{"x", "y"}, "|")
	if got != "x|y" {
		t.Errorf("joinStrings([x,y], |) = %q, want %q", got, "x|y")
	}
}
