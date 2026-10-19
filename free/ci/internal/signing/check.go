package signing

// Purpose: re-check a release-manifest payload against the stored facts of its
// run before anything is signed (Epic CACHE D12): the signer trusts neither
// the coordinator nor the manifest, only the store.
// Inputs:  payload bytes and the run's facts.
// Outputs: the release attempt id (the secret resolution is for that
// attempt) and the key id the manifest names.
// Constraints: schema v1 through manifest.Decode; revision equals the run's
// revision, the evidence's revision and every record's; the stored evidence
// passed (loadFacts); records come from the manifest's own passed release attempt
// and match in kind, platform and image digests; every artifact (name,
// digest, size) and every SBOM digest (a record of kind sbom) is a committed,
// byte-verified record of this run; the evidence digest is the digest of the stored pipeline evidence
// and that evidence passed; the release attempt is a passed attempt of a
// release job of this run; release.pipeline and finished_at are the evidence's.
// The first failure is returned; nothing is signed.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

// checkManifest validates one manifest payload.
func (f *runFacts) checkManifest(ctx context.Context, payload []byte) (attemptID, keyID string, err error) {
	if err := rejectAmbiguousEscapes(payload, "manifest"); err != nil {
		return "", "", err
	}
	m, err := manifest.Decode(payload)
	if err != nil {
		return "", "", Failf(ReasonPayloadRejected, "manifest: %v", err)
	}
	reject := func(format string, a ...any) (string, string, error) {
		return "", "", Failf(ReasonPayloadRejected, "manifest: "+format, a...)
	}
	if m.Release.RunID != f.runID {
		return reject("release.run_id is not the run being signed")
	}
	if m.Revision != f.pipe.Revision || m.Revision != f.evidence.Revision {
		return reject("revision is not the revision of the run")
	}
	if m.RepoID != f.evidence.RepoID || f.evidence.RunID != f.runID {
		return reject("repo_id or run id differs from the stored evidence")
	}
	if m.Evidence.SHA256 != f.evDigest {
		return reject("evidence.sha256 is not the digest of the stored pipeline evidence")
	}
	if m.Release.Pipeline != f.evidence.Pipeline.Name || !f.finishedAt(m.Release.FinishedAt) {
		return reject("release.pipeline or release.finished_at differ from the stored evidence")
	}
	if !f.releaseAttempt(m.Release.JobID, m.Release.AttemptID) {
		return reject("release.attempt_id is not a passed attempt of a release job of this run")
	}
	for _, a := range m.Artifacts {
		if err := f.checkArtifact(ctx, a, m.Release.AttemptID); err != nil {
			return "", "", err
		}
		for _, sb := range []struct{ digest, suffix string }{{a.SBOM.CycloneDX.SHA256, ".cdx.json"}, {a.SBOM.SPDX.SHA256, ".spdx.json"}} {
			want := a.Name + sb.suffix
			named := func(r records.Record) bool { return sbomRecordName(r.Name, want) }
			ok, err := f.match(ctx, "", records.KindSBOM, sb.digest, m.Release.AttemptID, named)
			if err != nil {
				return "", "", err
			}
			if !ok {
				return "", "", Failf(ReasonDigestNotCommitted, "the SBOM digest of artifact %q is not the committed sbom record %s of the release attempt", echo(a.Name), echo(want))
			}
		}
	}
	return m.Release.AttemptID, m.Signing.KeyID, nil
}

// checkArtifact needs a committed, verified record of the release attempt with
// this name, digest, size and kind, the platform and, for an image, the image
// reference, index digest and per-platform digests the manifest lists.
func (f *runFacts) checkArtifact(ctx context.Context, a manifest.Artifact, attempt string) error {
	ok, err := f.match(ctx, a.Name, "", a.SHA256, attempt, func(r records.Record) bool { return recordMatches(a, r) })
	if err != nil {
		return err
	}
	if !ok {
		return Failf(ReasonDigestNotCommitted, "artifact %q (digest %s) is not a committed record of the release attempt", echo(a.Name), echo(a.SHA256))
	}
	return nil
}

// recordMatches compares everything a manifest claims about an artifact.
func recordMatches(a manifest.Artifact, r records.Record) bool {
	if string(r.Kind) != a.Kind || r.Size != a.Size || (a.Platform != nil && *a.Platform != r.Platform) {
		return false
	}
	return imageMatches(a.Image, r.Image)
}

// imageMatches reports whether the manifest's image details are the record's.
func imageMatches(m *manifest.Image, r *records.ImageInfo) bool {
	if m == nil || r == nil {
		return m == nil && r == nil
	}
	if m.Ref != r.Ref || m.IndexDigest != r.IndexDigest || len(m.Manifests) != len(r.Manifests) {
		return false
	}
	want := map[string]string{}
	for _, x := range r.Manifests {
		want[x.Platform] = x.Digest
	}
	for _, x := range m.Manifests {
		if d, ok := want[x.Platform]; !ok || d != x.Digest {
			return false
		}
	}
	return true
}

// releaseAttempt reports whether attempt is a passed attempt of job, and job
// is a release-kind job of the run.
func (f *runFacts) releaseAttempt(job, attempt string) bool {
	isRelease := false
	for _, j := range f.jobs {
		if j.ID == job && j.Kind == "release" {
			isRelease = true
		}
	}
	if !isRelease {
		return false
	}
	for _, a := range f.attempts {
		if a.ID == attempt && a.JobID == job {
			return a.State == "passed"
		}
	}
	return false
}

// finishedAt reports whether the manifest's finished_at is the evidence's timings.finished_at.
func (f *runFacts) finishedAt(s string) bool {
	if f.evidence.Timings.FinishedAt == nil {
		return false
	}
	want, err1 := time.Parse(time.RFC3339Nano, *f.evidence.Timings.FinishedAt)
	got, err2 := time.Parse(time.RFC3339Nano, s)
	return err1 == nil && err2 == nil && want.Equal(got)
}

// sbomDirs are the places an SBOM record may sit: bare, under sbom/ (the
// manifest layout) or under the job's implicit .nself-ci/sbom/ output.
var sbomDirs = []string{"", "sbom/", ".nself-ci/sbom/"}

// sbomRecordName reports whether a record name is exactly want in one of the
// SBOM directories (a longer path, even one ending in want, is another file).
func sbomRecordName(name, want string) bool {
	for _, d := range sbomDirs {
		if name == d+want {
			return true
		}
	}
	return false
}
