package internal

import (
	"testing"
)

// TestNilIfEmpty_Empty verifies that an empty string returns nil.
func TestNilIfEmpty_Empty(t *testing.T) {
	if got := nilIfEmpty(""); got != nil {
		t.Errorf("nilIfEmpty(%q) = %v, want nil", "", got)
	}
}

// TestNilIfEmpty_NonEmpty verifies that a non-empty string returns a non-nil
// pointer to the string value.
func TestNilIfEmpty_NonEmpty(t *testing.T) {
	got := nilIfEmpty("hello")
	if got == nil {
		t.Fatal("nilIfEmpty(non-empty) = nil, want non-nil pointer")
	}
	if *got != "hello" {
		t.Errorf("*nilIfEmpty(%q) = %q, want %q", "hello", *got, "hello")
	}
}
