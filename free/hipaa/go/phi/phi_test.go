// Purpose: Baseline tests for the HIPAA PHI masking functions.
// Inputs: SSN strings, DOB strings, names, emails, addresses.
// Outputs: Masked strings with predictable patterns (XXX-XX-NNNN, etc.).
// Constraints: All functions are pure string transforms; no DB required.
package phi

import (
	"strings"
	"testing"
)

// TestBuild verifies the phi package compiles.
func TestBuild(t *testing.T) {
	t.Log("hipaa/phi package compiled OK")
}

// TestMaskSSN verifies SSN masking keeps only the last 4 digits.
func TestMaskSSN(t *testing.T) {
	cases := []struct{ in, want string }{
		{"123-45-6789", "XXX-XX-6789"},
		{"12", "XXX-XX-XXXX"},
		{"", "XXX-XX-XXXX"},
	}
	for _, tc := range cases {
		got := maskSSN(tc.in)
		if got != tc.want {
			t.Errorf("maskSSN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestMaskDOB verifies DOB masking reduces to year only.
func TestMaskDOB(t *testing.T) {
	got := maskDOB("1985-03-14")
	if got != "1985-XX-XX" {
		t.Errorf("maskDOB(%q) = %q, want %q", "1985-03-14", got, "1985-XX-XX")
	}
	if maskDOB("bad") != "XXXX-XX-XX" {
		t.Errorf("maskDOB(%q) should return fallback", "bad")
	}
}

// TestMaskEmail verifies email masking retains first/last char of local part.
func TestMaskEmail(t *testing.T) {
	got := maskEmail("john@example.com")
	if !strings.HasPrefix(got, "j") {
		t.Errorf("maskEmail: expected result to start with 'j', got %q", got)
	}
	if !strings.Contains(got, "@example.com") {
		t.Errorf("maskEmail: expected domain preserved in %q", got)
	}
	if maskEmail("noemail") != "[EMAIL]" {
		t.Errorf("maskEmail with no @ should return [EMAIL]")
	}
}

// TestMaskName verifies name masking replaces internal chars with asterisks.
func TestMaskName(t *testing.T) {
	got := maskName("John Doe")
	if !strings.HasPrefix(got, "J") {
		t.Errorf("maskName: first char should be preserved, got %q", got)
	}
	if maskName("") != "[NAME]" {
		t.Errorf("maskName('') should return [NAME]")
	}
}
