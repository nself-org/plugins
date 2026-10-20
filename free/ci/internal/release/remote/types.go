// Package remote signs a TEAM release on the operator's machine (Epic CACHE D15,
// cap:ci.team-release-signing). A TEAM coordinator holds secret references only, so it
// cannot sign; this package has both halves of the exchange: the coordinator's two
// user-API handlers that serve a run's release inputs, and the operator's fetch,
// re-hash and local signing that turn them into the release directory.
package remote

// Purpose: the wire format of the release inputs and the errors both halves share.
// Inputs:  none.
// Outputs: Inputs (the JSON of GET /v1/runs/{id}/release-inputs), the route paths,
// Error (the code, reason key and message of a refusal on either side).
// Constraints: the wire carries facts and digests only, never a key, a secret value or a
// blob; the evidence is the exact stored bytes (base64), because its digest is bound into
// the manifest; a refusal never echoes caller-supplied text beyond a validated run id.

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
)

// Schema names the release-inputs document.
const Schema = "ci.release-inputs/v1"

// Route patterns (the coordinator registers them) and the path builders (the client calls them).
const (
	PatternInputs = "/v1/runs/{id}/release-inputs"
	PatternBlob   = "/v1/runs/{id}/release-blobs/{sha256}"
)

// InputsPath is the path of the release inputs of run.
func InputsPath(run string) string { return "/v1/runs/" + run + "/release-inputs" }

// BlobPath is the path of one release blob of run.
func BlobPath(run, sha256 string) string { return "/v1/runs/" + run + "/release-blobs/" + sha256 }

// Limits that keep a hostile or broken peer from exhausting the operator's machine.
const (
	// MaxInputsBytes bounds the release-inputs document.
	MaxInputsBytes = 32 << 20
	// MaxRecords is the most records one run may list (the store's own query cap).
	MaxRecords = 1000
	// MaxBlobBytes bounds one downloaded release blob; MaxTotalBytes all of a run's blobs together.
	MaxBlobBytes  = int64(32) << 30
	MaxTotalBytes = int64(64) << 30
)

var (
	runIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)
	shaRe   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidRunID reports whether id is a plain run id (one path element).
func ValidRunID(id string) bool { return runIDRe.MatchString(id) && id != "." && id != ".." }

// Pipeline is the run row the signer's checks read.
type Pipeline struct {
	ID          string `json:"id"`
	Project     string `json:"project"`
	Revision    string `json:"revision"`
	InputDigest string `json:"input_digest"`
}

// Job is one job of the run.
type Job struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Attempt is one attempt of a job; Coordinator is the coordinator id that ran it.
type Attempt struct {
	ID          string `json:"id"`
	Job         string `json:"job"`
	N           int    `json:"n"`
	State       string `json:"state"`
	Coordinator string `json:"coordinator,omitempty"`
}

// Inputs is everything the operator's machine needs to rebuild and sign a release,
// except the blobs, which it downloads and re-hashes one by one.
type Inputs struct {
	Schema   string           `json:"schema"`
	Run      string           `json:"run"`
	Pipeline Pipeline         `json:"pipeline"`
	Jobs     []Job            `json:"jobs"`
	Attempts []Attempt        `json:"attempts"`
	Evidence []byte           `json:"evidence"` // the stored pipeline evidence, exact bytes
	Records  []records.Record `json:"records"`
}

// Error is a refusal of either half: Code is E664 (user API), E687 (not authorized), E713
// (not found) or E717 / E719 (signing, verification); Reason is the stable key.
type Error struct{ ErrCode, Reason, Msg string }

func (e *Error) Error() string { return fmt.Sprintf("%s %s: %s", e.ErrCode, e.Reason, e.Msg) }

// Code is the E-code of the refusal.
func (e *Error) Code() string { return e.ErrCode }

// ExitCode is the process exit status of a refused sign.
func (e *Error) ExitCode() int { return 2 }

func fail(code, reason, format string, a ...any) error {
	return &Error{ErrCode: code, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
