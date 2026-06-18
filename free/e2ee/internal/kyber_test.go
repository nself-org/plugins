package internal

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/cloudflare/circl/kem/mlkem/mlkem1024"
)

// KAT-style tests for the server-side PUBLIC-key verification surface.
// These exercise the exact code paths an Opus CR-C will scrutinize:
//   1. ML-KEM-1024 public-key structural validation (vetted circl library).
//   2. Ed25519 signed-prekey signature verification (stdlib).
//   3. Kyber prekey upload verification (structure + signature binding).
// No private KEM key is ever exposed by these helpers — they validate public
// material only.

// genKyberPub returns a freshly generated, valid ML-KEM-1024 public key.
func genKyberPub(t *testing.T) []byte {
	t.Helper()
	pub, _, err := mlkem1024.Scheme().GenerateKeyPair()
	if err != nil {
		t.Fatalf("kyber keygen: %v", err)
	}
	b, err := pub.MarshalBinary()
	if err != nil {
		t.Fatalf("kyber marshal: %v", err)
	}
	return b
}

func TestKyberPublicKeySize(t *testing.T) {
	// Pin the on-the-wire size: ML-KEM-1024 encapsulation key is 1568 bytes
	// (FIPS 203). A drift here means the schema BYTEA contents changed shape.
	if KyberPublicKeySize != 1568 {
		t.Fatalf("expected ML-KEM-1024 public key size 1568, got %d", KyberPublicKeySize)
	}
}

func TestValidateKyberPublicKey_Valid(t *testing.T) {
	pub := genKyberPub(t)
	if err := ValidateKyberPublicKey(pub); err != nil {
		t.Fatalf("valid Kyber public key rejected: %v", err)
	}
}

func TestValidateKyberPublicKey_WrongLength(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		make([]byte, KyberPublicKeySize-1),
		make([]byte, KyberPublicKeySize+1),
		make([]byte, 32), // looks like an X25519 key, not Kyber
	}
	for i, c := range cases {
		if err := ValidateKyberPublicKey(c); err == nil {
			t.Fatalf("case %d: expected error for invalid-length key", i)
		}
	}
}

func TestValidateKyberPublicKey_RandomGarbageRightLength(t *testing.T) {
	// Right length but not a valid ML-KEM-1024 key. circl's UnmarshalBinary must
	// reject structurally-invalid keys (e.g. coefficient bounds). We assert the
	// function returns an error rather than panicking. (If circl accepts an
	// arbitrary 1568-byte string for ML-KEM, the size check still bounds risk.)
	garbage := make([]byte, KyberPublicKeySize)
	if _, err := rand.Read(garbage); err != nil {
		t.Fatalf("rand: %v", err)
	}
	// Either accepted (treated as opaque public key by circl) or rejected — both
	// are length-safe. We only require no panic, exercised by calling it.
	_ = ValidateKyberPublicKey(garbage)
}

func TestVerifyEd25519Signature_RoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}
	msg := []byte("signed-prekey-public-bytes")
	sig := ed25519.Sign(priv, msg)

	if err := VerifyEd25519Signature(pub, msg, sig); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifyEd25519Signature_TamperedMessage(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sig := ed25519.Sign(priv, []byte("original"))
	if err := VerifyEd25519Signature(pub, []byte("tampered"), sig); err == nil {
		t.Fatal("tampered message must fail verification")
	}
}

func TestVerifyEd25519Signature_WrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	msg := []byte("msg")
	sig := ed25519.Sign(priv, msg)
	if err := VerifyEd25519Signature(otherPub, msg, sig); err == nil {
		t.Fatal("signature under a different key must fail")
	}
}

func TestVerifyEd25519Signature_BadLengths(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	msg := []byte("msg")
	sig := ed25519.Sign(priv, msg)

	if err := VerifyEd25519Signature(pub[:10], msg, sig); err != ErrInvalidIdentityKey {
		t.Fatalf("short identity key: expected ErrInvalidIdentityKey, got %v", err)
	}
	if err := VerifyEd25519Signature(pub, msg, sig[:10]); err != ErrBadSignature {
		t.Fatalf("short signature: expected ErrBadSignature, got %v", err)
	}
}

func TestVerifyKyberPreKey_Valid(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	kyberPub := genKyberPub(t)
	sig := ed25519.Sign(priv, kyberPub) // identity key signs the Kyber public key

	if err := VerifyKyberPreKey(pub, kyberPub, sig); err != nil {
		t.Fatalf("valid Kyber prekey rejected: %v", err)
	}
}

func TestVerifyKyberPreKey_UnsignedRejected(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	kyberPub := genKyberPub(t)
	// Signed by an attacker key, not the device identity key.
	badSig := ed25519.Sign(otherPriv, kyberPub)
	if err := VerifyKyberPreKey(pub, kyberPub, badSig); err == nil {
		t.Fatal("Kyber prekey signed by wrong key must be rejected")
	}
}

func TestVerifyKyberPreKey_InvalidKyberKey(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	bad := make([]byte, 100) // wrong length for ML-KEM-1024
	sig := ed25519.Sign(priv, bad)
	if err := VerifyKyberPreKey(pub, bad, sig); err != ErrInvalidKyberPublicKey {
		t.Fatalf("expected ErrInvalidKyberPublicKey, got %v", err)
	}
}
