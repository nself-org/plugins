package verify

// Purpose: the provenance and SBOM checks (Epic CACHE D16 `provenance_subject`,
// `sbom_link`): the provenance envelope is signed by the manifest's key, its
// statement describes exactly the manifest's artifacts and build, and every
// SBOM is bound to its artifact through the statement's byproducts.
// Inputs:  the decoded manifest and the verified files.
// Outputs: nil or a Failure.
// Constraints: the statement is trusted only after its DSSE signature verified
// under the same verifier as the manifest; its digests are compared with the
// manifest's, which the manifest signature already binds.

import (
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

// provenance verifies the provenance envelope and statement.
func (r *run) provenance() *Failure {
	m := r.man
	b, err := readFile(r.dir, m.Provenance.Path, maxEnvelope)
	if err != nil {
		return fail(ReasonProvenanceSig, "cannot read %s: %v", m.Provenance.Path, err)
	}
	env, err := parseEnvelope(b)
	if err != nil {
		return fail(ReasonProvenanceSig, "%s is not a DSSE envelope: %v", m.Provenance.Path, err)
	}
	if env.PayloadType != provenance.PayloadType {
		return fail(ReasonProvenanceSig, "payloadType is %q, want %q", clip(env.PayloadType), provenance.PayloadType)
	}
	payload, keyID, f := verifyEnvelope(r.verifier, env, ReasonProvenanceSig)
	if f != nil {
		return f
	}
	if keyID != m.Signing.KeyID {
		return fail(ReasonProvenanceSig, "the provenance is signed by %q, the manifest by %q", keyID, m.Signing.KeyID)
	}
	st, err := provenance.Parse(payload)
	if err != nil {
		return fail(ReasonProvenanceSubject, "%v", err)
	}
	if f = r.checkStatement(st); f != nil {
		return f
	}
	r.stmt = st
	return nil
}

// checkStatement compares the statement with the manifest.
func (r *run) checkStatement(st provenance.Statement) *Failure {
	m := r.man
	subjects := st.SubjectDigests()
	if len(subjects) != len(m.Artifacts) {
		return fail(ReasonProvenanceSubject, "the provenance names %d subjects, the manifest %d artifacts", len(subjects), len(m.Artifacts))
	}
	for _, a := range m.Artifacts {
		if got, ok := subjects[a.Name]; !ok || got != a.SHA256 {
			return fail(ReasonProvenanceSubject, "the provenance has no subject %q with digest %s", a.Name, a.SHA256)
		}
	}
	bd, rd := st.Predicate.BuildDefinition, st.Predicate.RunDetails
	ext := bd.ExternalParameters
	if bd.BuildType != provenance.BuildTypeRelease {
		return fail(ReasonProvenanceSubject, "buildType is %q, want %q", clip(bd.BuildType), provenance.BuildTypeRelease)
	}
	if ext["repository"] != m.RepoID || ext["revision"] != m.Revision || st.GitCommit() != m.Revision {
		return fail(ReasonProvenanceSubject, "the provenance is for another repository or revision")
	}
	if ext["pipeline"] != m.Release.Pipeline || ext["job"] != m.Release.JobID {
		return fail(ReasonProvenanceSubject, "the provenance is for another pipeline or job")
	}
	if rd.Metadata.InvocationID != m.Release.RunID+"/"+m.Release.AttemptID {
		return fail(ReasonProvenanceSubject, "the provenance is for another invocation")
	}
	if !strings.HasPrefix(rd.Builder.ID, "https://nself.org/ci/builder/v1#") {
		return fail(ReasonProvenanceSubject, "the builder id is not a nself-ci builder")
	}
	if fin, err := time.Parse(time.RFC3339Nano, m.Release.FinishedAt); err != nil ||
		rd.Metadata.FinishedOn != fin.UTC().Truncate(time.Second).Format("2006-01-02T15:04:05Z") {
		return fail(ReasonProvenanceSubject, "the provenance finished at another time than the release")
	}
	if got, ok := st.Byproduct(m.Evidence.Path); !ok || got != m.Evidence.SHA256 {
		return fail(ReasonProvenanceSubject, "the provenance does not carry the manifest's evidence digest")
	}
	return r.checkInputs(st)
}

// checkInputs compares the inputs the provenance names with those the evidence
// binds: the config digest and exactly the lockfiles.
func (r *run) checkInputs(st provenance.Statement) *Failure {
	in := r.ev.InputDigests
	if st.Predicate.BuildDefinition.ExternalParameters["config_digest"] != in.Config {
		return fail(ReasonProvenanceSubject, "the provenance names another config digest than the evidence")
	}
	locks := 0
	for _, d := range st.Predicate.BuildDefinition.ResolvedDependencies {
		if d.Name != "" {
			locks++
		}
	}
	if locks != len(in.Lockfiles) {
		return fail(ReasonProvenanceSubject, "the provenance names %d lockfiles, the evidence binds %d", locks, len(in.Lockfiles))
	}
	for _, lf := range in.Lockfiles {
		if got, ok := st.Dependency(lf.Path); !ok || got != lf.SHA256 {
			return fail(ReasonProvenanceSubject, "the provenance does not carry the evidence's digest of %q", lf.Path)
		}
	}
	return nil
}

// sbomLinks checks every SBOM exists and is bound to its artifact.
func (r *run) sbomLinks() *Failure {
	if len(r.files.missingSBOM) > 0 {
		return fail(ReasonSBOMLink, "the SBOM %q is missing", r.files.missingSBOM[0])
	}
	for _, a := range r.man.Artifacts {
		for _, ref := range []struct{ path, sha string }{{a.SBOM.CycloneDX.Path, a.SBOM.CycloneDX.SHA256}, {a.SBOM.SPDX.Path, a.SBOM.SPDX.SHA256}} {
			if got, ok := r.stmt.Byproduct(ref.path); !ok || got != ref.sha {
				return fail(ReasonSBOMLink, "the provenance does not bind %q to its artifact %q", ref.path, a.Name)
			}
		}
	}
	return nil
}
