package nodekey

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// TestNodeKeyPersists proves the acceptance line "The node key file is 0600,
// stable across restarts, and its key id appears in hello": the key id is the
// first 16 hex of the SHA-256 of the public key and a restart loads the same key.
func TestNodeKeyPersists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	k1, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if k1.KeyID() != k2.KeyID() || k1.PublicString() != k2.PublicString() {
		t.Fatal("key changed across restart")
	}
	sum := sha256.Sum256(k1.Public())
	if want := hex.EncodeToString(sum[:])[:16]; k1.KeyID() != want {
		t.Fatalf("key id %s, want %s", k1.KeyID(), want)
	}
	sig, err := k1.Sign([]byte("m"))
	if err != nil || !ed25519.Verify(k2.Public(), []byte("m"), sig) {
		t.Fatal("signature does not verify with the reloaded key")
	}
	if runtime.GOOS == "windows" {
		return
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, FileName): 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s mode %v err %v, want %o", path, info.Mode().Perm(), err, want)
		}
	}
}

// TestNodeKeyRefusesLoosePermissions proves a key readable by group or other is
// refused rather than silently used or repaired.
func TestNodeKeyRefusesLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
	dir := t.TempDir()
	if _, err := LoadOrCreate(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, FileName), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(dir); err == nil {
		t.Fatal("accepted a 0644 key")
	}
}

// TestNodeKeyRefusesSymlinkAndGarbage proves a symlinked or malformed key file
// is an error, never a regenerated key.
func TestNodeKeyRefusesSymlinkAndGarbage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges")
	}
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, FileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(dir); err == nil {
		t.Fatal("followed a symlink")
	}
	_ = os.Remove(filepath.Join(dir, FileName))
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(dir); err == nil {
		t.Fatal("accepted garbage")
	}
}

// TestNodeKeyConcurrentCreate proves racing first starts agree on one key.
func TestNodeKeyConcurrentCreate(t *testing.T) {
	dir := t.TempDir()
	ids := make([]string, 8)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			k, err := LoadOrCreate(dir)
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = k.KeyID()
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("key ids differ: %v", ids)
		}
	}
}
