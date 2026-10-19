package checkout

// Purpose: the coordinator half: pack one exact revision into a git bundle and
// compute its digest.
// Inputs:  the repository root, a full commit id, a destination file.
// Outputs: "sha256:<hex>" and the byte size of the bundle.
// Constraints: git needs a ref to bundle, so the revision is pinned under a
// private ref (refs/nself-ci/bundle-<random>) for the duration of the call and
// the ref is deleted afterwards, even on failure.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var fullRev = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// ValidRevision reports whether rev is a full 40 or 64 hex commit id.
func ValidRevision(rev string) bool { return fullRev.MatchString(rev) }

// Create bundles revision of the repository at repo into dest (mode 0600).
func Create(ctx context.Context, run Runner, repo, revision, dest string) (digest string, size int64, err error) {
	if run == nil {
		run = DefaultRunner
	}
	if !ValidRevision(revision) {
		return "", 0, fmt.Errorf("checkout: revision must be a full 40 or 64 hex commit id")
	}
	if !filepath.IsAbs(repo) || !filepath.IsAbs(dest) {
		return "", 0, fmt.Errorf("checkout: repository and bundle paths must be absolute")
	}
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", 0, err
	}
	ref := "refs/nself-ci/bundle-" + hex.EncodeToString(nonce[:])
	if _, err := git(ctx, run, "-C", repo, "update-ref", ref, revision+"^{commit}"); err != nil {
		return "", 0, fmt.Errorf("checkout: pin revision: %w", err)
	}
	defer func() { _, _ = git(context.WithoutCancel(ctx), run, "-C", repo, "update-ref", "-d", ref) }()
	if _, err := git(ctx, run, "-C", repo, "bundle", "create", dest, ref); err != nil {
		_ = os.Remove(dest)
		return "", 0, fmt.Errorf("checkout: create bundle: %w", err)
	}
	if err := os.Chmod(dest, 0o600); err != nil {
		return "", 0, err
	}
	f, err := os.Open(dest)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), n, nil
}

// RefPrefix is the private ref namespace the bundle carries.
const RefPrefix = "refs/nself-ci/"
