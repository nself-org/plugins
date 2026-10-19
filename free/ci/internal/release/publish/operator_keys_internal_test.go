package publish

// Purpose: readOperatorKeys fails closed (D-1248): an unreadable trust file is
// never read as an empty one, so verification never runs on fewer keys than
// the operator trusts.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOperatorKeysAbsentIsEmpty(t *testing.T) {
	b, err := readOperatorKeys(filepath.Join(t.TempDir(), "ci-release.keys"))
	if err != nil || len(b) != 0 {
		t.Fatalf("absent file: %q, %v", b, err)
	}
}

func TestReadOperatorKeysFailsClosed(t *testing.T) {
	dir := t.TempDir()
	notRegular := filepath.Join(dir, "dir.keys")
	if err := os.Mkdir(notRegular, 0o700); err != nil {
		t.Fatal(err)
	}
	over := filepath.Join(dir, "over.keys")
	if err := os.WriteFile(over, bytes.Repeat([]byte("k"), maxOperatorKeys+1), 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(dir, "locked.keys")
	if err := os.WriteFile(unreadable, []byte("k\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	if f, err := os.Open(unreadable); err == nil { // root reads anything: this case cannot be tested
		_ = f.Close()
		unreadable = ""
	}
	cases := map[string]string{"non-regular file": notRegular, "over the cap": over, "unreadable": unreadable}
	for name, path := range cases {
		if path == "" {
			t.Logf("%s: skipped, the process can read a mode 0 file", name)
			continue
		}
		if _, err := readOperatorKeys(path); err == nil {
			t.Errorf("%s: want an error, got none", name)
		}
	}
}

func TestReadOperatorKeysAcceptsExactlyTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ci-release.keys")
	if err := os.WriteFile(path, bytes.Repeat([]byte("k"), maxOperatorKeys), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := readOperatorKeys(path); err != nil || len(b) != maxOperatorKeys {
		t.Fatalf("a file of exactly the cap: %d bytes, %v", len(b), err)
	}
}
