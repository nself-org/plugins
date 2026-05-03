package internal

import (
	"testing"
)

// TestStrPtr_Empty verifies that strPtr returns nil for an empty string.
func TestStrPtr_Empty(t *testing.T) {
	got := strPtr("")
	if got != nil {
		t.Errorf("strPtr(%q) = %v, want nil", "", got)
	}
}

// TestStrPtr_NonEmpty verifies that strPtr returns a non-nil pointer to the value.
func TestStrPtr_NonEmpty(t *testing.T) {
	got := strPtr("hello")
	if got == nil {
		t.Fatal("strPtr(non-empty) = nil, want non-nil pointer")
	}
	if *got != "hello" {
		t.Errorf("*strPtr = %q, want %q", *got, "hello")
	}
}
