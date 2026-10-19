package records

// Purpose: list, show and fetch records for a viewer (Epic CACHE D6): what
// `nself ci artifacts list|show|get` call.
// Inputs:  a Viewer (project and role), ids, a destination and the caller's cwd.
// Outputs: visible Records, or the one E713 error; Get returns the file written.
// Constraints: "not found" and "not visible" are the same error value; a
// record whose blob is gone is reported missing (and remembered) and Get
// refuses it (E712 missing_blob); Get writes only inside a directory that
// authz.ArtifactGetDest accepted, creates every file exclusively and
// re-verifies size and digest of what it wrote.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

// Reader reads records on behalf of a viewer.
type Reader struct {
	Ledger Ledger
	Blobs  Blobs
}

// List returns the committed records the viewer may see, newest first.
func (r Reader) List(ctx context.Context, v authz.Viewer, f Filter) ([]Record, error) {
	f.Project = v.Project
	all, err := r.Ledger.ArtifactList(ctx, f)
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, rec := range all {
		if rec.State == StateCommitted && authz.ArtifactVisibility(rec.Meta(), v) {
			out = append(out, r.checkBlob(ctx, rec))
		}
	}
	return out, nil
}

// Show returns one record, with Missing set when its blob is gone.
func (r Reader) Show(ctx context.Context, v authz.Viewer, id string) (Record, error) {
	rec, err := r.visible(ctx, v, id)
	if err != nil {
		return Record{}, err
	}
	return r.checkBlob(ctx, rec), nil
}

func (r Reader) visible(ctx context.Context, v authz.Viewer, id string) (Record, error) {
	rec, ok, err := r.Ledger.ArtifactGet(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if !ok || rec.State != StateCommitted || !authz.ArtifactVisibility(rec.Meta(), v) {
		return Record{}, ErrNotFound
	}
	return rec, nil
}

// checkBlob marks a record missing when its blob is no longer in the store.
func (r Reader) checkBlob(ctx context.Context, rec Record) Record {
	if _, err := r.Blobs.Stat(cas.ArtifactRealm(rec.Project), rec.SHA256); err != nil {
		if !rec.Missing {
			_ = r.Ledger.ArtifactMarkMissing(ctx, rec.ID)
		}
		rec.Missing = true
	}
	return rec
}

// Get writes the record's bytes under dest/<name> and returns the path.
func (r Reader) Get(ctx context.Context, v authz.Viewer, id, dest, cwd string) (string, Record, error) {
	rec, err := r.visible(ctx, v, id)
	if err != nil {
		return "", Record{}, err
	}
	dir, err := authz.ArtifactGetDest(dest, cwd)
	if err != nil {
		return "", rec, err
	}
	rel, err := authz.ArtifactPath(authz.Entry{Name: rec.Name, Mode: 0}, authz.Limits{})
	if err != nil {
		return "", rec, err
	}
	if rec = r.checkBlob(ctx, rec); rec.Missing {
		return "", rec, failf(ReasonMissingBlob, "%s: the stored bytes are gone (restore the blobs or rerun the job)", rec.Name)
	}
	if err := verifyBlob(r.Blobs, rec); err != nil {
		return "", rec, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", rec, failf(ReasonUnreadable, "destination: %v", err)
	}
	// re-check after the create: the directory must still be empty and ours
	if dir, err = authz.ArtifactGetDest(dir, cwd); err != nil {
		return "", rec, err
	}
	return r.write(dir, rel, rec)
}

func (r Reader) write(dir, rel string, rec Record) (string, Record, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", rec, failf(ReasonUnreadable, "destination: %v", err)
	}
	defer func() { _ = root.Close() }()
	if d := filepath.Dir(rel); d != "." {
		if err := root.MkdirAll(d, 0o700); err != nil {
			return "", rec, failf(ReasonUnreadable, "destination: %v", err)
		}
	}
	src, err := r.Blobs.Open(cas.ArtifactRealm(rec.Project), rec.SHA256)
	if err != nil {
		return "", rec, failf(ReasonDigest, "%s: the stored bytes do not verify: %v", rec.Name, err)
	}
	defer func() { _ = src.Close() }()
	out, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", rec, failf(ReasonUnreadable, "destination: %v", err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil || n != rec.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), rec.SHA256) {
		_ = root.Remove(rel)
		return "", rec, failf(ReasonDigest, "%s: written bytes do not match the recorded digest (%v)", rec.Name, err)
	}
	return filepath.Join(dir, rel), rec, nil
}
