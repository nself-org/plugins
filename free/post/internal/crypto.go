package internal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// =============================================================================
// Encryption helpers
// =============================================================================

// encryptCredentials encrypts plaintext using AES-256-GCM and returns a
// base64-encoded nonce||ciphertext string.
func (h *Handlers) encryptCredentials(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(h.Key[:])
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptCredentials decrypts a base64-encoded nonce||ciphertext blob.
func (h *Handlers) decryptCredentials(encoded string) ([]byte, error) {
	combined, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(h.Key[:])
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(combined) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := combined[:nonceSize], combined[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}
