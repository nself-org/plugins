package hostedev

// Purpose: the artifact and evidence half of the hosted evidence check: the spec digest, the zip and
// the ci.evidence/v1 document inside it, the attempt, revision and producer bindings.
// Inputs:  a VerifyInput (see hostedev.go).
// Outputs: the evidence document bytes, or an E705 with one reason key.
// Constraints: sizes are bounded before anything is parsed (zip bomb); the artifact holds exactly one
//          file named evidence.json; the document is held to the published schema first; errors carry
//          the reason key only.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// checkSpec holds the dispatched spec digest to its shape and, when the spec itself is at hand, to
// the spec: a provider's API echoes no dispatch inputs back, so the digest the coordinator journalled
// is what is bound here and the workflow re-checks the spec against it before it runs anything.
func checkSpec(in VerifyInput) error {
	if !digestRE.MatchString(in.SpecDigest) {
		return reject(ReasonSpec)
	}
	if len(in.Spec) > 0 {
		sum := sha256.Sum256(in.Spec)
		if "sha256:"+hex.EncodeToString(sum[:]) != in.SpecDigest {
			return reject(ReasonSpec)
		}
	}
	return nil
}

// ReadEvidence returns the evidence document inside an artifact zip, under the same size and shape
// rules Verify applies; a caller that needs the document after Verify accepted the zip uses this.
func ReadEvidence(zipBytes []byte) ([]byte, error) {
	return readEvidence(VerifyInput{EvidenceZip: zipBytes})
}

// readEvidence returns the evidence.json inside the artifact zip.
func readEvidence(in VerifyInput) ([]byte, error) {
	if in.ArtifactSize > MaxArtifactBytes || int64(len(in.EvidenceZip)) > MaxArtifactBytes {
		return nil, reject(ReasonTooLarge)
	}
	if len(in.EvidenceZip) == 0 {
		return nil, reject(ReasonMissing)
	}
	zr, err := zip.NewReader(bytes.NewReader(in.EvidenceZip), int64(len(in.EvidenceZip)))
	if err != nil || len(zr.File) != 1 || zr.File[0].Name != EvidenceFile || zr.File[0].UncompressedSize64 > MaxArtifactBytes {
		return nil, reject(ReasonArtifact)
	}
	f, err := zr.File[0].Open()
	if err != nil {
		return nil, reject(ReasonArtifact)
	}
	defer func() { _ = f.Close() }()
	doc, err := io.ReadAll(io.LimitReader(f, MaxArtifactBytes+1))
	if err != nil || len(doc) > MaxArtifactBytes {
		return nil, reject(ReasonArtifact)
	}
	return doc, nil
}

func checkEvidence(in VerifyInput, doc []byte) error {
	schemas, err := model.Schemas()
	if err != nil {
		return reject(ReasonEvidence)
	}
	if model.ValidateJSON(schemas["ci-evidence.v1.schema.json"], doc) != nil {
		return reject(ReasonEvidence)
	}
	var ev model.Evidence
	if json.Unmarshal(doc, &ev) != nil || ev.Schema != "ci.evidence/v1" {
		return reject(ReasonEvidence)
	}
	switch {
	case ev.Attempt == nil || ev.Attempt.ID != in.AttemptID || in.AttemptID == "":
		return reject(ReasonAttempt)
	case !strings.EqualFold(ev.Revision, in.Revision) || in.Revision == "":
		return reject(ReasonRevision)
	case !versionAtLeast(ev.Producer.Version, in.MinAgent):
		return reject(ReasonProducerStale)
	}
	return nil
}

// versionAtLeast compares dotted numeric versions ("1", "1.5.0"); a missing part is 0 and a version
// that is not numeric is below every minimum. An empty minimum accepts any numeric version.
func versionAtLeast(got, minimum string) bool {
	g, ok := parts(got)
	if !ok {
		return false
	}
	m, ok := parts(minimum)
	if !ok {
		return false
	}
	for i := 0; i < max(len(g), len(m)); i++ {
		var a, b int
		if i < len(g) {
			a = g[i]
		}
		if i < len(m) {
			b = m[i]
		}
		if a != b {
			return a > b
		}
	}
	return true
}

func parts(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil, true
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}
