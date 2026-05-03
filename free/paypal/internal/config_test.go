package internal

import (
	"testing"
)

// TestBaseURL_Sandbox verifies that the sandbox environment returns the
// sandbox API base URL.
func TestBaseURL_Sandbox(t *testing.T) {
	cfg := &Config{Environment: "sandbox"}
	got := cfg.BaseURL()
	want := "https://api-m.sandbox.paypal.com"
	if got != want {
		t.Errorf("BaseURL(sandbox) = %q, want %q", got, want)
	}
}

// TestBaseURL_Live verifies that the live environment returns the live API
// base URL.
func TestBaseURL_Live(t *testing.T) {
	cfg := &Config{Environment: "live"}
	got := cfg.BaseURL()
	want := "https://api-m.paypal.com"
	if got != want {
		t.Errorf("BaseURL(live) = %q, want %q", got, want)
	}
}

// TestBaseURL_Default verifies that any non-"live" environment returns the
// sandbox URL (safe default).
func TestBaseURL_Default(t *testing.T) {
	cfg := &Config{Environment: ""}
	got := cfg.BaseURL()
	want := "https://api-m.sandbox.paypal.com"
	if got != want {
		t.Errorf("BaseURL(empty) = %q, want %q", got, want)
	}
}

// TestSplitCSV verifies that splitCSV correctly tokenises comma-separated
// strings and handles edge cases.
func TestSplitCSV(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"  a , b , c  ", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"a,,b", []string{"a", "b"}}, // empty segment skipped
		{"", nil},
		{"  ,  ,  ", nil}, // all whitespace-only segments skipped
	}
	for _, tc := range cases {
		got := splitCSV(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitCSV(%q) = %v (len %d), want %v (len %d)",
				tc.input, got, len(got), tc.want, len(tc.want))
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}
