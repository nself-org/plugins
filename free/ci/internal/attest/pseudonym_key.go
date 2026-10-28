package attest

// Purpose: the operator-held secret behind the publication pseudonyms (D-1198). An unkeyed hash of a
// repo id and a host name lets anyone with a dictionary of host names recover the builder id of a
// public provenance statement; an HMAC under a secret the operator keeps does not.
// Inputs:  the CI home directory (LoadPseudonymKey) or a key (UsePseudonymKey).
// Outputs: the key a Profile built afterwards uses for RunnerID and CoordinatorID.
// Constraints: the key is 32 random bytes in <home>/pseudonym.key, mode 0600, never a link, created
//          on first use; a file of another size, mode or type is an error, never replaced. With no
//          key registered a Profile keeps the unkeyed pseudonym (the form the golden statements hold),
//          so a caller that publishes must register one. The key never appears in a document or a log.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
)

// PseudonymKeyFile is the name of the key file under the CI home.
const PseudonymKeyFile = "pseudonym.key"

const pseudonymKeyLen = 32

var (
	pseudonymKey atomic.Pointer[[]byte]
	loadMu       sync.Mutex // one creator at a time within the process
)

// UsePseudonymKey sets the key later Profiles pseudonymize with; nil clears it. A key shorter than 32
// bytes is refused.
func UsePseudonymKey(key []byte) error {
	if key == nil {
		pseudonymKey.Store(nil)
		return nil
	}
	if len(key) < pseudonymKeyLen {
		return errors.New("attest: the pseudonym key must be at least 32 bytes")
	}
	k := append([]byte(nil), key...)
	pseudonymKey.Store(&k)
	return nil
}

func currentPseudonymKey() []byte {
	if k := pseudonymKey.Load(); k != nil {
		return *k
	}
	return nil
}

// LoadPseudonymKey returns the key under home, creating it when the file is absent.
func LoadPseudonymKey(home string) ([]byte, error) {
	if home == "" {
		return nil, errors.New("attest: no CI home for the pseudonym key")
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(home, PseudonymKeyFile)
	loadMu.Lock()
	defer loadMu.Unlock()
	key, err := readPseudonymKey(path)
	if errors.Is(err, os.ErrNotExist) {
		return createPseudonymKey(path)
	}
	return key, err
}

func readPseudonymKey(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() != pseudonymKeyLen || runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("attest: %s must be a regular 32-byte file with mode 0600", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if fs, serr := f.Stat(); serr != nil || !os.SameFile(st, fs) { // the file the checks judged is the file read
		return nil, fmt.Errorf("attest: %s changed while it was read", path)
	}
	key := make([]byte, pseudonymKeyLen)
	if _, err = io.ReadFull(f, key); err != nil {
		return nil, err
	}
	return key, nil
}

// createPseudonymKey writes the key to a private temporary file and links it into place, so no reader
// ever sees a partly written key file and a concurrent creator loses cleanly.
func createPseudonymKey(path string) ([]byte, error) {
	key := make([]byte, pseudonymKeyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pseudonym-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err = tmp.Write(key); err == nil {
		err = tmp.Close()
	} else {
		_ = tmp.Close()
	}
	if err != nil {
		return nil, err
	}
	if err = os.Link(tmp.Name(), path); errors.Is(err, os.ErrExist) {
		return readPseudonymKey(path) // another process created it first
	} else if err != nil {
		return nil, err
	}
	return key, nil
}

// keyedPseudonym is the first 12 hex digits of HMAC-SHA256(key, msg).
func keyedPseudonym(key []byte, msg string) string {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))[:12]
}
