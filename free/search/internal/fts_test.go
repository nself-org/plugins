package internal

import (
	"context"
	"testing"
)

// TestParseIntDefault verifies that parseIntDefault handles valid, invalid,
// and empty strings correctly.
func TestParseIntDefault(t *testing.T) {
	cases := []struct {
		input    string
		defVal   int
		expected int
	}{
		{"10", 5, 10},
		{"0", 5, 0},
		{"", 5, 5},
		{"abc", 5, 5},
		{"-3", 5, -3},
	}
	for _, tc := range cases {
		got := parseIntDefault(tc.input, tc.defVal)
		if got != tc.expected {
			t.Errorf("parseIntDefault(%q, %d) = %d, want %d", tc.input, tc.defVal, got, tc.expected)
		}
	}
}

// TestBuildSearchText verifies that buildSearchText joins values of the
// specified searchable fields.
func TestBuildSearchText(t *testing.T) {
	fields := map[string]interface{}{
		"title":   "Hello World",
		"body":    "Some content",
		"ignored": "not-included",
	}

	got := buildSearchText([]string{"title", "body"}, fields)
	if got != "Hello World Some content" {
		t.Errorf("buildSearchText = %q, want %q", got, "Hello World Some content")
	}

	// Missing key is skipped.
	got2 := buildSearchText([]string{"title", "missing"}, fields)
	if got2 != "Hello World" {
		t.Errorf("buildSearchText (missing key) = %q, want %q", got2, "Hello World")
	}

	// Nil value is skipped.
	fieldsWithNil := map[string]interface{}{"x": nil, "y": "present"}
	got3 := buildSearchText([]string{"x", "y"}, fieldsWithNil)
	if got3 != "present" {
		t.Errorf("buildSearchText (nil val) = %q, want %q", got3, "present")
	}

	// Empty fields.
	got4 := buildSearchText([]string{}, fields)
	if got4 != "" {
		t.Errorf("buildSearchText (no fields) = %q, want %q", got4, "")
	}
}

// TestWithSourceAccount verifies that WithSourceAccount stores a value that
// resolveSourceAccountID can retrieve.
func TestWithSourceAccount(t *testing.T) {
	ctx := WithSourceAccount(context.Background(), "account-xyz")
	got := resolveSourceAccountID(ctx)
	if got != "account-xyz" {
		t.Errorf("resolveSourceAccountID = %q, want %q", got, "account-xyz")
	}
}

// TestResolveSourceAccountID_Default verifies the default value when no
// source account is in context.
func TestResolveSourceAccountID_Default(t *testing.T) {
	got := resolveSourceAccountID(context.Background())
	if got == "" {
		t.Error("expected non-empty default source account ID")
	}
}
