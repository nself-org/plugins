package internal

import (
	"crypto/ed25519"
	"errors"

	"github.com/cloudflare/circl/kem/mlkem/mlkem1024"
)

// Package crypto helpers for the e2ee key directory.
//
// Purpose:   Provide SERVER-SIDE verification of PUBLIC key material only.
// Inputs:    Raw public-key / signature bytes uploaded by clients.
// Outputs:   Validation errors; no secret material is produced or stored.
// Constraints (CR-C critical):
//   - The server is a KEY DIRECTORY. It NEVER holds a private KEM key and NEVER
//     decapsulates on a user's behalf. Encapsulation/decapsulation is client-side.
//   - All KEM math is delegated to github.com/cloudflare/circl (FIPS 203 /
//     ML-KEM-1024). No KEM primitive is hand-rolled here.
//   - Ed25519 verification uses the Go standard library (crypto/ed25519).
//
// SPORT: np_e2ee_kyber_prekeys, np_e2ee_signed_prekeys.

// Public sizes exposed for handler-level length checks and tests.
const (
	// KyberPublicKeySize is the ML-KEM-1024 (Kyber-1024) encapsulation-key size.
	KyberPublicKeySize = mlkem1024.PublicKeySize
	// Ed25519PublicKeySize / Ed25519SignatureSize from the standard library.
	Ed25519PublicKeySize = ed25519.PublicKeySize
	Ed25519SignatureSize = ed25519.SignatureSize
)

var (
	// ErrInvalidKyberPublicKey is returned when a byte slice does not decode to
	// a valid ML-KEM-1024 public key.
	ErrInvalidKyberPublicKey = errors.New("invalid Kyber-1024 (ML-KEM-1024) public key")
	// ErrInvalidIdentityKey is returned when the identity key is not a valid
	// Ed25519 public key.
	ErrInvalidIdentityKey = errors.New("invalid Ed25519 identity key")
	// ErrBadSignature is returned when a signed-prekey / Kyber-prekey signature
	// does not verify against the device identity key.
	ErrBadSignature = errors.New("signature verification failed")
)

// ValidateKyberPublicKey checks that pub is a well-formed ML-KEM-1024 public key.
//
// It uses circl's UnmarshalBinary, which enforces the exact byte length and the
// internal structural constraints of an ML-KEM-1024 encapsulation key. The
// server stores only this PUBLIC key; it never derives or retains a shared
// secret server-side.
func ValidateKyberPublicKey(pub []byte) error {
	if len(pub) != KyberPublicKeySize {
		return ErrInvalidKyberPublicKey
	}
	scheme := mlkem1024.Scheme()
	if _, err := scheme.UnmarshalBinaryPublicKey(pub); err != nil {
		return ErrInvalidKyberPublicKey
	}
	return nil
}

// VerifyEd25519Signature verifies that sig is an Ed25519 signature of message
// under identityKey. Used to validate that an uploaded signed prekey (classic
// X25519) or Kyber prekey was actually signed by the device's identity key.
//
// Returns nil on success, ErrInvalidIdentityKey for a malformed key, or
// ErrBadSignature when verification fails.
func VerifyEd25519Signature(identityKey, message, sig []byte) error {
	if len(identityKey) != Ed25519PublicKeySize {
		return ErrInvalidIdentityKey
	}
	if len(sig) != Ed25519SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(ed25519.PublicKey(identityKey), message, sig) {
		return ErrBadSignature
	}
	return nil
}

// VerifySignedPreKey verifies a classic (X25519) signed prekey: the signature
// must cover the prekey public bytes and verify under the device identity key.
func VerifySignedPreKey(identityKey, prekeyPublic, sig []byte) error {
	return VerifyEd25519Signature(identityKey, prekeyPublic, sig)
}

// VerifyKyberPreKey verifies a Kyber-1024 prekey on upload: the Kyber public key
// must be structurally valid AND its Ed25519 signature must verify under the
// device identity key. This binds the post-quantum prekey to the identity,
// preventing an attacker from substituting an unsigned Kyber key.
func VerifyKyberPreKey(identityKey, kyberPublic, sig []byte) error {
	if err := ValidateKyberPublicKey(kyberPublic); err != nil {
		return err
	}
	return VerifyEd25519Signature(identityKey, kyberPublic, sig)
}
