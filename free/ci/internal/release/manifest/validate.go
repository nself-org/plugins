package manifest

// Purpose: the field rules of contract:ci.release-manifest v1, shared by Build
// (a producer must not emit a manifest a verifier refuses) and Decode.
// Inputs:  a Manifest. Outputs: nil or an error wrapping ErrInvalid that names
// the first broken field.
// Constraints: pure; every digest is lowercase hex, every path is the fixed
// layout of the release directory, nothing is trusted from the document.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrInvalid wraps every rejection of a malformed or inconsistent manifest.
var ErrInvalid = errors.New("release manifest: invalid")

// ErrSchema is returned for a payload whose schema is not ci.release-manifest/v1
// (another major, or not a release manifest at all).
var ErrSchema = errors.New("release manifest: unsupported schema")

const (
	maxPayload   = 4 << 20
	maxArtifacts = 4096
	maxField     = 512
)

var (
	revisionRe = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	sha256Re   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	digestRe   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	nameRe     = regexp.MustCompile(`^[A-Za-z0-9._-][A-Za-z0-9._/-]{0,199}$`)
	platformRe = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+$`)
	keyIDRe    = regexp.MustCompile(`^ci-release-[0-9a-f]{16}$`) // signing.KeyID(purpose, pub)
	refRe      = regexp.MustCompile(`^[!-~]{1,512}$`)            // printable ASCII: an image ref must not be spoofable
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// text checks a free-text field: present, bounded, valid UTF-8.
func text(field, v string) error {
	switch {
	case v == "":
		return invalid("%s is empty", field)
	case utf8.RuneCountInString(v) > maxField:
		return invalid("%s is longer than %d characters", field, maxField)
	case !utf8.ValidString(v):
		return invalid("%s is not valid UTF-8", field)
	case strings.ContainsFunc(v, unicode.IsControl):
		return invalid("%s holds a control character", field)
	}
	return nil
}

func fileRef(field string, f FileRef, path string) error {
	if f.Path != path {
		return invalid("%s.path is %q, want %q", field, f.Path, path)
	}
	if !sha256Re.MatchString(f.SHA256) {
		return invalid("%s.sha256 is not 64 lowercase hex digits", field)
	}
	return nil
}

// validate applies every rule of the contract to m.
func validate(m Manifest) error {
	if m.Schema != Schema {
		return fmt.Errorf("%w: %q", ErrSchema, m.Schema)
	}
	if err := text("repo_id", m.RepoID); err != nil {
		return err
	}
	if !revisionRe.MatchString(m.Revision) {
		return invalid("revision is not 40 or 64 lowercase hex digits")
	}
	if err := validateRelease(m); err != nil {
		return err
	}
	if err := validateArtifacts(m.Artifacts); err != nil {
		return err
	}
	if err := fileRef("provenance", FileRef{m.Provenance.Path, m.Provenance.SHA256}, ProvenancePath); err != nil {
		return err
	}
	if m.Provenance.PredicateType != ProvenancePredicate || m.Provenance.SLSABuildLevel != SLSALevel {
		return invalid("provenance predicate_type or slsa_build_level is not the v1 value")
	}
	if err := fileRef("checksums", m.Checksums, ChecksumsPath); err != nil {
		return err
	}
	if m.Signing.Purpose != SigningPurpose {
		return invalid("signing.purpose is %q, want %q", m.Signing.Purpose, SigningPurpose)
	}
	if !keyIDRe.MatchString(m.Signing.KeyID) {
		return invalid("signing.key_id is not ci-release-<16 hex digits>")
	}
	if err := text("producer.nself_ci_version", m.Producer.NselfCIVersion); err != nil {
		return err
	}
	return text("producer.coordinator", m.Producer.Coordinator)
}

func validateRelease(m Manifest) error {
	r := m.Release
	for _, f := range []struct{ name, v string }{{"release.run_id", r.RunID}, {"release.pipeline", r.Pipeline},
		{"release.job_id", r.JobID}, {"release.attempt_id", r.AttemptID}, {"release.idempotency_key", r.IdempotencyKey}} {
		if err := text(f.name, f.v); err != nil {
			return err
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, r.FinishedAt); err != nil {
		return invalid("release.finished_at is not an RFC 3339 time")
	}
	if err := fileRef("evidence", FileRef{m.Evidence.Path, m.Evidence.SHA256}, EvidencePath); err != nil {
		return err
	}
	if m.Evidence.Result != EvidenceResult || m.Evidence.Class != EvidenceClass {
		return invalid("evidence result and class must be %q and %q", EvidenceResult, EvidenceClass)
	}
	return nil
}

func validateArtifacts(list []Artifact) error {
	if len(list) == 0 || len(list) > maxArtifacts {
		return invalid("artifacts must hold 1 to %d entries", maxArtifacts)
	}
	seen := make(map[string]bool, len(list))
	for i, a := range list {
		if err := validateArtifact(a); err != nil {
			return fmt.Errorf("%w (artifacts[%d])", err, i)
		}
		folded := strings.ToLower(a.Name) // a case-insensitive checkout would merge two spellings
		if seen[folded] {
			return invalid("artifact name %q repeats (names compare without letter case)", a.Name)
		}
		seen[folded] = true
	}
	// a name that is a directory of another (a and a/b) cannot both exist in the release directory
	for name := range seen {
		for i := 0; i < len(name); i++ {
			if name[i] == '/' && seen[name[:i]] {
				return invalid("artifact %q is both a file and a directory of %q", name[:i], name)
			}
		}
	}
	return nil
}

func validName(name string) bool {
	if !nameRe.MatchString(name) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validateArtifact(a Artifact) error {
	if !validName(a.Name) {
		return invalid("artifact name %q is not allowed", a.Name)
	}
	if a.Path != "artifacts/"+a.Name {
		return invalid("artifact path %q, want artifacts/%s", a.Path, a.Name)
	}
	switch a.Kind {
	case KindPackage, KindBinary, KindArchive, KindImage, KindReport, KindOther:
	default:
		return invalid("artifact kind %q is not known", a.Kind)
	}
	if !sha256Re.MatchString(a.SHA256) || a.Size < 0 {
		return invalid("artifact %q needs a sha256 and a size of 0 or more", a.Name)
	}
	if a.Platform != nil && !platformRe.MatchString(*a.Platform) {
		return invalid("artifact %q platform is not <os>/<arch>", a.Name)
	}
	if (a.Kind == KindImage) != (a.Image != nil) {
		return invalid("artifact %q: image details belong to kind image, and kind image needs them", a.Name)
	}
	if a.Image != nil {
		if err := validateImage(a.Name, *a.Image); err != nil {
			return err
		}
	}
	if err := fileRef("sbom.cyclonedx", a.SBOM.CycloneDX, "sbom/"+a.Name+".cdx.json"); err != nil {
		return err
	}
	return fileRef("sbom.spdx", a.SBOM.SPDX, "sbom/"+a.Name+".spdx.json")
}

func validateImage(name string, im Image) error {
	if !refRe.MatchString(im.Ref) {
		return invalid("image.ref of %q is not 1 to %d printable ASCII characters", name, maxField)
	}
	if !digestRe.MatchString(im.IndexDigest) || len(im.Manifests) == 0 {
		return invalid("image of %q needs an index digest and at least one platform manifest", name)
	}
	seen := map[string]bool{}
	for _, mf := range im.Manifests {
		if text("image platform", mf.Platform) != nil || !digestRe.MatchString(mf.Digest) || seen[mf.Platform] {
			return invalid("image of %q has a bad or repeated platform manifest", name)
		}
		seen[mf.Platform] = true
	}
	return nil
}
