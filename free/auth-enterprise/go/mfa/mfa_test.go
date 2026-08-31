// Purpose: Baseline tests for the mfa package (TOTP enrollment and backup codes).
// Inputs: internal generateSecret, generateBackupCodes, DefaultDigits, DefaultPeriod constants.
// Outputs: base32-encoded secret string, slice of backup code strings.
// Constraints: No DB required; only crypto/rand functions tested here.
package mfa

import (
	"testing"
	"unicode"
)

// TestBuild verifies the mfa package compiles and exports expected constants.
func TestBuild(t *testing.T) {
	t.Log("mfa package compiled OK")
	if DefaultDigits != 6 {
		t.Errorf("DefaultDigits = %d, want 6", DefaultDigits)
	}
	if DefaultPeriod != 30 {
		t.Errorf("DefaultPeriod = %d, want 30", DefaultPeriod)
	}
	if DefaultBackupCodeCount != 8 {
		t.Errorf("DefaultBackupCodeCount = %d, want 8", DefaultBackupCodeCount)
	}
}

// TestGenerateSecret verifies that generateSecret returns a non-empty base32 string.
func TestGenerateSecret(t *testing.T) {
	secret, err := generateSecret()
	if err != nil {
		t.Fatalf("generateSecret() returned error: %v", err)
	}
	if len(secret) == 0 {
		t.Fatal("generateSecret() returned empty string")
	}
	// Base32 characters are uppercase letters and digits 2-7, no padding here.
	for _, ch := range secret {
		if !unicode.IsUpper(ch) && (ch < '2' || ch > '7') {
			t.Errorf("generateSecret() returned non-base32 character %q", ch)
		}
	}
}

// TestGenerateBackupCodes verifies that generateBackupCodes returns the correct count.
func TestGenerateBackupCodes(t *testing.T) {
	codes, err := generateBackupCodes(DefaultBackupCodeCount)
	if err != nil {
		t.Fatalf("generateBackupCodes(%d) returned error: %v", DefaultBackupCodeCount, err)
	}
	if len(codes) != DefaultBackupCodeCount {
		t.Errorf("generateBackupCodes(%d) returned %d codes, want %d", DefaultBackupCodeCount, len(codes), DefaultBackupCodeCount)
	}
	seen := make(map[string]bool)
	for _, code := range codes {
		if len(code) != 8 {
			t.Errorf("backup code %q has length %d, want 8", code, len(code))
		}
		if seen[code] {
			t.Errorf("duplicate backup code %q", code)
		}
		seen[code] = true
	}
}
