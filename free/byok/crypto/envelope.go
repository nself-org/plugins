// Package crypto implements AES-256-GCM envelope encryption for BYOK.
// The DEK is generated fresh per record-group, wrapped by the tenant's CMK
// via a KMSProvider, and stored alongside the ciphertext.
package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/nself-org/nself-byok/kms"
)

const (
	// DEKSize is 256-bit (32 bytes) for AES-256-GCM.
	DEKSize = 32
	// NonceSize is 12 bytes as required by GCM.
	NonceSize = 12
)

// EncryptedBundle holds all material needed to decrypt a record later.
type EncryptedBundle struct {
	Ciphertext   []byte // AES-256-GCM ciphertext
	EncryptedDEK []byte // DEK wrapped by CMK
	IV           []byte // 12-byte GCM nonce
	KMSKeyRef    string // which CMK wrapped this DEK
	Algorithm    string // always "AES-256-GCM"
}

// Envelope provides encrypt and decrypt operations over a KMSProvider.
type Envelope struct {
	provider kms.KMSProvider
}

// NewEnvelope creates an Envelope backed by the given KMSProvider.
func NewEnvelope(provider kms.KMSProvider) *Envelope {
	return &Envelope{provider: provider}
}

// Encrypt generates a fresh 256-bit DEK, encrypts plaintext with
// AES-256-GCM, wraps the DEK using the CMK identified by kmsKeyRef,
// and returns the EncryptedBundle. The plaintext DEK is zeroed before
// the function returns.
func (e *Envelope) Encrypt(ctx context.Context, plaintext []byte, kmsKeyRef string) (EncryptedBundle, error) {
	// 1. Generate fresh DEK.
	dek := make([]byte, DEKSize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return EncryptedBundle{}, fmt.Errorf("envelope encrypt: generate DEK: %w", err)
	}
	defer zeroBytes(dek)

	// 2. Generate 12-byte nonce.
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return EncryptedBundle{}, fmt.Errorf("envelope encrypt: generate nonce: %w", err)
	}

	// 3. Encrypt plaintext with AES-256-GCM.
	block, err := aes.NewCipher(dek)
	if err != nil {
		return EncryptedBundle{}, fmt.Errorf("envelope encrypt: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedBundle{}, fmt.Errorf("envelope encrypt: new GCM: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// 4. Wrap DEK with CMK.
	wrappedDEK, err := e.provider.WrapKey(ctx, dek, kmsKeyRef)
	if err != nil {
		return EncryptedBundle{}, fmt.Errorf("envelope encrypt: wrap DEK: %w", err)
	}

	return EncryptedBundle{
		Ciphertext:   ciphertext,
		EncryptedDEK: wrappedDEK,
		IV:           nonce,
		KMSKeyRef:    kmsKeyRef,
		Algorithm:    "AES-256-GCM",
	}, nil
}

// Decrypt unwraps the DEK using the CMK, then decrypts the ciphertext.
// If the CMK has been revoked, UnwrapKey returns an error and this function
// surfaces it — no fallback to a cached plaintext DEK is attempted here.
// DEK caching (optional Enterprise feature) is handled at a higher layer
// (see cache/dek_cache.go) which must re-call Decrypt on cache miss.
func (e *Envelope) Decrypt(ctx context.Context, bundle EncryptedBundle) ([]byte, error) {
	// 1. Unwrap DEK using CMK. Revoked key → error propagates to caller.
	dek, err := e.provider.UnwrapKey(ctx, bundle.EncryptedDEK, bundle.KMSKeyRef)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: unwrap DEK: %w", err)
	}
	defer zeroBytes(dek)

	// 2. Decrypt ciphertext with AES-256-GCM.
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: new GCM: %w", err)
	}
	plaintext, err := gcm.Open(nil, bundle.IV, bundle.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: GCM open: %w", err)
	}
	return plaintext, nil
}

// zeroBytes overwrites b with zeros to limit DEK lifetime in memory.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
