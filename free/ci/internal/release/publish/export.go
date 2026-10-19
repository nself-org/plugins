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
	if err = d.selfVerify(ctx, src, row.KeyID); err != nil {
		return Release{}, withRecovery(err, runID)
	}
	if err = copyTree(src, dest); err != nil {
		return Release{}, refuse(signing.ReasonSignerFailed, "the release could not be exported")
	}
	if err = d.selfVerify(ctx, dest, row.KeyID); err != nil {
		return Release{}, err
	}
	return Release{RunID: runID, State: Published, Dir: dest, ManifestSHA256: row.ManifestSHA256, KeyID: row.KeyID, Message: PrivateNotice}, nil
}

func withRecovery(err error, runID string) error {
	e, ok := err.(*Error)
	if !ok {
		return err
	}
	out := *e
	out.Msg += ": run `" + RecoveryCommand(runID) + "` to rebuild it"
	return &out
}

// copyTree copies the regular files of src into dst, creating dst if needed.
func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, de os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return err
		}
		if de.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o700)
		}
		if !de.Type().IsRegular() {
			return os.ErrInvalid
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(filepath.Join(dst, rel), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		if e := out.Close(); err == nil {
			err = e
		}
		return err
	})
}
