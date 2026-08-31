package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
)

// newKeyRing builds an in-memory KeyRing for tests without touching env or
// the filesystem. The current version is the highest key passed in.
func newKeyRing(t *testing.T, keys map[int]string, current int) *KeyRing {
	t.Helper()
	merged := make(map[int][]byte)
	versions := make([]int, 0, len(keys))
	for v, h := range keys {
		buf, err := decodeKEKHex(h)
		if err != nil {
			t.Fatalf("newKeyRing decode V%d: %v", v, err)
		}
		merged[v] = buf
		versions = append(versions, v)
	}
	// Sort ascending — KeyRing.Versions invariant.
	for i := 1; i < len(versions); i++ {
		for j := i; j > 0 && versions[j-1] > versions[j]; j-- {
			versions[j-1], versions[j] = versions[j], versions[j-1]
		}
	}
	if current == 0 {
		current = versions[len(versions)-1]
	}
	return &KeyRing{keys: merged, versions: versions, current: current}
}

// randomDEK returns a deterministic-length pseudo-random 32-byte DEK.
func randomDEK(t *testing.T) []byte {
	t.Helper()
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return dek
}

func TestEnvelope_RoundTrip_SingleVersion(t *testing.T) {
	kr := newKeyRing(t, map[int]string{1: kek1Hex}, 1)
	dek := randomDEK(t)

	env, err := WrapDEK(kr, dek)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}
	if env.KEKVersion != 1 {
		t.Fatalf("env.KEKVersion = %d, want 1", env.KEKVersion)
	}
	if len(env.Nonce) != 24 {
		t.Fatalf("env.Nonce size = %d, want 24", len(env.Nonce))
	}
	if len(env.Ciphertext) != len(dek)+16 { // +16 = Poly1305 tag
		t.Fatalf("env.Ciphertext size = %d, want %d", len(env.Ciphertext), len(dek)+16)
	}

	out, err := UnwrapDEK(kr, env)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	if !bytes.Equal(out, dek) {
		t.Fatal("round-trip plaintext mismatch")
	}
}

func TestEnvelope_NoncesAreUnique(t *testing.T) {
	kr := newKeyRing(t, map[int]string{1: kek1Hex}, 1)
	dek := randomDEK(t)
	seen := make(map[string]bool)
	for i := 0; i < 16; i++ {
		env, err := WrapDEK(kr, dek)
		if err != nil {
			t.Fatalf("WrapDEK: %v", err)
		}
		nonceKey := string(env.Nonce)
		if seen[nonceKey] {
			t.Fatalf("nonce reused at iteration %d", i)
		}
		seen[nonceKey] = true
	}
}

func TestEnvelope_RotationOldStillReadable(t *testing.T) {
	// Phase 1: write envelope under V1.
	krV1 := newKeyRing(t, map[int]string{1: kek1Hex}, 1)
	dek := randomDEK(t)
	envV1, err := WrapDEK(krV1, dek)
	if err != nil {
		t.Fatalf("WrapDEK V1: %v", err)
	}
	if envV1.KEKVersion != 1 {
		t.Fatalf("envV1.KEKVersion = %d, want 1", envV1.KEKVersion)
	}

	// Phase 2: rotate — V1 stays for reads, V2 is new current for writes.
	krRotated := newKeyRing(t, map[int]string{1: kek1Hex, 2: kek2Hex}, 2)
	if krRotated.Current() != 2 {
		t.Fatalf("post-rotation Current = %d, want 2", krRotated.Current())
	}

	// Phase 2a: old envelope decrypts cleanly using the V1 KEK.
	out, err := UnwrapDEK(krRotated, envV1)
	if err != nil {
		t.Fatalf("UnwrapDEK after rotation: %v", err)
	}
	if !bytes.Equal(out, dek) {
		t.Fatal("old-envelope plaintext mismatch after rotation")
	}

	// Phase 2b: new write under rotated ring uses V2.
	envV2, err := WrapDEK(krRotated, dek)
	if err != nil {
		t.Fatalf("WrapDEK V2: %v", err)
	}
	if envV2.KEKVersion != 2 {
		t.Fatalf("envV2.KEKVersion = %d, want 2", envV2.KEKVersion)
	}
	// And it decrypts.
	out2, err := UnwrapDEK(krRotated, envV2)
	if err != nil {
		t.Fatalf("UnwrapDEK V2: %v", err)
	}
	if !bytes.Equal(out2, dek) {
		t.Fatal("new-envelope plaintext mismatch after rotation")
	}
}

func TestEnvelope_ReWrap_OldToNew(t *testing.T) {
	krV1 := newKeyRing(t, map[int]string{1: kek1Hex}, 1)
	dek := randomDEK(t)
	envOld, err := WrapDEK(krV1, dek)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}

	krRotated := newKeyRing(t, map[int]string{1: kek1Hex, 2: kek2Hex}, 2)
	envNew, err := ReWrap(krRotated, envOld)
	if err != nil {
		t.Fatalf("ReWrap: %v", err)
	}
	if envNew.KEKVersion != 2 {
		t.Fatalf("re-wrapped KEKVersion = %d, want 2", envNew.KEKVersion)
	}
	// Plaintext preserved.
	out, err := UnwrapDEK(krRotated, envNew)
	if err != nil {
		t.Fatalf("UnwrapDEK re-wrapped: %v", err)
	}
	if !bytes.Equal(out, dek) {
		t.Fatal("ReWrap altered plaintext")
	}
	// New envelope's nonce differs from the original.
	if bytes.Equal(envNew.Nonce, envOld.Nonce) {
		t.Fatal("ReWrap reused nonce — must be fresh per envelope")
	}
}

func TestEnvelope_MissingKEKVersionReturnsTypedError(t *testing.T) {
	// Write under V2, then try to decrypt with only V1 in the ring.
	krBoth := newKeyRing(t, map[int]string{1: kek1Hex, 2: kek2Hex}, 2)
	dek := randomDEK(t)
	envV2, err := WrapDEK(krBoth, dek)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}
	krV1Only := newKeyRing(t, map[int]string{1: kek1Hex}, 1)
	_, err = UnwrapDEK(krV1Only, envV2)
	if !errors.Is(err, ErrKEKVersionNotFound) {
		t.Fatalf("UnwrapDEK with missing V2: want ErrKEKVersionNotFound, got %v", err)
	}
}

func TestEnvelope_TamperedCiphertextRejected(t *testing.T) {
	kr := newKeyRing(t, map[int]string{1: kek1Hex}, 1)
	dek := randomDEK(t)
	env, err := WrapDEK(kr, dek)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}
	// Flip a single bit in the ciphertext — AEAD must reject.
	env.Ciphertext[0] ^= 0x01
	if _, err := UnwrapDEK(kr, env); err == nil {
		t.Fatal("tampered ciphertext accepted — AEAD broken")
	}
}

func TestEnvelope_TamperedNonceRejected(t *testing.T) {
	kr := newKeyRing(t, map[int]string{1: kek1Hex}, 1)
	dek := randomDEK(t)
	env, err := WrapDEK(kr, dek)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}
	env.Nonce[0] ^= 0x01
	if _, err := UnwrapDEK(kr, env); err == nil {
		t.Fatal("tampered nonce accepted — AEAD broken")
	}
}

func TestEnvelope_MarshalUnmarshalRoundTrip(t *testing.T) {
	kr := newKeyRing(t, map[int]string{1: kek1Hex, 2: kek2Hex}, 2)
	dek := randomDEK(t)
	env, err := WrapDEK(kr, dek)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}
	raw, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope: %v", err)
	}
	if got.KEKVersion != env.KEKVersion {
		t.Fatalf("KEKVersion: got %d want %d", got.KEKVersion, env.KEKVersion)
	}
	if !bytes.Equal(got.Nonce, env.Nonce) {
		t.Fatal("Nonce round-trip mismatch")
	}
	if !bytes.Equal(got.Ciphertext, env.Ciphertext) {
		t.Fatal("Ciphertext round-trip mismatch")
	}
	// And it still decrypts.
	plain, err := UnwrapDEK(kr, got)
	if err != nil {
		t.Fatalf("UnwrapDEK after marshal round-trip: %v", err)
	}
	if !bytes.Equal(plain, dek) {
		t.Fatal("plaintext mismatch after marshal round-trip")
	}
}

func TestEnvelope_UnmarshalTooShort(t *testing.T) {
	if _, err := UnmarshalEnvelope(nil); !errors.Is(err, ErrEnvelopeTooShort) {
		t.Errorf("nil: want ErrEnvelopeTooShort, got %v", err)
	}
	if _, err := UnmarshalEnvelope([]byte{1, 2, 3}); !errors.Is(err, ErrEnvelopeTooShort) {
		t.Errorf("short: want ErrEnvelopeTooShort, got %v", err)
	}
}

func TestEnvelope_NilGuards(t *testing.T) {
	if _, err := WrapDEK(nil, []byte("x")); err == nil {
		t.Error("WrapDEK(nil): expected error")
	}
	if _, err := UnwrapDEK(nil, &Envelope{KEKVersion: 1}); err == nil {
		t.Error("UnwrapDEK(nil ring): expected error")
	}
	kr := newKeyRing(t, map[int]string{1: kek1Hex}, 1)
	if _, err := UnwrapDEK(kr, nil); err == nil {
		t.Error("UnwrapDEK(nil envelope): expected error")
	}
}
