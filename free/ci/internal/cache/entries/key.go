package entries

// Purpose: the cache key (Epic D3, R-SEC-30): sha256 over length-prefixed fields.
// Inputs: KeyInputs. Outputs: Key (64 lowercase hex), NamespaceOf.
// Constraints: pure and deterministic. Every scalar is a 4-byte big-endian
// length plus its bytes; every list is a 4-byte count, then each item the same
// way, sorted bytewise, so ("ab","c") and ("a","bc") never collide in any
// position. The namespace field is authz.Namespace.Canonical(). There is no
// prefix or restore-key fallback anywhere in this package.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	maxFieldBytes = 1 << 20
	maxListItems  = 1 << 16
	maxKeyBytes   = 16 << 20 // all fields together
)

// KeyInputs are the key fields in encoding order. Toolchain is the image digest
// for container levels (one item) or the sorted name=version probes for process
// level; EnvDigest is the sha256 of the spec's allowlisted env (values are never
// stored); Lockfiles items are path=sha256.
type KeyInputs struct {
	Namespace   authz.Namespace
	SpecID      string
	SpecVersion string
	Platform    string
	Isolation   string
	Toolchain   []string
	EnvDigest   string
	Paths       []string
	Lockfiles   []string
	Extras      []string
}

func putField(buf []byte, b []byte) ([]byte, error) {
	if len(b) > maxFieldBytes {
		return nil, fmt.Errorf("entries: key field of %d bytes exceeds %d", len(b), maxFieldBytes)
	}
	if len(buf)+4+len(b) > maxKeyBytes { // checked before every append, so inputs are never expanded past the limit
		return nil, fmt.Errorf("entries: key inputs exceed %d bytes", maxKeyBytes)
	}
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(b)))
	return append(buf, b...), nil
}

func putList(buf []byte, items []string) ([]byte, error) {
	if len(items) > maxListItems {
		return nil, fmt.Errorf("entries: key list of %d items exceeds %d", len(items), maxListItems)
	}
	if len(buf)+4 > maxKeyBytes {
		return nil, fmt.Errorf("entries: key inputs exceed %d bytes", maxKeyBytes)
	}
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(sorted)))
	var err error
	for _, it := range sorted {
		if buf, err = putField(buf, []byte(it)); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// encode returns the bytes Key hashes.
func (k KeyInputs) encode() ([]byte, error) {
	if !k.Namespace.Valid() {
		return nil, errors.New("entries: invalid namespace")
	}
	var buf []byte
	var err error
	for _, f := range [][]byte{k.Namespace.Canonical(), []byte(k.SpecID), []byte(k.SpecVersion), []byte(k.Platform), []byte(k.Isolation)} {
		if buf, err = putField(buf, f); err != nil {
			return nil, err
		}
	}
	if buf, err = putList(buf, k.Toolchain); err != nil {
		return nil, err
	}
	if buf, err = putField(buf, []byte(k.EnvDigest)); err != nil {
		return nil, err
	}
	for _, l := range [][]string{k.Paths, k.Lockfiles, k.Extras} {
		if buf, err = putList(buf, l); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// Key derives the cache key.
func Key(k KeyInputs) (string, error) {
	b, err := k.encode()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// NamespaceOf derives a namespace from recorded facts, only through
// authz.CacheNamespace.
func NamespaceOf(f Facts) (authz.Namespace, error) {
	return authz.CacheNamespace(f.Project, model.TrustClass(f.Trust), authz.Protection(f.Protection), f.HeadRepo, f.PR)
}
