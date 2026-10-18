package crypto

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/nself-byok/kms"
)

// mockKMS is a local in-memory KMS that XORs the DEK with a fixed key byte.
type mockKMS struct {
	maskByte byte
	revoked  bool
}

func (m *mockKMS) WrapKey(_ context.Context, dek []byte, _ string) ([]byte, error) {
	if m.revoked {
		return nil, errors.New("mock kms: key revoked")
	}
	out := make([]byte, len(dek))
	for i, b := range dek {
		out[i] = b ^ m.maskByte
	}
	return out, nil
}

func (m *mockKMS) UnwrapKey(_ context.Context, wrapped []byte, _ string) ([]byte, error) {
	if m.revoked {
		return nil, errors.New("mock kms: key revoked — permission denied")
	}
	out := make([]byte, len(wrapped))
	for i, b := range wrapped {
		out[i] = b ^ m.maskByte
	}
	return out, nil
}

func (m *mockKMS) DescribeKey(_ context.Context, keyRef string) (kms.KeyMetadata, error) {
	return kms.KeyMetadata{KeyRef: keyRef, Provider: "mock", State: "ENABLED"}, nil
}

// TestEnvelopeRoundTrip verifies that Encrypt → Decrypt recovers the original plaintext.
func TestEnvelopeRoundTrip(t *testing.T) {
	env := NewEnvelope(&mockKMS{maskByte: 0xAB})
	ctx := context.Background()
	plaintext := []byte("hello, nSelf BYOK")

	bundle, err := env.Encrypt(ctx, plaintext, "test-key-ref")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(bundle.Ciphertext) == 0 {
		t.Fatal("Encrypt: ciphertext is empty")
	}
	if len(bundle.IV) != NonceSize {
		t.Fatalf("Encrypt: IV length = %d, want %d", len(bundle.IV), NonceSize)
	}
	if bundle.Algorithm != "AES-256-GCM" {
		t.Fatalf("Encrypt: algorithm = %q, want AES-256-GCM", bundle.Algorithm)
	}

	recovered, err := env.Decrypt(ctx, bundle)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(recovered) != string(plaintext) {
		t.Fatalf("Decrypt: got %q, want %q", recovered, plaintext)
	}
}

// TestEnvelopeRevokedKey verifies that a revoked CMK causes Decrypt to return an error,
// not stale plaintext (satisfies the BYOK spec: "revoked key → error, not plaintext").
func TestEnvelopeRevokedKey(t *testing.T) {
	mock := &mockKMS{maskByte: 0x55}
	env := NewEnvelope(mock)
	ctx := context.Background()
	plaintext := []byte("secret data")

	bundle, err := env.Encrypt(ctx, plaintext, "my-key")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Revoke the key.
	mock.revoked = true

	_, err = env.Decrypt(ctx, bundle)
	if err == nil {
		t.Fatal("Decrypt with revoked key should return an error, got nil")
	}
}

// TestEnvelopeDifferentIVEachCall verifies that two encryptions of the same
// plaintext produce different nonces (no IV reuse).
func TestEnvelopeDifferentIVEachCall(t *testing.T) {
	env := NewEnvelope(&mockKMS{maskByte: 0x01})
	ctx := context.Background()
	plaintext := []byte("same plaintext")

	b1, err := env.Encrypt(ctx, plaintext, "k")
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}
	b2, err := env.Encrypt(ctx, plaintext, "k")
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}

	if string(b1.IV) == string(b2.IV) {
		t.Fatal("Two encryptions produced identical IVs — IV reuse detected")
	}
	if string(b1.Ciphertext) == string(b2.Ciphertext) {
		t.Fatal("Two encryptions produced identical ciphertexts — expected probabilistic encryption")
	}
}

// TestEnvelopeEmptyPlaintext verifies that empty plaintext is handled without panic.
func TestEnvelopeEmptyPlaintext(t *testing.T) {
	env := NewEnvelope(&mockKMS{maskByte: 0xFF})
	ctx := context.Background()

	bundle, err := env.Encrypt(ctx, []byte{}, "k")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	recovered, err := env.Decrypt(ctx, bundle)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("expected empty plaintext, got %q", recovered)
	}
}
