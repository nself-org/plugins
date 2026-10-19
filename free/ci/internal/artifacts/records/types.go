package records

// Purpose: the data types of an artifact record (Epic CACHE D6): the
// normalized declaration, the stored record and the facts of the producing attempt.
// Inputs:  model.ArtifactDecl values from the job config.
// Outputs: Decl, Record, Facts, ImageInfo, the state and kind vocabularies.
// Constraints: pure data; a Record is immutable after commit except ExpiresAt
// (prune) and Missing (a blob gone after a store restore).

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

// Kind is what an artifact is (contract:ci.pipeline-config artifacts[].kind).
type Kind string

// The kinds a job may declare; the default is KindOther.
const (
	KindPackage  Kind = "package"
	KindBinary   Kind = "binary"
	KindArchive  Kind = "archive"
	KindImage    Kind = "image"
	KindReport   Kind = "report"
	KindCoverage Kind = "coverage"
	KindLog      Kind = "log"
	KindBench    Kind = "benchmark"
	KindSBOM     Kind = "sbom"
	KindOther    Kind = "other"
)

var kinds = map[Kind]bool{KindPackage: true, KindBinary: true, KindArchive: true, KindImage: true, KindReport: true,
	KindCoverage: true, KindLog: true, KindBench: true, KindSBOM: true, KindOther: true}

// ParseKind maps "" to KindOther and refuses an unknown kind.
func ParseKind(s string) (Kind, bool) {
	if s == "" {
		return KindOther, true
	}
	return Kind(s), kinds[Kind(s)]
}

// Record states: staged bytes are in the store but not yet part of a pass.
const (
	StateStaged    = "staged"
	StateCommitted = "committed"
)

// KeepRetention is the retention value that never expires.
const KeepRetention = "keep"

// Decl is one normalized declaration of a job (a path or glob and its facts).
type Decl struct {
	Ord        int
	Path, Name string
	Kind       Kind
	Retention  string // "keep" or a duration such as "30d"
	Visibility authz.Visibility
	ImageRef   string
}

// ImageManifest is one per-platform manifest of an OCI layout.
type ImageManifest struct {
	Platform string `json:"platform"`
	Digest   string `json:"digest"`
}

// ImageInfo is what an OCI layout tarball of kind image records.
type ImageInfo struct {
	Ref         string          `json:"ref,omitempty"`
	IndexDigest string          `json:"index_digest"`
	Manifests   []ImageManifest `json:"manifests"`
}

// Record is one stored artifact version of one attempt.
type Record struct {
	ID         string           `json:"id"`
	Project    string           `json:"project"`
	Run        string           `json:"run"`
	Job        string           `json:"job"`
	Attempt    string           `json:"attempt"`
	Revision   string           `json:"revision"`
	Name       string           `json:"name"`
	Kind       Kind             `json:"kind"`
	SHA256     string           `json:"sha256"`
	Size       int64            `json:"size"`
	Platform   string           `json:"platform"`
	RunnerID   string           `json:"runner_id"`
	Isolation  string           `json:"isolation"`
	Producer   string           `json:"producer"`
	Visibility authz.Visibility `json:"visibility"`
	Retention  string           `json:"retention"`
	CreatedAt  time.Time        `json:"created_at"`
	ExpiresAt  *time.Time       `json:"expires_at,omitempty"` // nil: never expires
	State      string           `json:"state"`
	Release    bool             `json:"release"` // produced by a release-kind job
	Missing    bool             `json:"missing,omitempty"`
	Image      *ImageInfo       `json:"image,omitempty"`

	// Path is the declared path or glob; Decl its ordinal; Epoch the lease
	// epoch the record was staged under. Internal to the store.
	Path  string `json:"-"`
	Decl  int    `json:"-"`
	Epoch int64  `json:"-"`
}

// Facts are the stored facts of the producing attempt; the store reads them
// (never the job) so a record cannot claim another project or revision.
type Facts struct {
	Project, Run, Job, Revision string
	RunnerID, Platform          string
	Isolation, Producer         string
	Release                     bool
}

// RecordID is deterministic per (attempt, name), so a retry of a stage is
// idempotent and two attempts never share an id.
func RecordID(attempt, name string) string {
	sum := sha256.Sum256([]byte(attempt + "\x00" + name))
	return "art_" + hex.EncodeToString(sum[:8])
}

// Meta is the authorization-relevant part of the record.
func (r Record) Meta() authz.ArtifactMeta {
	return authz.ArtifactMeta{Project: r.Project, Visibility: r.Visibility}
}
