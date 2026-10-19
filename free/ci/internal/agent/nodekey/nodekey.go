// Package nodekey owns the node's Ed25519 signing key (Epic D13, ADR 0025 §9).
//
// Purpose: create the key on first start and load it on every later start.
// Inputs:  the agent state directory.
// Outputs: a Key that reports its public half, its key id and signs bytes.
// Constraints: the state dir is 0700 and the key file 0600; the key file is
// created atomically and never replaced, so two starting agents agree on one
// key; a key file with group or other access is refused, not repaired. The
// package never hands out the private half (callers get a Signer). It does not
// hide the file from a job running as the same UID: in user mode that
// separation comes from the slot-derived job UID (P7-TRUST-09) or container
// isolation, and in system mode from the agent user owning a 0700 state dir
// (P7-NODE-06).
package nodekey

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileName is the key file under the state directory.
const FileName = "node.key"

// Signer signs bytes with the node key (P7-TRUST implements the evidence
// payload on top of it).
type Signer interface {
	KeyID() string
	Sign(msg []byte) ([]byte, error)
}

// Key is the node signing key.
type Key struct{ priv ed25519.PrivateKey }

var _ Signer = (*Key)(nil)

// KeyIDOf is the first 16 hex characters of the SHA-256 of the raw public key.
func KeyIDOf(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])[:16]
}

// EnsureDir creates dir as 0700 (or tightens an existing directory to it) and
// refuses a symlink or a non-directory.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("state dir %s is not a directory", dir)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// CheckPrivateFile refuses anything but a regular, non-symlink file with no
// group or other permission bits.
func CheckPrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s has permissions %o; want 0600", path, info.Mode().Perm())
	}
	return nil
}

// WriteExclusive writes data to path with mode 0600 only if path does not
// exist: the content is written and synced to a temporary file and hard-linked
// into place, so the final name is never seen half written (a power loss
// cannot leave an empty key) and a concurrent creator wins cleanly (fs.ErrExist
// for the loser). Where hard links are unavailable it falls back to an
// exclusive create.
func WriteExclusive(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	err = os.Link(tmp.Name(), path)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		err = createExclusive(path, data) // no hard links here (some network homes)
	}
	if err == nil {
		syncDir(filepath.Dir(path))
	}
	return err
}

func createExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path) // never leave a partial key behind
	}
	return err
}

// syncDir flushes a directory entry; best effort (not every platform allows it).
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// LoadOrCreate returns the node key of stateDir, creating it on first use.
func LoadOrCreate(stateDir string) (*Key, error) {
	if err := EnsureDir(stateDir); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, FileName)
	for range 2 {
		k, err := load(path)
		if err == nil {
			return k, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, err
		}
		err = WriteExclusive(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
		if err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("node key %s could not be created", path)
}

func load(path string) (*Key, error) {
	if err := CheckPrivateFile(path); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("%s is not a PKCS#8 private key", path)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an Ed25519 key", path)
	}
	return &Key{priv: priv}, nil
}

// Public is the raw public key.
func (k *Key) Public() ed25519.PublicKey { return k.priv.Public().(ed25519.PublicKey) }

// PublicString is the standard base64 of the raw public key (the hello field).
func (k *Key) PublicString() string { return base64.StdEncoding.EncodeToString(k.Public()) }

// KeyID is the key id sent in hello.
func (k *Key) KeyID() string { return KeyIDOf(k.Public()) }

// Sign signs msg with the node key.
func (k *Key) Sign(msg []byte) ([]byte, error) { return ed25519.Sign(k.priv, msg), nil }
