package internal

import (
	"testing"
)

// TestNilIfEmpty_NilPointer verifies that a nil pointer returns nil.
func TestNilIfEmpty_NilPointer(t *testing.T) {
	if got := nilIfEmpty(nil); got != nil {
		t.Errorf("nilIfEmpty(nil) = %v, want nil", got)
	}
}

// TestNilIfEmpty_EmptyString verifies that a pointer to an empty string
// returns nil.
func TestNilIfEmpty_EmptyString(t *testing.T) {
	s := ""
	if got := nilIfEmpty(&s); got != nil {
		t.Errorf("nilIfEmpty(&%q) = %v, want nil", s, got)
	}
}

// TestNilIfEmpty_NonEmptyString verifies that a pointer to a non-empty string
// returns the string value.
func TestNilIfEmpty_NonEmptyString(t *testing.T) {
	s := "hello"
	got := nilIfEmpty(&s)
	if got == nil {
		t.Fatal("nilIfEmpty(&non-empty) = nil, want non-nil")
	}
	if got != s {
		t.Errorf("nilIfEmpty = %v, want %q", got, s)
	}
}

// TestNilIfEmptyStr verifies the string (not pointer) variant.
func TestNilIfEmptyStr(t *testing.T) {
	if got := nilIfEmptyStr(""); got != nil {
		t.Errorf("nilIfEmptyStr(%q) = %v, want nil", "", got)
	}
	got := nilIfEmptyStr("value")
	if got == nil {
		t.Fatal("nilIfEmptyStr(non-empty) = nil, want non-nil")
	}
	if got != "value" {
		t.Errorf("nilIfEmptyStr = %v, want %q", got, "value")
	}
}

// TestNilIfZeroInt64 verifies the int64 zero-check helper.
func TestNilIfZeroInt64(t *testing.T) {
	if got := nilIfZeroInt64(0); got != nil {
		t.Errorf("nilIfZeroInt64(0) = %v, want nil", got)
	}
	got := nilIfZeroInt64(42)
	if got == nil {
		t.Fatal("nilIfZeroInt64(42) = nil, want non-nil")
	}
	if got != int64(42) {
		t.Errorf("nilIfZeroInt64 = %v, want 42", got)
	}
}

// TestNilIfZeroInt verifies the int zero-check helper.
func TestNilIfZeroInt(t *testing.T) {
	if got := nilIfZeroInt(0); got != nil {
		t.Errorf("nilIfZeroInt(0) = %v, want nil", got)
	}
	got := nilIfZeroInt(7)
	if got == nil {
		t.Fatal("nilIfZeroInt(7) = nil, want non-nil")
	}
	if got != 7 {
		t.Errorf("nilIfZeroInt = %v, want 7", got)
	}
}
