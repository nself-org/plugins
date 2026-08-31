// Package crypto implements the KEK (Key Encryption Key) loader and envelope
// encryption primitives for nself-vault.
//
// The KEK scheme uses version-numbered keys (V1, V2, ...) following the same
// dual-key rotation pattern as cli/internal/auth/jwt_rotation.go:
//
//   - VAULT_KEK_V{N} env vars hold the 32-byte (64-hex-char) KEKs.
//   - VAULT_CURRENT_KEK_VERSION names the version used for NEW envelopes.
//   - ALL configured versions are retained for DECRYPT during the grace window.
//   - When VAULT_CURRENT_KEK_VERSION is not set, the loader selects the highest
//     present version automatically (newest-for-writes).
//
// Storage strategy (v1.1.2):
//
//  1. Env vars VAULT_KEK_V1, VAULT_KEK_V2, ... — primary path.
//  2. File fallback at /etc/nself-vault/keks/v{N}.key — used when env is empty
//     AND the file is readable. Files contain one 64-character lowercase hex
//     line. Path is overridable via VAULT_KEK_DIR.
//
// KMS-backed loaders (AWS KMS, GCP KMS, HashiCorp Vault Transit) are planned
// for v1.2 — see docs/kek-rotation.md.
package crypto

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// KeySize is the required KEK length in bytes (32 = XChaCha20-Poly1305 key size).
const KeySize = 32

// EnvVarPrefix is the env var name prefix for KEK versions: VAULT_KEK_V{N}.
const EnvVarPrefix = "VAULT_KEK_V"

// CurrentVersionEnv names the env var that selects which KEK encrypts new
// envelopes. When empty, the loader picks the highest known version.
const CurrentVersionEnv = "VAULT_CURRENT_KEK_VERSION"

// DirEnv overrides the default file-fallback directory.
const DirEnv = "VAULT_KEK_DIR"

// DefaultDir is the file-fallback directory checked when env vars are empty.
const DefaultDir = "/etc/nself-vault/keks"

// ErrKEKVersionNotFound is returned when a requested KEK version is not
// configured in the loader. Callers MUST treat this as a typed error so that
// decrypt-with-unknown-version is distinguishable from cryptographic failure.
var ErrKEKVersionNotFound = errors.New("vault/crypto: KEK version not configured")

// ErrNoKEKsConfigured is returned by Load when zero KEK versions are present
// in env AND the file-fallback directory yields no readable keys. This is a
// startup-fatal condition.
var ErrNoKEKsConfigured = errors.New("vault/crypto: no KEK versions configured (set VAULT_KEK_V1)")

// ErrInvalidKEKHex is returned when a KEK value is not a 64-character
// lowercase-hex string. Uppercase hex is accepted (Go's hex.DecodeString is
// case-insensitive) but the canonical form is lowercase.
var ErrInvalidKEKHex = errors.New("vault/crypto: KEK must be 32 bytes hex-encoded (64 hex chars)")

// envVarRegexp matches VAULT_KEK_V<digits>. The version part must be a base-10
// integer ≥ 1. Anchored ^$ to avoid matching VAULT_KEK_V1_FOO.
var envVarRegexp = regexp.MustCompile(`^` + regexp.QuoteMeta(EnvVarPrefix) + `(\d+)$`)

// KeyRing holds the configured KEK versions for the lifetime of the process.
//
// Reads accept ANY configured version; writes use only the current version.
// The current version is the highest numbered version unless overridden by
// VAULT_CURRENT_KEK_VERSION.
//
// KeyRing values are immutable after Load. To rotate, restart the process
// with new env vars — this is intentional to keep the in-memory key set
// consistent across goroutines without locking.
type KeyRing struct {
	// keys maps version → 32-byte KEK material. Never logged, never serialised.
	keys map[int][]byte
	// versions is a sorted (ascending) slice of all known version numbers.
	versions []int
	// current is the version used for new envelope writes.
	current int
}

// Load reads KEK material from env vars and the file fallback directory and
// returns a KeyRing with at least one version, or an error explaining why
// startup must abort.
//
// Sources (merged; env wins on duplicate version):
//
//  1. Env: every VAULT_KEK_V<N> where the value is a valid 64-hex-char string.
//  2. Files: every {dir}/v<N>.key whose contents trim to a valid hex KEK.
//     `dir` is VAULT_KEK_DIR or DefaultDir.
//
// The selected "current" version is VAULT_CURRENT_KEK_VERSION if set AND
// present in the merged set; otherwise it is the highest known version.
func Load() (*KeyRing, error) {
	merged := make(map[int][]byte)

	if err := loadFromEnv(merged); err != nil {
		return nil, err
	}
	if err := loadFromDir(merged, kekDir()); err != nil {
		// Directory-source failures are non-fatal only when env already supplied
		// at least one key. Otherwise startup must fail loudly.
		if len(merged) == 0 {
			return nil, err
		}
	}

	if len(merged) == 0 {
		return nil, ErrNoKEKsConfigured
	}

	versions := make([]int, 0, len(merged))
	for v := range merged {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	current, err := pickCurrent(versions, os.Getenv(CurrentVersionEnv))
	if err != nil {
		return nil, err
	}

	return &KeyRing{keys: merged, versions: versions, current: current}, nil
}

// Current returns the KEK version used for new envelope writes.
func (kr *KeyRing) Current() int { return kr.current }

// Versions returns all configured KEK versions in ascending order.
// The returned slice is a copy — safe to mutate.
func (kr *KeyRing) Versions() []int {
	out := make([]int, len(kr.versions))
	copy(out, kr.versions)
	return out
}

// Get returns the KEK material for the given version, or ErrKEKVersionNotFound.
//
// Callers MUST NOT retain the returned slice beyond the immediate
// decrypt/encrypt operation — the KeyRing owns the underlying memory and may
// zero it in future versions when memguard support lands (V05-F1, deferred S03).
func (kr *KeyRing) Get(version int) ([]byte, error) {
	k, ok := kr.keys[version]
	if !ok {
		return nil, fmt.Errorf("%w: V%d", ErrKEKVersionNotFound, version)
	}
	return k, nil
}

// CurrentKey returns the KEK material for the version used by new envelope
// writes. Equivalent to kr.Get(kr.Current()).
func (kr *KeyRing) CurrentKey() ([]byte, error) {
	return kr.Get(kr.current)
}

// Has reports whether the given version is configured.
func (kr *KeyRing) Has(version int) bool {
	_, ok := kr.keys[version]
	return ok
}

// ---------- private helpers ----------

// kekDir returns the file-fallback directory: VAULT_KEK_DIR if set, else
// DefaultDir.
func kekDir() string {
	if d := os.Getenv(DirEnv); d != "" {
		return d
	}
	return DefaultDir
}

// loadFromEnv scans os.Environ for VAULT_KEK_V<N> assignments and merges any
// validly-formatted KEKs into dst. Invalid hex returns an error so a typo in
// production env fails startup loudly instead of silently dropping the key.
func loadFromEnv(dst map[int][]byte) error {
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		key, val := kv[:eq], kv[eq+1:]
		m := envVarRegexp.FindStringSubmatch(key)
		if m == nil {
			continue
		}
		if val == "" {
			// An empty VAULT_KEK_V2= is treated as "not set" — useful for
			// templating systems that leave placeholders blank.
			continue
		}
		version, err := strconv.Atoi(m[1])
		if err != nil || version < 1 {
			return fmt.Errorf("vault/crypto: invalid KEK version in env var %s", key)
		}
		buf, err := decodeKEKHex(val)
		if err != nil {
			return fmt.Errorf("vault/crypto: %s: %w", key, err)
		}
		dst[version] = buf
	}
	return nil
}

// loadFromDir scans `dir` for files named v<N>.key and merges valid KEKs into
// dst. A missing directory is NOT an error (the env path may have supplied
// everything). Per-file decode failures ARE errors.
func loadFromDir(dst map[int][]byte, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("vault/crypto: read kek dir %q: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "v") || !strings.HasSuffix(name, ".key") {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimPrefix(name, "v"), ".key")
		version, err := strconv.Atoi(stem)
		if err != nil || version < 1 {
			// Ignore non-conforming filenames so an admin can stage unrelated
			// files in the directory without crashing startup.
			continue
		}
		if _, already := dst[version]; already {
			// Env source wins on conflict (see Load() doc).
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("vault/crypto: read %q: %w", name, err)
		}
		// Files may carry trailing newline / surrounding whitespace.
		trimmed := strings.TrimSpace(string(raw))
		// Files may also store `VAULT_KEK_V<N>=<hex>` form — accept both shapes.
		if eq := strings.IndexByte(trimmed, '='); eq >= 0 {
			trimmed = trimmed[eq+1:]
		}
		buf, err := decodeKEKHex(trimmed)
		if err != nil {
			return fmt.Errorf("vault/crypto: %s: %w", name, err)
		}
		dst[version] = buf
	}
	return nil
}

// decodeKEKHex validates and decodes a hex-encoded KEK to a 32-byte slice.
func decodeKEKHex(s string) ([]byte, error) {
	if len(s) != 2*KeySize {
		return nil, ErrInvalidKEKHex
	}
	buf, err := hex.DecodeString(s)
	if err != nil {
		return nil, ErrInvalidKEKHex
	}
	if len(buf) != KeySize {
		return nil, ErrInvalidKEKHex
	}
	return buf, nil
}

// pickCurrent resolves the active KEK version for new writes.
//
// When override is empty, return the highest configured version (versions are
// sorted ascending). When override is non-empty, parse and validate it.
func pickCurrent(versions []int, override string) (int, error) {
	if len(versions) == 0 {
		return 0, ErrNoKEKsConfigured
	}
	if override == "" {
		return versions[len(versions)-1], nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(override))
	if err != nil || v < 1 {
		return 0, fmt.Errorf("vault/crypto: %s must be a positive integer (got %q)", CurrentVersionEnv, override)
	}
	for _, kv := range versions {
		if kv == v {
			return v, nil
		}
	}
	return 0, fmt.Errorf("%w: %s=V%d but no matching env/file key", ErrKEKVersionNotFound, CurrentVersionEnv, v)
}
