package pki

// Purpose: the key-encryption key and the AES-256-GCM sealing of every private key at rest.
// Inputs:  the pki directory (<data>/pki), a plaintext key and its purpose label.
// Outputs: a 32-byte KEK from <data>/pki/kek (0600, its own file, never in the store or a backup);
//          sealed blobs (nonce || ciphertext).
// Constraints: stdlib crypto/aes + cipher.NewGCM only; the purpose label is the GCM additional data, so
//          a blob sealed for one authority cannot be opened as another; a KEK file readable by group or
//          others is refused; a missing KEK is never silently recreated over existing sealed keys.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// KEKSize is the key length in bytes.
const KEKSize = 32

// ErrNoKEK means sealed keys exist but the KEK file does not (restore the file; never regenerate it).
var ErrNoKEK = errors.New("pki: key-encryption key missing")

// KEKPath is where the KEK of a data directory lives.
func KEKPath(dataDir string) string { return filepath.Join(dataDir, "pki", "kek") }

// LoadKEK reads the KEK. When create is true and the file does not exist it writes a new one (O_EXCL).
func LoadKEK(dataDir string, create bool) ([]byte, error) {
	path := KEKPath(dataDir)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return nil, ErrNoKEK
		}
		return newKEK(path)
	}
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("pki: %s is accessible to others (mode %o); run chmod 600", path, st.Mode().Perm())
	}
	if len(b) != KEKSize {
		return nil, fmt.Errorf("pki: %s has %d bytes, want %d", path, len(b), KEKSize)
	}
	return b, nil
}

func newKEK(path string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	key := make([]byte, KEKSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err = f.Write(key); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return key, nil
}

func gcm(kek []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal encrypts plain under the KEK, bound to purpose.
func Seal(kek, plain []byte, purpose string) ([]byte, error) {
	g, err := gcm(kek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plain, []byte(purpose)), nil
}

// Unseal decrypts a blob made by Seal for the same purpose.
func Unseal(kek, sealed []byte, purpose string) ([]byte, error) {
	g, err := gcm(kek)
	if err != nil {
		return nil, err
	}
	if len(sealed) < g.NonceSize() {
		return nil, errors.New("pki: sealed blob too short")
	}
	return g.Open(nil, sealed[:g.NonceSize()], sealed[g.NonceSize():], []byte(purpose))
}
