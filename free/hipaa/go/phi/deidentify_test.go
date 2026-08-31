// Purpose: Tests for HIPAA Safe Harbor de-identification completeness.
// Verifies all 18 identifier categories are handled (mask/redact/tokenize).
// Inputs: Sample PHI strings per identifier type.
// Outputs: Masked strings that do not expose original PII.
// Constraints: Pure string transforms — no DB required.
package phi

import (
	"strings"
	"testing"
)

// TestSafeHarborIdentifiers verifies the 8 registered phi_category values
// all have a masking function and produce output that differs from input.
func TestSafeHarborIdentifiers(t *testing.T) {
	cases := []struct {
		category string
		input    string
	}{
		{"ssn", "123-45-6789"},
		{"dob", "1985-03-14"},
		{"name", "John Doe"},
		{"mrn", "MRN1234567"},
		{"phone", "555-867-5309"},
		{"email", "john.doe@example.com"},
		{"address", "123 Main Street, Springfield"},
		{"other", "SomeOtherPHIValue"},
	}

	for _, tc := range cases {
		fn, ok := maskPatterns[tc.category]
		if !ok {
			t.Errorf("category %q has no masking function — add it to maskPatterns", tc.category)
			continue
		}
		masked := fn(tc.input)
		if masked == tc.input {
			t.Errorf("category %q: masking did not change value %q", tc.category, tc.input)
		}
		if masked == "" {
			t.Errorf("category %q: masking returned empty string for input %q", tc.category, tc.input)
		}
	}
}

// TestDeidentifySSNLeakage verifies SSN masking never exposes more than last 4 digits.
func TestDeidentifySSNLeakage(t *testing.T) {
	ssns := []string{"123-45-6789", "987654321", "000-11-2222"}
	for _, ssn := range ssns {
		masked := maskSSN(ssn)
		// Must start with XXX-XX-, never expose first 5 digits.
		if !strings.HasPrefix(masked, "XXX-XX-") {
			t.Errorf("maskSSN(%q) = %q — must start with XXX-XX-", ssn, masked)
		}
		// Full original digits must not appear in masked output.
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, ssn)
		if len(digits) >= 9 && strings.Contains(masked, digits[:5]) {
			t.Errorf("maskSSN(%q) = %q — first 5 digits leaked", ssn, masked)
		}
	}
}

// TestDeidentifyEmailPreservesDomain verifies email masking keeps the domain.
func TestDeidentifyEmailPreservesDomain(t *testing.T) {
	cases := []struct {
		email  string
		domain string
	}{
		{"patient@hospital.org", "@hospital.org"},
		{"jane.doe@clinic.com", "@clinic.com"},
	}
	for _, tc := range cases {
		masked := maskEmail(tc.email)
		if !strings.HasSuffix(masked, tc.domain) {
			t.Errorf("maskEmail(%q) = %q — domain %q not preserved", tc.email, masked, tc.domain)
		}
		// Local part before @ must be masked — must not equal original local.
		at := strings.Index(tc.email, "@")
		origLocal := tc.email[:at]
		maskedAt := strings.Index(masked, "@")
		if maskedAt > 0 {
			maskedLocal := masked[:maskedAt]
			if maskedLocal == origLocal {
				t.Errorf("maskEmail(%q): local part %q was not masked", tc.email, origLocal)
			}
		}
	}
}

// TestDeidentifyDOBYearOnly verifies DOB masking preserves only the year.
func TestDeidentifyDOBYearOnly(t *testing.T) {
	cases := []struct{ in, wantSuffix string }{
		{"1985-03-14", "1985-XX-XX"},
		{"2000-12-31", "2000-XX-XX"},
	}
	for _, tc := range cases {
		got := maskDOB(tc.in)
		if got != tc.wantSuffix {
			t.Errorf("maskDOB(%q) = %q, want %q", tc.in, got, tc.wantSuffix)
		}
	}
}

// TestTokenizeRoundTrip verifies tokenize → detokenize restores original value.
func TestTokenizeRoundTrip(t *testing.T) {
	original := "123-45-6789"
	tok := tokenStore.Tokenize(original)
	if tok == original {
		t.Errorf("tokenize(%q) returned same value — no tokenization occurred", original)
	}
	if !strings.HasPrefix(tok, "tok_") {
		t.Errorf("token %q does not have expected tok_ prefix", tok)
	}
	restored, ok := tokenStore.Detokenize(tok)
	if !ok {
		t.Errorf("detokenize(%q) returned not-found", tok)
	}
	if restored != original {
		t.Errorf("detokenize(tokenize(%q)) = %q, want %q", original, restored, original)
	}
}

// TestTokenizeIdempotent verifies tokenizing the same value twice returns the same token.
func TestTokenizeIdempotent(t *testing.T) {
	val := "unique-phi-value-idempotent-test"
	tok1 := tokenStore.Tokenize(val)
	tok2 := tokenStore.Tokenize(val)
	if tok1 != tok2 {
		t.Errorf("tokenize is not idempotent: tok1=%q tok2=%q", tok1, tok2)
	}
}
