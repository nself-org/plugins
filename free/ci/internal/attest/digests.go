package attest

// Purpose: bind evidence to the inputs it was produced from.
// Inputs:  the effective config (any JSON-shaped value) and the lockfiles and
// declared generated files the caller read (path and content).
// Outputs: model.InputDigests: sha256 hex of the canonical config and of each
// file, files sorted by path.
// Constraints: pure (the caller reads files); a float anywhere in the config
// is an error because trust.Canonical refuses it; paths are repo-relative,
// slash-separated and clean, and a path appears once per list.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// File is one bound input: a repo-relative path and the bytes read.
type File struct {
	Path    string
	Content []byte
}

// Digest returns the lowercase sha256 hex of b.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ConfigDigest is sha256(trust.Canonical(config)).
func ConfigDigest(config any) (string, error) {
	if config == nil {
		return "", errors.New("attest: effective config is required")
	}
	body, err := trust.Canonical(config)
	if err != nil {
		return "", fmt.Errorf("attest: config digest: %w", err)
	}
	return Digest(body), nil
}

// InputDigests digests the config, the lockfiles and the generated inputs.
func InputDigests(effectiveConfig any, lockfiles, generated []File) (model.InputDigests, error) {
	cfg, err := ConfigDigest(effectiveConfig)
	if err != nil {
		return model.InputDigests{}, err
	}
	locks, err := fileDigests("lockfile", lockfiles)
	if err != nil {
		return model.InputDigests{}, err
	}
	gen, err := fileDigests("generated", generated)
	if err != nil {
		return model.InputDigests{}, err
	}
	return model.InputDigests{Config: cfg, Lockfiles: locks, Generated: gen}, nil
}

func fileDigests(kind string, files []File) ([]model.DigestRef, error) {
	out := make([]model.DigestRef, 0, len(files))
	seen := map[string]bool{}
	for _, f := range files {
		if err := checkPath(f.Path); err != nil {
			return nil, fmt.Errorf("attest: %s %q: %w", kind, f.Path, err)
		}
		folded := strings.ToLower(f.Path) // a case-insensitive checkout cannot hold both
		if seen[folded] {
			return nil, fmt.Errorf("attest: %s %q listed twice", kind, f.Path)
		}
		seen[folded] = true
		out = append(out, model.DigestRef{Path: f.Path, SHA256: Digest(f.Content)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func checkPath(p string) error {
	switch {
	case p == "":
		return errors.New("empty path")
	case !utf8.ValidString(p) || strings.ContainsRune(p, 0) || strings.ContainsRune(p, '\\'):
		return errors.New("path has invalid characters")
	case strings.HasPrefix(p, "/") || path.Clean(p) != p || p == "." || p == ".." || strings.HasPrefix(p, "../"):
		return errors.New("path must be repo-relative and clean")
	}
	return nil
}
