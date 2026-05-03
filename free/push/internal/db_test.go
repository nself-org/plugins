package internal

import (
	"encoding/json"
	"testing"
)

// TestDedupeHash_NonEmpty verifies that the hash is a 64-character hex string.
func TestDedupeHash_NonEmpty(t *testing.T) {
	payload := json.RawMessage(`{}`)
	h := DedupeHash("tok", "fcm", payload)
	if h == "" {
		t.Error("DedupeHash returned empty string")
	}
	// SHA-256 produces 32 bytes = 64 hex chars
	if len(h) != 64 {
		t.Errorf("hash length = %d, want 64", len(h))
	}
}
