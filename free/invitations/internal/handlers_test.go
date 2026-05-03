package internal

import (
	"encoding/hex"
	"testing"
)

// TestGenerateToken_Format verifies that generateToken returns a 64-character
// lowercase hex string (32 random bytes encoded).
func TestGenerateToken_Format(t *testing.T) {
	tok, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error: %v", err)
	}
	if len(tok) != 64 {
		t.Errorf("token length = %d, want 64", len(tok))
	}
	// Must be valid hex.
	if _, err := hex.DecodeString(tok); err != nil {
		t.Errorf("token %q is not valid hex: %v", tok, err)
	}
}

// TestGenerateToken_Uniqueness verifies that two successive calls produce
// different tokens (collision probability is negligible with 32 random bytes).
func TestGenerateToken_Uniqueness(t *testing.T) {
	tok1, err1 := generateToken()
	tok2, err2 := generateToken()
	if err1 != nil || err2 != nil {
		t.Fatalf("generateToken errors: %v, %v", err1, err2)
	}
	if tok1 == tok2 {
		t.Error("expected two generateToken calls to return different values")
	}
}
