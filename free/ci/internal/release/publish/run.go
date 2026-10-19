package publish

// Purpose: Run, the publication of one finished release run (Epic CACHE D15):
// read the stored facts, build and sign the files, self-verify them against
// the signer's public key, install the directory and record `published`.
// Inputs:  Deps and a run id.
// Outputs: Release; ErrNotRelease for a run with nothing to publish (nothing
// recorded); an *Error otherwise, after the failure was recorded.
// Constraints: the evidence is never touched; a failure records `failed` with
// the reason (never a path or a payload) and its message names
// `nself ci artifacts sign <run>`, which repeats these steps and, because
// every byte is deterministic, ends with byte-identical files; a second call
// for a published, intact release changes nothing and signs nothing; a
// published record is never replaced by different bytes; on a TEAM coordinator
// the release is `awaiting-signature`, the signer is not called and no secret
// is resolved.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const verifyManifestFile = verify.ManifestFile

// releaseKey is the idempotency key of a release attempt (the one its job received).
var releaseKey = job.IdempotencyKey

// Run publishes the release of runID.
func Run(ctx context.Context, d Deps, runID string) (Release, error) {
	if !validRunID(runID) {
		return Release{}, refuse(signing.ReasonPayloadRejected, "run id is not a plain name")
	}
	f, err := gather(ctx, d, runID)
	if err != nil {
		if errors.Is(err, ErrNotRelease) {
			return Release{RunID: runID}, err
		}
		return Release{RunID: runID}, d.fail(ctx, runID, nil, err)
	}
	if d.Team {
		return d.await(ctx, runID, f)
	}
	if rel, ok := d.existing(ctx, runID, f); ok {
		return rel, nil
	}
	b, err := build(ctx, d, runID, f)
	if err != nil {
		return Release{RunID: runID}, d.fail(ctx, runID, f, err)
	}
	rel, err := d.install(ctx, runID, f, b)
	if err != nil {
		return Release{RunID: runID}, d.fail(ctx, runID, f, err)
	}
	return rel, nil
}

// install writes, verifies and records the bundle.
func (d Deps) install(ctx context.Context, runID string, f *facts, b *bundle) (Release, error) {
	prior, had, err := d.Ledger.ReleaseFor(ctx, runID)
	if err != nil {
		return Release{}, refuse(signing.ReasonSignerFailed, "the release ledger could not be read")
	}
	if had && prior.State == store.ReleasePublished && prior.ManifestSHA256 != b.manifestSHA {
		return Release{}, refuse(signing.ReasonSignerFailed, "the rebuilt release differs from the published one: keep the published directory")
	}
	staged, err := stage(d.Home)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = os.RemoveAll(staged) }()
	if err = writeBundle(ctx, d, f.pipe.Project, b, staged); err != nil {
		return Release{}, err
	}
	if err = d.selfVerify(ctx, staged, b.keyID); err != nil {
		return Release{}, err
	}
	target := Dir(d.Home, runID)
	if err = install(staged, target, true); err != nil {
		return Release{}, refuse(signing.ReasonSignerFailed, "the release directory could not be put in place")
	}
	row := store.ReleaseRow{RunID: runID, Project: f.pipe.Project, Revision: f.pipe.Revision, State: store.ReleasePublished, ManifestSHA256: b.manifestSHA, KeyID: b.keyID}
	if err = d.Ledger.RecordRelease(ctx, row); err != nil {
		return Release{}, refuse(signing.ReasonSignerFailed, "the release could not be recorded")
	}
	return Release{RunID: runID, State: Published, Dir: target, ManifestSHA256: b.manifestSHA, KeyID: b.keyID}, nil
}

// existing returns the published release when its directory is intact and
// still verifies (no signer call needed to prove it).
func (d Deps) existing(ctx context.Context, runID string, f *facts) (Release, bool) {
	row, ok, err := d.Ledger.ReleaseFor(ctx, runID)
	if err != nil || !ok || row.State != store.ReleasePublished || row.Project != f.pipe.Project {
		return Release{}, false
	}
	dir := Dir(d.Home, runID)
	b, err := os.ReadFile(filepath.Join(dir, verifyManifestFile))
	if err != nil || sha(b) != row.ManifestSHA256 || d.selfVerify(ctx, dir, row.KeyID) != nil {
		return Release{}, false
	}
	return Release{RunID: runID, State: Published, Dir: dir, ManifestSHA256: row.ManifestSHA256, KeyID: row.KeyID}, true
}

// await records the TEAM state: the coordinator never signs (Epic X8).
func (d Deps) await(ctx context.Context, runID string, f *facts) (Release, error) {
	msg := "this TEAM coordinator holds no release key: run `" + TeamCommand(runID) + "` on your own machine"
	row := store.ReleaseRow{RunID: runID, Project: f.pipe.Project, Revision: f.pipe.Revision, State: store.ReleaseAwaiting, Reason: msg}
	if err := d.Ledger.RecordRelease(ctx, row); err != nil {
		return Release{RunID: runID}, refuse(signing.ReasonSignerFailed, "the release could not be recorded")
	}
	return Release{RunID: runID, State: Awaiting, Message: msg}, nil
}

// fail records `failed` (when the run is known) and returns the error with the recovery command.
func (d Deps) fail(ctx context.Context, runID string, f *facts, err error) error {
	var e *Error
	if !errors.As(err, &e) {
		e = refuse(signing.ReasonSignerFailed, "%v", err).(*Error)
	}
	if f == nil {
		if pipe, ok, perr := d.Src.PipelineByID(ctx, runID); perr == nil && ok {
			f = &facts{pipe: pipe}
		}
	}
	if f != nil {
		_ = d.Ledger.RecordRelease(ctx, store.ReleaseRow{RunID: runID, Project: f.pipe.Project, Revision: f.pipe.Revision, State: store.ReleaseFailed, Reason: failedRecord(e)})
	}
	out := *e
	out.Msg += ": release NOT published; the run's evidence is unchanged; run `" + RecoveryCommand(runID) + "` to publish it"
	return &out
}

// selfVerify checks dir with internal/release/verify against the signer's key.
func (d Deps) selfVerify(ctx context.Context, dir, keyID string) error {
	info, err := d.Signer.PublicKey(ctx)
	if err != nil {
		return wrapSigner(err)
	}
	if info.KeyID != keyID {
		return refuse(signing.ReasonKeyIDMismatch, "the release key changed since the release was signed")
	}
	tmp, err := os.MkdirTemp(releasesDir(d.Home), ".keys-")
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "a trust file could not be written")
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	keys := filepath.Join(tmp, "ci-release.keys")
	if err = os.WriteFile(keys, []byte(info.TrustLine()+"\n"), 0o600); err != nil {
		return refuse(signing.ReasonSignerFailed, "a trust file could not be written")
	}
	vf := d.Verify
	if vf == nil {
		vf = verify.Release
	}
	res := vf(ctx, dir, verify.Options{Root: d.Root, Capture: d.Capture, OperatorKeys: keys})
	if !res.Verified {
		msg := "the release does not verify: " + res.Message
		if len(res.Evidence) > 0 {
			msg += " [" + strings.Join(res.Evidence, ",") + "]"
		}
		return &Error{ErrCode: verify.Code, Reason: res.Reason, Msg: msg}
	}
	return nil
}
