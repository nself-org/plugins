package crypto

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kek1Hex / kek2Hex / kek3Hex are deterministic test KEKs (32 bytes each).
// NEVER use these in production — they are public test fixtures.
const (
	kek1Hex = "0101010101010101010101010101010101010101010101010101010101010101"
	kek2Hex = "0202020202020202020202020202020202020202020202020202020202020202"
	kek3Hex = "0303030303030303030303030303030303030303030303030303030303030303"
)

// clearKEKEnv removes every VAULT_KEK_V* and the current-version selector from
// the process env for the lifetime of the test. Required because t.Setenv only
// sets, it does not unset values pre-existing from the harness env.
func clearKEKEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		key := kv[:eq]
		if strings.HasPrefix(key, EnvVarPrefix) || key == CurrentVersionEnv || key == DirEnv {
			t.Setenv(key, "")
		}
	}
}

func TestLoad_SingleVersionFromEnv(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	// Point DirEnv at a guaranteed-empty dir so file fallback doesn't interfere.
	t.Setenv(DirEnv, t.TempDir())

	kr, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if kr.Current() != 1 {
		t.Fatalf("Current() = %d, want 1", kr.Current())
	}
	if got := kr.Versions(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("Versions() = %v, want [1]", got)
	}
	k, err := kr.CurrentKey()
	if err != nil {
		t.Fatalf("CurrentKey: %v", err)
	}
	if len(k) != KeySize {
		t.Fatalf("CurrentKey len = %d, want %d", len(k), KeySize)
	}
}

func TestLoad_MultipleVersionsPicksHighest(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_V2", kek2Hex)
	t.Setenv("VAULT_KEK_V3", kek3Hex)
	t.Setenv(DirEnv, t.TempDir())

	kr, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if kr.Current() != 3 {
		t.Fatalf("Current() = %d, want 3 (newest)", kr.Current())
	}
	if vs := kr.Versions(); len(vs) != 3 || vs[0] != 1 || vs[2] != 3 {
		t.Fatalf("Versions() = %v, want [1 2 3]", vs)
	}
	// All three versions must be retrievable for decrypt.
	for _, v := range []int{1, 2, 3} {
		if !kr.Has(v) {
			t.Errorf("Has(%d) = false, want true (decrypt path needs all versions)", v)
		}
		if _, err := kr.Get(v); err != nil {
			t.Errorf("Get(%d): %v", v, err)
		}
	}
}

func TestLoad_CurrentVersionOverride(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_V2", kek2Hex)
	t.Setenv("VAULT_KEK_V3", kek3Hex)
	t.Setenv(CurrentVersionEnv, "2")
	t.Setenv(DirEnv, t.TempDir())

	kr, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if kr.Current() != 2 {
		t.Fatalf("Current() = %d, want 2 (override)", kr.Current())
	}
}

func TestLoad_CurrentVersionOverride_NotConfigured(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv(CurrentVersionEnv, "5") // V5 not configured
	t.Setenv(DirEnv, t.TempDir())

	_, err := Load()
	if err == nil {
		t.Fatal("Load: expected error for unconfigured override version")
	}
	if !errors.Is(err, ErrKEKVersionNotFound) {
		t.Errorf("error = %v, want wraps ErrKEKVersionNotFound", err)
	}
}

func TestLoad_NoKEKsConfigured(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv(DirEnv, t.TempDir())

	_, err := Load()
	if !errors.Is(err, ErrNoKEKsConfigured) {
		t.Fatalf("Load: want ErrNoKEKsConfigured, got %v", err)
	}
}

func TestLoad_InvalidHexRejected(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv("VAULT_KEK_V1", "not-hex")
	t.Setenv(DirEnv, t.TempDir())

	_, err := Load()
	if err == nil {
		t.Fatal("Load: expected error for invalid hex")
	}
	if !errors.Is(err, ErrInvalidKEKHex) {
		t.Errorf("error = %v, want wraps ErrInvalidKEKHex", err)
	}
}

func TestLoad_WrongLengthHexRejected(t *testing.T) {
	clearKEKEnv(t)
	// 60 hex chars = 30 bytes, not 32.
	t.Setenv("VAULT_KEK_V1", strings.Repeat("ab", 30))
	t.Setenv(DirEnv, t.TempDir())

	_, err := Load()
	if !errors.Is(err, ErrInvalidKEKHex) {
		t.Errorf("error = %v, want wraps ErrInvalidKEKHex", err)
	}
}

func TestLoad_EmptyEnvVarIgnored(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_V2", "") // blank — must be ignored, not error
	t.Setenv(DirEnv, t.TempDir())

	kr, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if kr.Current() != 1 {
		t.Fatalf("Current() = %d, want 1 (V2 blank, ignored)", kr.Current())
	}
}

func TestLoad_FileFallback(t *testing.T) {
	clearKEKEnv(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "v1.key"), []byte(kek1Hex+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "v2.key"), []byte("VAULT_KEK_V2="+kek2Hex+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(DirEnv, dir)

	kr, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if kr.Current() != 2 {
		t.Fatalf("Current() = %d, want 2", kr.Current())
	}
	if !kr.Has(1) || !kr.Has(2) {
		t.Fatal("file fallback failed to load both versions")
	}
}

func TestLoad_EnvWinsOverFile(t *testing.T) {
	clearKEKEnv(t)
	dir := t.TempDir()
	// File says V1 = kek2Hex.
	if err := os.WriteFile(filepath.Join(dir, "v1.key"), []byte(kek2Hex), 0o600); err != nil {
		t.Fatal(err)
	}
	// Env says V1 = kek1Hex — env must win.
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv(DirEnv, dir)

	kr, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := kr.Get(1)
	if err != nil {
		t.Fatalf("Get(1): %v", err)
	}
	want, _ := decodeKEKHex(kek1Hex)
	if !bytes.Equal(got, want) {
		t.Fatal("env did not override file for V1")
	}
}

func TestGet_UnknownVersion(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv(DirEnv, t.TempDir())

	kr, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, err = kr.Get(99)
	if !errors.Is(err, ErrKEKVersionNotFound) {
		t.Fatalf("Get(99): want ErrKEKVersionNotFound, got %v", err)
	}
}

func TestVersions_ReturnsCopy(t *testing.T) {
	clearKEKEnv(t)
	t.Setenv("VAULT_KEK_V1", kek1Hex)
	t.Setenv("VAULT_KEK_V2", kek2Hex)
	t.Setenv(DirEnv, t.TempDir())

	kr, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := kr.Versions()
	got[0] = 999 // mutate caller's copy
	if vs := kr.Versions(); vs[0] != 1 {
		t.Fatalf("Versions caller-mutation leaked back: %v", vs)
	}
}
