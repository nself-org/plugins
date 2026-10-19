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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	unlock, err := lockRun(ctx, d.Home, runID)
	if err != nil {
		return Release{RunID: runID}, d.fail(ctx, runID, f, err)
	}
	defer unlock()
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

// install writes, verifies and records the bundle. The caller holds the run lock.
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
	if err = d.selfVerify(ctx, staged, b.keyID, true); err != nil {
		return Release{}, err
	}
	target := Dir(d.Home, runID)
	commit, undo, err := installDir(staged, target)
	if err != nil {
		return Release{}, refuse(signing.ReasonSignerFailed, "the release directory could not be put in place")
	}
	row := store.ReleaseRow{RunID: runID, Project: f.pipe.Project, Revision: f.pipe.Revision, State: store.ReleasePublished, ManifestSHA256: b.manifestSHA, KeyID: b.keyID}
	if err = d.Ledger.RecordRelease(ctx, row); err != nil {
		undo()
		return Release{}, refuse(signing.ReasonSignerFailed, "the release could not be recorded")
	}
	commit()
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
	if err != nil || sha(b) != row.ManifestSHA256 || d.selfVerify(ctx, dir, row.KeyID, false) != nil {
		return Release{}, false
	}
	return Release{RunID: runID, State: Published, Dir: dir, ManifestSHA256: row.ManifestSHA256, KeyID: row.KeyID}, true
}

// await records the TEAM state: the coordinator never signs (Epic X8). A
// release that is already published is left alone.
func (d Deps) await(ctx context.Context, runID string, f *facts) (Release, error) {
	if row, ok, err := d.Ledger.ReleaseFor(ctx, runID); err == nil && ok && row.State == store.ReleasePublished {
		return Release{RunID: runID, State: Published, Dir: Dir(d.Home, runID), ManifestSHA256: row.ManifestSHA256, KeyID: row.KeyID}, nil
	}
	msg := "this TEAM coordinator holds no release key: run `" + TeamCommand(runID) + "` on your own machine"
	row := store.ReleaseRow{RunID: runID, Project: f.pipe.Project, Revision: f.pipe.Revision, State: store.ReleaseAwaiting, Reason: msg}
	if err := d.Ledger.RecordRelease(ctx, row); err != nil {
		return Release{RunID: runID}, refuse(signing.ReasonSignerFailed, "the release could not be recorded")
	}
	return Release{RunID: runID, State: Awaiting, Message: msg}, nil
}

// failBudget bounds the failure record: the caller's context may be spent.
const failBudget = 30 * time.Second

// fail records `failed` (when the run is known and not already published) and
// returns the error with the recovery command. Text from outside this package
// never reaches the record or the message: only a fixed sentence does. It uses
// a fresh context, so a spent deadline cannot hide the record or the recovery.
func (d Deps) fail(ctx context.Context, runID string, f *facts, err error) error {
	var e *Error
	if !errors.As(err, &e) {
		e = refuse(signing.ReasonSignerFailed, "the publication failed").(*Error)
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), failBudget)
	defer cancel()
	out := *e
	out.Msg = strings.ReplaceAll(out.Msg, d.Home, "<home>")
	if f == nil {
		pipe, ok, perr := d.Src.PipelineByID(rctx, runID)
		switch {
		case perr == nil && !ok: // an unknown run: nothing to record, nothing to recover
			return &out
		case perr == nil:
			f = &facts{pipe: pipe}
		}
	}
	if f != nil {
		if row, ok, rerr := d.Ledger.ReleaseFor(rctx, runID); rerr == nil && ok && row.State == store.ReleasePublished {
			out.Msg += ": release " + runID + " is recorded as published (manifest " + row.ManifestSHA256[:12] + ") but could not be confirmed now"
			return &out
		}
		_ = d.Ledger.RecordRelease(rctx, store.ReleaseRow{RunID: runID, Project: f.pipe.Project, Revision: f.pipe.Revision, State: store.ReleaseFailed, Reason: failedRecord(&out)})
	}
	out.Msg += ": release NOT published; the run's evidence is unchanged; run `" + RecoveryCommand(runID) + "` to publish it"
	return &out
}

// trustFile writes the keys the verification trusts: the signer's current key
// and the operator's own trust file (a rotated key's old line lives there), and
// returns the revoked file when there is one.
func (d Deps) trustFile(dir string, info signing.KeyInfo) (keys, revoked string, err error) {
	keys = filepath.Join(dir, "ci-release.keys")
	body := info.TrustLine() + "\n"
	b, rerr := readOperatorKeys(filepath.Join(d.Home, "trust", "ci-release.keys"))
	if rerr != nil {
		return "", "", rerr // an unreadable trust file is never read as an empty one
	}
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if f := strings.Fields(t); len(f) > 0 && f[0] != info.KeyID { // the signer's own line (any whitespace) is already there
			body += t + "\n"
		}
	}
	if err = os.WriteFile(keys, []byte(body), 0o600); err != nil {
		return "", "", err
	}
	// The verifier tells an absent file from an unreadable one; this path is
	// always passed so an error is never read as "nothing revoked".
	return keys, filepath.Join(d.Home, "trust", "ci-release.revoked"), nil
}

// maxOperatorKeys bounds the operator trust file read here (verify bounds its own).
const maxOperatorKeys = 1 << 20

// readOperatorKeys returns the operator's trust lines: nothing when the file is
// absent, an error when it is anything but a readable regular file.
func readOperatorKeys(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err == nil && !st.Mode().IsRegular() {
		err = os.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxOperatorKeys+1))
	if err == nil && len(b) > maxOperatorKeys {
		err = os.ErrInvalid
	}
	return b, err
}

// selfVerify checks dir with internal/release/verify against the signer's key
// (and the operator's trust file). With exact set the signer's key must be keyID:
// the release was just signed with it.
func (d Deps) selfVerify(ctx context.Context, dir, keyID string, exact bool) error {
	info, err := d.Signer.PublicKey(ctx)
	if err != nil {
		return wrapSigner(err)
	}
	if exact && info.KeyID != keyID {
		return refuse(signing.ReasonKeyIDMismatch, "the release key changed while the release was being signed")
	}
	tmp, err := os.MkdirTemp(releasesDir(d.Home), ".keys-")
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "a trust file could not be written")
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	keys, revoked, err := d.trustFile(tmp, info)
	if err != nil {
		return refuse(signing.ReasonSignerFailed, "a trust file could not be written")
	}
	vf := d.Verify
	if vf == nil {
		vf = verify.Release
	}
	res := vf(ctx, dir, verify.Options{Root: d.Root, Capture: d.Capture, OperatorKeys: keys, OperatorRevoked: revoked})
	if !res.Verified {
		msg := "the release does not verify: " + res.Message
		if len(res.Evidence) > 0 {
			msg += " [" + strings.Join(res.Evidence, ",") + "]"
		}
		return &Error{ErrCode: verify.Code, Reason: res.Reason, Msg: strings.ReplaceAll(msg, d.Home, "<home>")}
	}
	return nil
}
