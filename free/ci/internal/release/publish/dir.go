package publish

// Purpose: write a release directory and move it into place.
// Inputs:  a *bundle, the blob store and a staging parent directory.
// Outputs: a complete directory under <home>/releases/<run>.
// Constraints: files are written into a private staging directory next to the
// target and renamed in one step, so a killed process leaves no half-written
// release; every file is created exclusively with mode 0600 (directories 0700):
// the evidence is private operator material; artifact bytes are re-hashed while
// they are copied and must equal the committed record; a path never leaves the
// release directory.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

// releasesDir is the parent of every published release.
func releasesDir(home string) string { return filepath.Join(home, "releases") }

// Dir is the published directory of a run.
func Dir(home, runID string) string { return filepath.Join(releasesDir(home), runID) }

// validRunID keeps a run id a single plain path element.
func validRunID(id string) bool {
	return id != "" && id != "." && id != ".." && len(id) <= 200 && !strings.ContainsAny(id, "/\\\x00")
}

// stage creates a private staging directory beside the target.
func stage(home string) (string, error) {
	parent := releasesDir(home)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", refuse(signing.ReasonSignerFailed, "the releases directory could not be created")
	}
	sweep(home)
	d, err := os.MkdirTemp(parent, ".stage-")
	if err != nil {
		return "", refuse(signing.ReasonSignerFailed, "a staging directory could not be created")
	}
	return d, nil
}

// writeBundle fills dir with every file of the bundle.
func writeBundle(ctx context.Context, d Deps, project string, b *bundle, dir string) error {
	for _, f := range b.files {
		dst := filepath.Join(dir, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return refuse(signing.ReasonSignerFailed, "a release directory could not be created")
		}
		var err error
		if f.data != nil {
			err = writeNew(dst, f.data)
		} else {
			err = copyBlob(ctx, d, project, f, dst)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func writeNew(path string, data []byte) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "a release file could not be created")
	}
	_, werr := out.Write(data)
	if cerr := out.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return refuse(signing.ReasonSignerFailed, "a release file could not be written")
	}
	return nil
}

// copyBlob copies the stored bytes of an artifact or SBOM record, re-hashing them.
func copyBlob(ctx context.Context, d Deps, project string, f file, dst string) error {
	rec := f.rec.rec
	switch f.kind {
	case "cdx":
		rec = f.rec.cdx
	case "spdx":
		rec = f.rec.spdx
	}
	rc, err := d.Blobs.Open(cas.ArtifactRealm(project), rec.SHA256)
	if err != nil {
		return refuse(signing.ReasonDigestNotCommitted, "the stored bytes of %q are gone", f.path)
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "a release file could not be created")
	}
	h := sha256.New()
	n, cerr := io.Copy(io.MultiWriter(out, h), io.LimitReader(&ctxReader{ctx, rc}, rec.Size+1))
	if e := out.Close(); cerr == nil {
		cerr = e
	}
	if cerr != nil {
		return refuse(signing.ReasonSignerFailed, "a release file could not be written")
	}
	if n != rec.Size || hex.EncodeToString(h.Sum(nil)) != rec.SHA256 {
		return refuse(signing.ReasonDigestNotCommitted, "the stored bytes of %q do not verify", f.path)
	}
	return nil
}

// ctxReader stops a copy when the context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// installDir moves the staged directory to the target. commit drops the
// directory it replaced; undo puts that directory back (the ledger write failed).
func installDir(staged, target string) (commit, undo func(), err error) {
	old := ""
	if _, serr := os.Lstat(target); serr == nil {
		old = target + ".old-" + filepath.Base(staged)
		if err = os.Rename(target, old); err != nil {
			return nil, nil, err
		}
		// a rename keeps the old modification time: without this a backup of an old
		// release would look abandoned to another run's sweep while it is live
		now := time.Now()
		_ = os.Chtimes(old, now, now)
	}
	if err = os.Rename(staged, target); err != nil {
		if old != "" {
			_ = os.Rename(old, target)
		}
		return nil, nil, err
	}
	commit = func() {
		if old != "" {
			_ = os.RemoveAll(old)
		}
	}
	undo = func() {
		_ = os.RemoveAll(target)
		if old != "" {
			_ = os.Rename(old, target)
		}
	}
	return commit, undo, nil
}

// backupRe names the rollback backup of a replaced release: <run>.old-<staging name>.
var backupRe = regexp.MustCompile(`^[^.][^/]*\.old-\.stage-[^/]+$`)

// staleAfter is how long an abandoned staging entry is kept.
const staleAfter = time.Hour

// sweep removes staging leftovers of killed publications.
func sweep(home string) {
	ents, err := os.ReadDir(releasesDir(home))
	if err != nil {
		return
	}
	for _, e := range ents {
		n := e.Name()
		if !strings.HasPrefix(n, ".stage-") && !strings.HasPrefix(n, ".keys-") && !backupRe.MatchString(n) {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > staleAfter {
			_ = os.RemoveAll(filepath.Join(releasesDir(home), n))
		}
	}
}
