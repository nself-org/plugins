package publish

// Purpose: Export, `nself ci artifacts release <run> --out <dir>` (Epic CACHE
// D15): copy the published release directory into a fresh empty directory
// that authz.ArtifactGetDest accepts, and re-verify the copy.
// Inputs:  Deps, a run id and the destination.
// Outputs: Release with Dir set to the export, and the privacy notice text.
// Constraints: only a published release exports (a failed or awaiting one
// names its recovery command); the source is verified before and the copy
// after; files are created exclusively with owner-only mode; the export holds
// evidence.json (raw evidence with runner ids): private operator material,
// which a public project does not publish (D15).

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// PrivateNotice is printed after an export.
const PrivateNotice = "The directory holds evidence.json with runner ids: private operator material. A public project publishes the artifacts, SBOMs, provenance and manifest, not evidence.json."

// Export copies the release of runID into out (a fresh empty directory or a new one).
func Export(ctx context.Context, d Deps, runID, out, cwd string) (Release, error) {
	if !validRunID(runID) {
		return Release{}, refuse(signing.ReasonPayloadRejected, "run id is not a plain name")
	}
	row, ok, err := d.Ledger.ReleaseFor(ctx, runID)
	switch {
	case err != nil:
		return Release{}, refuse(signing.ReasonSignerFailed, "the release ledger could not be read")
	case !ok:
		return Release{}, refuse(signing.ReasonPayloadRejected, "run %s has no release record: it has no passing release job, or was not published; run `%s`", runID, RecoveryCommand(runID))
	case row.State == store.ReleaseAwaiting:
		return Release{}, refuse(signing.ReasonSignerFailed, "release %s awaits a signature: run `%s`", runID, TeamCommand(runID))
	case row.State != store.ReleasePublished:
		return Release{}, refuse(signing.ReasonSignerFailed, "release %s failed to publish: run `%s`", runID, RecoveryCommand(runID))
	}
	dest, err := authz.ArtifactGetDest(out, cwd)
	if err != nil {
		return Release{}, err
	}
	src := Dir(d.Home, runID)
	if err = d.checkDir(ctx, src, row); err != nil {
		return Release{}, withRecovery(err, runID)
	}
	undo, done, err := copyTree(src, dest)
	if err != nil {
		return Release{}, refuse(signing.ReasonSignerFailed, "the release could not be exported")
	}
	defer done()
	if err = d.checkDir(ctx, dest, row); err != nil {
		undo() // nothing unverified stays behind
		return Release{}, err
	}
	return Release{RunID: runID, State: Published, Dir: dest, ManifestSHA256: row.ManifestSHA256, KeyID: row.KeyID, Message: PrivateNotice}, nil
}

// checkDir verifies dir and requires its manifest to be the one the ledger recorded.
func (d Deps) checkDir(ctx context.Context, dir string, row store.ReleaseRow) error {
	b, err := os.ReadFile(filepath.Join(dir, verifyManifestFile))
	if err != nil || sha(b) != row.ManifestSHA256 {
		return refuse(signing.ReasonDigestNotCommitted, "the release directory is not the release the ledger recorded")
	}
	return d.selfVerify(ctx, dir, row.KeyID, false)
}

func withRecovery(err error, runID string) error {
	e, ok := err.(*Error)
	if !ok {
		return err
	}
	out := *e
	switch e.Reason {
	case "signature", "revoked", "key_purpose": // a rebuild signs with today's key, and a published record never changes: trust is the question
		out.Msg += ": the release key may have been rotated or revoked; trust lines are read from <home>/trust/ci-release.keys and .revoked"
	default:
		out.Msg += ": run `" + RecoveryCommand(runID) + "` to rebuild it"
	}
	return &out
}

// copyTree copies the regular files of src into dst. dst is created with an
// exclusive mkdir, or must already be an empty real directory (never a
// symlink); every write goes through an os.Root on dst, so a name planted in
// dst while the copy runs cannot redirect a write outside it. A failed copy
// removes what it created. undo removes through the handle opened here, never
// the pathname again, so a dst swapped for a symlink afterwards cannot make it
// delete files elsewhere; the caller must call done when it no longer needs undo.
//
// Residual: Mkdir and OpenRoot are two steps, and opening dst with
// O_DIRECTORY|O_NOFOLLOW before the Mkdir is not portable. A dst swapped
// between them fails the identity check below with an empty directory left
// behind; nothing outside dst is read or written (D-1248).
func copyTree(src, dst string) (undo, done func(), err error) {
	madeDst := false
	if info, lerr := os.Lstat(dst); lerr == nil {
		if !info.IsDir() {
			return nil, nil, os.ErrInvalid // a symlink or a file
		}
	} else if err = os.Mkdir(dst, 0o700); err != nil {
		return nil, nil, err
	} else {
		madeDst = true
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		return nil, nil, err
	}
	var ident os.FileInfo
	if before, lerr := os.Lstat(dst); lerr != nil || !before.IsDir() {
		_ = root.Close()
		return nil, nil, os.ErrInvalid
	} else if at, serr := root.Stat("."); serr != nil || !os.SameFile(before, at) {
		_ = root.Close()
		return nil, nil, os.ErrInvalid
	} else {
		ident = at
	}
	var made []string // root-relative names, in creation order
	done = func() { _ = root.Close() }
	undo = func() {
		for i := len(made) - 1; i >= 0; i-- {
			_ = root.Remove(made[i])
		}
		if now, lerr := os.Lstat(dst); madeDst && lerr == nil && os.SameFile(ident, now) {
			_ = os.Remove(dst) // only the directory made here, and only while empty
		}
	}
	err = filepath.WalkDir(src, func(p string, de os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil || rel == "." {
			return rerr
		}
		if de.IsDir() {
			if err := root.Mkdir(rel, 0o700); err != nil { // exclusive: an existing name, a planted symlink included, fails
				return err
			}
			made = append(made, rel)
			return nil
		}
		if !de.Type().IsRegular() {
			return os.ErrInvalid
		}
		return copyFile(root, p, rel, &made)
	})
	if err != nil {
		undo()
		done()
		return nil, nil, err
	}
	return undo, done, nil
}

func copyFile(root *os.Root, srcPath, rel string, made *[]string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	*made = append(*made, rel)
	_, err = io.Copy(out, in)
	if e := out.Close(); err == nil {
		err = e
	}
	return err
}
