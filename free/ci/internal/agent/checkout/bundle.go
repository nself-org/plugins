package checkout

// Purpose: the coordinator half: pack one exact revision into a git bundle and
// compute its digest.
// Inputs:  the repository root, a full commit id, a destination file.
// Outputs: "sha256:<hex>" and the byte size of the bundle.
// Constraints: git needs a ref to bundle, so the revision is pinned under a
// private ref (refs/nself-ci/bundle-<random>) for the duration of the call and
// the ref is deleted afterwards, even on failure. A SIGKILL skips that delete,
// so each Create first sweeps pins older than staleRefAge (D-1058).

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
	"strconv"
	"strings"
	"time"
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
	sweepStaleRefs(ctx, run, repo)
	ref := bundleRefPrefix + strconv.FormatInt(now().Unix(), 10) + "-" + hex.EncodeToString(nonce[:])
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

// bundleRefPrefix names the pins Create makes: bundleRefPrefix<unix seconds>-<hex nonce>.
const bundleRefPrefix = RefPrefix + "bundle-"

// staleRefAge is how old a pin must be before the next Create treats it as the leftover of a
// killed coordinator. It exceeds gitTimeout, so a pin of a live concurrent Create is never swept.
const staleRefAge = gitTimeout + time.Hour

var now = time.Now

// sweepStaleRefs deletes bundle pins a SIGKILLed coordinator left behind (the deferred delete in
// Create never ran). A pin without a parsable timestamp is from the older naming and is stale.
// Best effort: a failure here never fails the bundle.
func sweepStaleRefs(ctx context.Context, run Runner, repo string) {
	out, err := git(ctx, run, "-C", repo, "for-each-ref", "--format=%(refname)", RefPrefix)
	if err != nil {
		return
	}
	for _, ref := range strings.Fields(out) {
		if !strings.HasPrefix(ref, bundleRefPrefix) || !refIsStale(ref) {
			continue
		}
		_, _ = git(ctx, run, "-C", repo, "update-ref", "-d", ref)
	}
}

func refIsStale(ref string) bool {
	secs, _, ok := strings.Cut(strings.TrimPrefix(ref, bundleRefPrefix), "-")
	ts, err := strconv.ParseInt(secs, 10, 64)
	if !ok || err != nil {
		return true
	}
	return now().Sub(time.Unix(ts, 0)) > staleRefAge
}
