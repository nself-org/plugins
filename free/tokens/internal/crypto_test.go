package internal

import (
	"encoding/hex"
	"strings"
	"testing"
)

// TestEncryptDecryptRoundTrip verifies that encrypting then decrypting returns
// the original plaintext for various string inputs.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		plaintext string
		masterKey string
	}{
		{"ascii", "hello world", "master-key-32-bytes-padding-here"},
		{"unicode", "こんにちは", "unicode-master-key"},
		{"empty", "", "some-key"},
		{"long", strings.Repeat("x", 1024), "long-key"},
		{"special chars", "!@#$%^&*()_+-=[]{}|;':\",./<>?", "special"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encrypted, err := EncryptKeyMaterial(tc.plaintext, tc.masterKey)
			if err != nil {
				t.Fatalf("EncryptKeyMaterial error: %v", err)
			}
			if encrypted == "" {
				t.Fatal("EncryptKeyMaterial returned empty string")
			}
			// Encrypted output should contain the iv:ciphertext separator.
			if !strings.Contains(encrypted, ":") {
				t.Errorf("expected iv:ciphertext format, got %q", encrypted)
			}

			decrypted, err := DecryptKeyMaterial(encrypted, tc.masterKey)
			if err != nil {
				t.Fatalf("DecryptKeyMaterial error: %v", err)
			}
			if decrypted != tc.plaintext {
				t.Errorf("got %q, want %q", decrypted, tc.plaintext)
			}
		})
	}
}

// TestEncryptNonDeterministic verifies that two calls with the same input
// produce different ciphertexts (due to random IV).
func TestEncryptNonDeterministic(t *testing.T) {
	a, err := EncryptKeyMaterial("secret", "key")
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncryptKeyMaterial("secret", "key")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("expected different ciphertexts for same input (IV should be random)")
	}
}

// TestDecryptInvalidFormat verifies that malformed encrypted strings return an error.
func TestDecryptInvalidFormat(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"no colon", "justahexstring"},
		{"bad IV hex", "ZZZZ:aabb"},
		{"bad ciphertext hex", "aabb:ZZZZ"},
		{"not block aligned", "aabb:" + hex.EncodeToString([]byte("not-aligned"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecryptKeyMaterial(tc.input, "key")
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}
		})
	}
}

// TestDecryptWrongKey verifies that decrypting with a different key returns
// an error (invalid padding after wrong key decryption).
func TestDecryptWrongKey(t *testing.T) {
	encrypted, err := EncryptKeyMaterial("secret text", "correct-key")
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecryptKeyMaterial(encrypted, "wrong-key")
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

// TestGenerateToken verifies the JWT-like token format and that the same
// payload + key always produces a consistent structure.
func TestGenerateToken(t *testing.T) {
	payload := map[string]interface{}{
		"sub": "user-123",
		"exp": 1700000000,
	}
	token, err := GenerateToken(payload, "signing-key")
	if err != nil {
		t.Fatalf("GenerateToken error: %v", err)
	}

	// Token must be three base64url segments.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 segments, got %d: %q", len(parts), token)
	}
	for i, part := range parts {
		if part == "" {
			t.Errorf("segment %d is empty", i)
		}
	}
}

// TestGenerateTokenDifferentKeys verifies that different keys produce different tokens.
func TestGenerateTokenDifferentKeys(t *testing.T) {
	payload := map[string]interface{}{"x": 1}
	a, err := GenerateToken(payload, "key-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateToken(payload, "key-b")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("expected different tokens for different signing keys")
	}
}

// TestHashToken verifies deterministic hashing — same input always produces the same hash.
func TestHashToken(t *testing.T) {
	h1 := HashToken("my-token-value")
	h2 := HashToken("my-token-value")
	if h1 != h2 {
		t.Errorf("HashToken is not deterministic: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("HashToken returned empty string")
	}

	// Different tokens must produce different hashes.
	h3 := HashToken("other-token")
	if h1 == h3 {
		t.Error("HashToken collision: different inputs produced same hash")
	}
}

// TestGenerateRandomHex verifies length, hex encoding, and uniqueness.
func TestGenerateRandomHex(t *testing.T) {
	cases := []int{8, 16, 32}
	for _, n := range cases {
		h, err := GenerateRandomHex(n)
		if err != nil {
			t.Fatalf("GenerateRandomHex(%d) error: %v", n, err)
		}
		want := n * 2 // hex encodes 1 byte to 2 chars
		if len(h) != want {
			t.Errorf("GenerateRandomHex(%d): got len %d, want %d", n, len(h), want)
		}
		if _, err := hex.DecodeString(h); err != nil {
			t.Errorf("GenerateRandomHex(%d) output is not valid hex: %v", n, err)
		}
	}

	// Two calls should not be equal.
	a, _ := GenerateRandomHex(16)
	b, _ := GenerateRandomHex(16)
	if a == b {
		t.Error("expected distinct random hex values")
	}
}

// TestGenerateRandomBytes verifies length and uniqueness.
func TestGenerateRandomBytes(t *testing.T) {
	b, err := GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes error: %v", err)
	}
	if len(b) != 32 {
		t.Errorf("got len %d, want 32", len(b))
	}

	c, _ := GenerateRandomBytes(32)
	equal := true
	for i := range b {
		if b[i] != c[i] {
			equal = false
			break
		}
	}
	if equal {
		t.Error("expected distinct random byte slices")
	}
}

// TestConstantTimeEqual verifies timing-safe comparison semantics.
func TestConstantTimeEqual(t *testing.T) {
	cases := []struct {
		a, b []byte
		want bool
	}{
		{[]byte("abc"), []byte("abc"), true},
		{[]byte("abc"), []byte("abd"), false},
		{[]byte(""), []byte(""), true},
		{[]byte("a"), []byte("b"), false},
		{[]byte("longer"), []byte("short"), false},
	}

	for _, tc := range cases {
		got := ConstantTimeEqual(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("ConstantTimeEqual(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
