package manifest

// Purpose: contract:ci.release-manifest v1 (Epic CACHE §Contracts, D10): the
// payload a release publication wraps in a DSSE envelope. This package owns
// the payload types, the byte-stable builder and the strict decoder; signing
// and publication belong to P7-CACHE-07/18.
// Inputs:  a Manifest (Build) or payload bytes (Decode).
// Outputs: canonical payload bytes, or a validated Manifest.
// Constraints: pure (no process, database or network); experimental until P8
// (X19): another schema major is refused, unknown additive fields are accepted
// and dropped, object keys are sorted, no insignificant whitespace.

// Schema is the value of the payload's `schema` field.
const Schema = "ci.release-manifest/v1"

// PayloadType is the DSSE payloadType of a v1 manifest.
const PayloadType = "application/vnd.nself.ci.release-manifest.v1+json"

// Fixed values of v1.
const (
	EvidencePath        = "evidence.json"
	ProvenancePath      = "provenance.intoto.dsse.json"
	ProvenancePredicate = "https://slsa.dev/provenance/v1"
	SLSALevel           = "L1"
	ChecksumsPath       = "SHA256SUMS"
	SigningPurpose      = "ci-release"
	EvidenceClass       = "release"
	EvidenceResult      = "pass"
)

// Artifact kinds.
const (
	KindPackage = "package"
	KindBinary  = "binary"
	KindArchive = "archive"
	KindImage   = "image"
	KindReport  = "report"
	KindOther   = "other"
)

// Manifest is the payload of a release manifest.
type Manifest struct {
	Schema     string      `json:"schema"`
	RepoID     string      `json:"repo_id"`
	Revision   string      `json:"revision"` // 40 or 64 lowercase hex
	Release    Release     `json:"release"`
	Evidence   EvidenceRef `json:"evidence"`
	Artifacts  []Artifact  `json:"artifacts"` // sorted by name
	Provenance Provenance  `json:"provenance"`
	Checksums  FileRef     `json:"checksums"`
	Signing    Signing     `json:"signing"`
	Producer   Producer    `json:"producer"`
}

// Release identifies the release job attempt that produced the artifacts.
type Release struct {
	RunID          string `json:"run_id"`
	Pipeline       string `json:"pipeline"`
	JobID          string `json:"job_id"`
	AttemptID      string `json:"attempt_id"`
	IdempotencyKey string `json:"idempotency_key"`
	FinishedAt     string `json:"finished_at"` // the evidence's timings.finished_at, RFC 3339
}

// EvidenceRef pins the pipeline evidence of the release.
type EvidenceRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Result string `json:"result"`
	Class  string `json:"class"`
}

// FileRef names a file of the release directory and its digest.
type FileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Artifact is one released file.
type Artifact struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"` // artifacts/<name>
	Kind     string  `json:"kind"`
	SHA256   string  `json:"sha256"`
	Size     int64   `json:"size"`
	Platform *string `json:"platform"` // "<os>/<arch>" or null
	Image    *Image  `json:"image"`    // null unless kind is image
	SBOM     SBOM    `json:"sbom"`
}

// Image describes a container image artifact.
type Image struct {
	Ref         string          `json:"ref"`
	IndexDigest string          `json:"index_digest"` // sha256:<64 hex>
	Manifests   []ImageManifest `json:"manifests"`    // sorted by platform
}

// ImageManifest is one platform image of an index.
type ImageManifest struct {
	Platform string `json:"platform"`
	Digest   string `json:"digest"` // sha256:<64 hex>
}

// SBOM names the two SBOM documents of an artifact.
type SBOM struct {
	CycloneDX FileRef `json:"cyclonedx"`
	SPDX      FileRef `json:"spdx"`
}

// Provenance pins the provenance statement.
type Provenance struct {
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	PredicateType  string `json:"predicate_type"`
	SLSABuildLevel string `json:"slsa_build_level"`
}

// Signing names the key purpose and id that sign the envelope.
type Signing struct {
	Purpose string `json:"purpose"`
	KeyID   string `json:"key_id"`
}

// Producer identifies the coordinator that built the manifest.
type Producer struct {
	NselfCIVersion string `json:"nself_ci_version"`
	Coordinator    string `json:"coordinator"`
}
