package crypto

// Envelope encryption primitives for nself-vault.
//
// Two-layer scheme:
//
//   Plaintext DEK (32 bytes, generated per-record) ──XChaCha20-Poly1305──▶ Envelope
//                                                          ▲
//                                                          │
//                          KEK[version] (32 bytes, in env or file)
//
// Storage shape:
//
//   { KEKVersion: 1, Nonce: 24B, Ciphertext: 32B DEK + 16B Poly1305 tag = 48B }
//
// The KEKVersion is stored alongside the ciphertext so decrypt can select the
// correct KEK without trying every configured version. This is identical to the
// `kek_version` column in np_vault_keys (SPEC.md § Schema).
//
// The envelope is REPLACED on rotation: a re-wrap reads the old envelope using
// KEK[old_version], then writes a new envelope using KEK[current_version]. The
// underlying DEK plaintext is unchanged, so data encrypted under that DEK
// remains decryptable without re-encrypting application records.

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

// Envelope is the on-disk shape of a wrapped DEK.
type Envelope struct {
	// KEKVersion identifies which KEK encrypted Ciphertext.
	KEKVersion int
	// Nonce is the 24-byte XChaCha20-Poly1305 nonce. Random per envelope.
	Nonce []byte
	// Ciphertext is the encrypted DEK plus the 16-byte Poly1305 auth tag.
	Ciphertext []byte
}

// ErrEnvelopeTooShort is returned when bytes passed to UnmarshalEnvelope are
// shorter than the minimum frame header + nonce + tag size.
var ErrEnvelopeTooShort = errors.New("vault/crypto: envelope too short")

// ErrEnvelopeBadVersion is returned when the version field in a serialised
// envelope is non-positive.
var ErrEnvelopeBadVersion = errors.New("vault/crypto: envelope has invalid KEK version")

// envelopeFrameVersion is bumped if/when the wire format changes.
const envelopeFrameVersion uint8 = 1

// envelopeHeaderSize is: 1 byte frame version + 4 bytes KEK version (uint32 big-endian).
const envelopeHeaderSize = 1 + 4

// WrapDEK encrypts plaintext (typically a 32-byte DEK) under the current KEK
// in the KeyRing. The returned Envelope can be marshalled to bytes for storage.
//
// Callers MUST NOT reuse Nonce values across envelopes encrypted with the same
// KEK; this function generates a fresh random 24-byte nonce each call.
func WrapDEK(kr *KeyRing, plaintext []byte) (*Envelope, error) {
	if kr == nil {
		return nil, errors.New("vault/crypto: nil KeyRing")
	}
	kek, err := kr.CurrentKey()
	if err != nil {
		return nil, err
	}
	return wrapWith(kek, kr.Current(), plaintext)
}

// UnwrapDEK decrypts the envelope using the KEK version named in env.KEKVersion.
// Returns ErrKEKVersionNotFound (typed, wrappable) if that version is not
// configured in the KeyRing. Returns a generic error on AEAD verification
// failure — the caller MUST NOT distinguish "wrong key" from "tampered
// ciphertext" in user-facing error messages.
func UnwrapDEK(kr *KeyRing, env *Envelope) ([]byte, error) {
	if kr == nil {
		return nil, errors.New("vault/crypto: nil KeyRing")
	}
	if env == nil {
		return nil, errors.New("vault/crypto: nil Envelope")
	}
	kek, err := kr.Get(env.KEKVersion)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(kek)
	if err != nil {
		return nil, fmt.Errorf("vault/crypto: AEAD init: %w", err)
	}
	if len(env.Nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("vault/crypto: bad nonce length %d", len(env.Nonce))
	}
	plain, err := aead.Open(nil, env.Nonce, env.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("vault/crypto: open: %w", err)
	}
	return plain, nil
}

// ReWrap is the rotation helper: it unwraps using whatever KEK encrypted env,
// then re-wraps under the current KEK. Idempotent — if env.KEKVersion already
// equals kr.Current(), the function still re-runs (with a fresh nonce) so the
// caller can use a single code path for "make sure this envelope uses the
// newest KEK".
func ReWrap(kr *KeyRing, env *Envelope) (*Envelope, error) {
	plain, err := UnwrapDEK(kr, env)
	if err != nil {
		return nil, err
	}
	defer zero(plain)
	return WrapDEK(kr, plain)
}

// Marshal serialises an Envelope to bytes for DB storage:
//
//   [1 byte frame=1] [4 bytes KEKVersion BE] [24 bytes nonce] [N bytes ciphertext+tag]
//
// The frame byte allows future format upgrades without breaking on-disk data.
func (e *Envelope) Marshal() ([]byte, error) {
	if e == nil {
		return nil, errors.New("vault/crypto: nil envelope")
	}
	if e.KEKVersion < 1 {
		return nil, ErrEnvelopeBadVersion
	}
	if len(e.Nonce) != chacha20poly1305.NonceSizeX {
		return nil, fmt.Errorf("vault/crypto: marshal nonce wrong size %d", len(e.Nonce))
	}
	out := make([]byte, 0, envelopeHeaderSize+len(e.Nonce)+len(e.Ciphertext))
	out = append(out, envelopeFrameVersion)
	verBytes := [4]byte{}
	binary.BigEndian.PutUint32(verBytes[:], uint32(e.KEKVersion))
	out = append(out, verBytes[:]...)
	out = append(out, e.Nonce...)
	out = append(out, e.Ciphertext...)
	return out, nil
}

// UnmarshalEnvelope parses an envelope from the byte form produced by Marshal.
func UnmarshalEnvelope(b []byte) (*Envelope, error) {
	if len(b) < envelopeHeaderSize+chacha20poly1305.NonceSizeX+chacha20poly1305.Overhead {
		return nil, ErrEnvelopeTooShort
	}
	frame := b[0]
	if frame != envelopeFrameVersion {
		return nil, fmt.Errorf("vault/crypto: unknown envelope frame version %d", frame)
	}
	ver := int(binary.BigEndian.Uint32(b[1:5]))
	if ver < 1 {
		return nil, ErrEnvelopeBadVersion
	}
	nonceStart := envelopeHeaderSize
	nonceEnd := nonceStart + chacha20poly1305.NonceSizeX
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	copy(nonce, b[nonceStart:nonceEnd])
	ct := make([]byte, len(b)-nonceEnd)
	copy(ct, b[nonceEnd:])
	return &Envelope{KEKVersion: ver, Nonce: nonce, Ciphertext: ct}, nil
}

// wrapWith encrypts plaintext with the supplied KEK + version. Internal — the
// public API forces use of the KeyRing so the version recorded in the envelope
// always matches the KEK material that signed it.
func wrapWith(kek []byte, version int, plaintext []byte) (*Envelope, error) {
	if len(kek) != KeySize {
		return nil, ErrInvalidKEKHex
	}
	if version < 1 {
		return nil, ErrEnvelopeBadVersion
	}
	aead, err := chacha20poly1305.NewX(kek)
	if err != nil {
		return nil, fmt.Errorf("vault/crypto: AEAD init: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("vault/crypto: nonce gen: %w", err)
	}
	ct := aead.Seal(nil, nonce, plaintext, nil)
	return &Envelope{KEKVersion: version, Nonce: nonce, Ciphertext: ct}, nil
}

// zero overwrites b with zeroes. Best-effort — Go's GC may copy the underlying
// memory before this runs. Real protection awaits memguard integration (V05-F1,
// deferred S03).
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
