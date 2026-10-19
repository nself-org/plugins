package pki

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKEKPath(t *testing.T) {
	if got := KEKPath("/d"); got != filepath.Join("/d", "pki", "kek") {
		t.Fatalf("path %q", got)
	}
}

func TestLoadKEKCreatesOnceAndReloads(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadKEK(dir, false); !errors.Is(err, ErrNoKEK) {
		t.Fatalf("missing KEK without create: %v", err)
	}
	if _, err := os.Stat(KEKPath(dir)); !os.IsNotExist(err) {
		t.Fatal("LoadKEK(create=false) created a file")
	}
	k1, err := LoadKEK(dir, true)
	if err != nil || len(k1) != KEKSize || KEKSize != 32 {
		t.Fatalf("create: %d %v", len(k1), err)
	}
	if bytes.Equal(k1, make([]byte, KEKSize)) {
		t.Fatal("KEK is all zero")
	}
	st, err := os.Stat(KEKPath(dir))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("kek mode %v %v", st.Mode(), err)
	}
	dst, err := os.Stat(filepath.Dir(KEKPath(dir)))
	if err != nil || dst.Mode().Perm() != 0o700 {
		t.Fatalf("pki dir mode %v %v", dst.Mode(), err)
	}
	onDisk, _ := os.ReadFile(KEKPath(dir))
	if !bytes.Equal(onDisk, k1) {
		t.Fatal("returned KEK differs from the file")
	}
	for _, create := range []bool{false, true} { // an existing file is never replaced
		k2, err := LoadKEK(dir, create)
		if err != nil || !bytes.Equal(k1, k2) {
			t.Fatalf("reload create=%v: %v", create, err)
		}
	}
	other, err := LoadKEK(t.TempDir(), true)
	if err != nil || bytes.Equal(other, k1) {
		t.Fatalf("two KEKs must differ: %v", err)
	}
}

func TestLoadKEKRefusals(t *testing.T) {
	good := bytes.Repeat([]byte{7}, KEKSize)
	tests := []struct {
		name    string
		content []byte
		mode    os.FileMode
		wantErr string // "" = accepted
	}{
		{"0600", good, 0o600, ""},
		{"0400", good, 0o400, ""},
		{"group read", good, 0o640, "(mode 640); run chmod 600"},
		{"group write", good, 0o620, "mode 620"},
		{"group exec", good, 0o610, "mode 610"},
		{"other read", good, 0o604, "mode 604"},
		{"other write", good, 0o602, "mode 602"},
		{"other exec", good, 0o601, "mode 601"},
		{"world", good, 0o666, "mode 666"},
		{"31 bytes", good[:31], 0o600, "has 31 bytes, want 32"},
		{"33 bytes", append(append([]byte{}, good...), 1), 0o600, "has 33 bytes, want 32"},
		{"empty", nil, 0o600, "has 0 bytes, want 32"},
		{"loose mode wins over bad length", good[:5], 0o644, "accessible to others"},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := KEKPath(dir)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, c.content, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, c.mode); err != nil {
				t.Fatal(err)
			}
			for _, create := range []bool{false, true} {
				k, err := LoadKEK(dir, create)
				if c.wantErr == "" {
					if err != nil || !bytes.Equal(k, good) {
						t.Fatalf("create=%v: %v", create, err)
					}
					continue
				}
				if err == nil || !strings.Contains(err.Error(), c.wantErr) || errors.Is(err, ErrNoKEK) || k != nil {
					t.Fatalf("create=%v: key=%v err=%v, want %q", create, k, err, c.wantErr)
				}
				if bytes.Contains([]byte(err.Error()), good) {
					t.Fatal("error carries key bytes")
				}
			}
		})
	}
}

func TestLoadKEKFilesystemErrors(t *testing.T) {
	// the KEK path is a directory: a read error, not "missing"
	dir := t.TempDir()
	if err := os.MkdirAll(KEKPath(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKEK(dir, true); err == nil || errors.Is(err, ErrNoKEK) {
		t.Fatalf("directory as KEK: %v", err)
	}
	// the pki directory cannot be created: <data>/pki is a file
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "pki"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if k, err := LoadKEK(dir2, true); err == nil || k != nil {
		t.Fatalf("pki is a file: %v %v", k, err)
	}
	// unreadable file
	if os.Geteuid() != 0 {
		dir3 := t.TempDir()
		k, err := LoadKEK(dir3, true)
		if err != nil || k == nil {
			t.Fatal(err)
		}
		if err = os.Chmod(KEKPath(dir3), 0); err != nil {
			t.Fatal(err)
		}
		if _, err = LoadKEK(dir3, true); err == nil || errors.Is(err, ErrNoKEK) {
			t.Fatalf("unreadable KEK: %v", err)
		}
	}
}

func TestSealUnseal(t *testing.T) {
	kek := bytes.Repeat([]byte{1}, KEKSize)
	plain := []byte("a private key goes here")
	orig := append([]byte{}, plain...)
	s1, err := Seal(kek, plain, "authority:root")
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := Seal(kek, plain, "authority:root")
	if bytes.Equal(s1, s2) {
		t.Fatal("two seals of one plaintext are identical (nonce reuse)")
	}
	if len(s1) != 12+len(plain)+16 {
		t.Fatalf("sealed length %d", len(s1))
	}
	if bytes.Contains(s1, plain) || !bytes.Equal(plain, orig) {
		t.Fatal("plaintext leaked into the blob or was modified")
	}
	got, err := Unseal(kek, s1, "authority:root")
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip: %q %v", got, err)
	}
	// purpose binding
	for _, p := range []string{"authority:node", "", "token-key", "authority:root "} {
		if _, err = Unseal(kek, s1, p); err == nil {
			t.Fatalf("opened under purpose %q", p)
		}
	}
	// wrong key
	if _, err = Unseal(bytes.Repeat([]byte{2}, KEKSize), s1, "authority:root"); err == nil {
		t.Fatal("opened under another KEK")
	}
	// tamper: every byte of nonce, ciphertext and tag
	for i := range s1 {
		bad := append([]byte{}, s1...)
		bad[i] ^= 1
		if _, err = Unseal(kek, bad, "authority:root"); err == nil {
			t.Fatalf("tampered byte %d accepted", i)
		}
	}
	// length edges: below the nonce is "too short", exactly the nonce is a failed open
	if _, err = Unseal(kek, s1[:11], "x"); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("11 bytes: %v", err)
	}
	if _, err = Unseal(kek, s1[:12], "x"); err == nil || strings.Contains(err.Error(), "too short") {
		t.Fatalf("12 bytes: %v", err)
	}
	if _, err = Unseal(kek, nil, "x"); err == nil {
		t.Fatal("nil blob accepted")
	}
	// empty plaintext
	e, err := Seal(kek, nil, "empty")
	if err != nil || len(e) != 12+16 {
		t.Fatalf("empty seal: %d %v", len(e), err)
	}
	if got, err = Unseal(kek, e, "empty"); err != nil || len(got) != 0 {
		t.Fatalf("empty unseal: %q %v", got, err)
	}
	// bad KEK length
	if _, err = Seal(kek[:15], plain, "x"); err == nil {
		t.Fatal("Seal accepted a 15-byte KEK")
	}
	if _, err = Unseal(kek[:15], s1, "x"); err == nil {
		t.Fatal("Unseal accepted a 15-byte KEK")
	}
}
