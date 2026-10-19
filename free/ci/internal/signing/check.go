package signing

// Purpose: re-check a release-manifest payload against the stored facts of its
// run before anything is signed (Epic CACHE D12): the signer trusts neither
// the coordinator nor the manifest, only the store.
// Inputs:  payload bytes and the run's facts.
// Outputs: the release attempt id (the secret resolution is for that
// attempt) and the key id the manifest names.
// Constraints: schema v1 through manifest.Decode; revision equals the run's
// revision, the evidence's revision and every record's; every artifact (name,
// digest, size) and every SBOM digest is a committed, byte-verified record of
// this run; the evidence digest is the digest of the stored pipeline evidence
// and that evidence passed; the release attempt is a passed attempt of a
// release job of this run. The first failure is returned; nothing is signed.

import (
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

// checkManifest validates one manifest payload.
func (f *runFacts) checkManifest(payload []byte) (attemptID, keyID string, err error) {
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
	if string(f.evidence.Result) != manifest.EvidenceResult {
		return reject("the stored evidence of the run is not a pass")
	}
	if !f.releaseAttempt(m.Release.JobID, m.Release.AttemptID) {
		return reject("release.attempt_id is not a passed attempt of a release job of this run")
	}
	for _, a := range m.Artifacts {
		if err := f.checkArtifact(a); err != nil {
			return "", "", err
		}
		for _, d := range []string{a.SBOM.CycloneDX.SHA256, a.SBOM.SPDX.SHA256} {
			if len(f.committed(d)) == 0 {
				return "", "", Failf(ReasonDigestNotCommitted, "the SBOM digest of artifact %q is not a committed record of this run", echo(a.Name))
			}
		}
	}
	return m.Release.AttemptID, m.Signing.KeyID, nil
}

// checkArtifact needs a committed, verified record with this name, digest and size.
func (f *runFacts) checkArtifact(a manifest.Artifact) error {
	for _, r := range f.committed(a.SHA256) {
		if r.Name == a.Name && r.Size == a.Size {
			return nil
		}
	}
	return Failf(ReasonDigestNotCommitted, "artifact %q (digest %s) is not a committed record of this run", echo(a.Name), echo(a.SHA256))
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
