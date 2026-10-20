// Package hostedev is the one verifier of evidence a hosted provider's run produced (P7-HOST-03,
// Epic D5). GitHub-hosted dispatch uses it for the run and artifact the API returned; the GitLab
// provider reuses it for the pipeline and the include file at the pipeline sha.
package hostedev

// Purpose: decide whether a hosted run's artifact is the evidence of the attempt the coordinator
// dispatched, and turn it into a provider attestation.
// Inputs:  VerifyInput: the run facts the provider API reported, the workflow file at the run's
//          own commit, the coordinator's rendering of that file for the template version it
//          declares, the dispatched nonce / attempt / revision / spec, and the artifact zip.
// Outputs: model.ProviderAttestation with every field set, or an E705 providers.Error whose
//          Reason is one reason key (see the Reason constants).
// Constraints: nothing here reaches the network; the first failed check decides, in the order of
//          the Reason constants below, and its error text is the key, never the document. The
//          revision that is checked is the dispatched one, never one resolved from a ref. The
//          workflow comparison is byte for byte against the coordinator's own rendering, so a
//          stale branch's older or edited workflow is refused. The producer inside the evidence
//          is self-reported: this package proves binding (run, workflow, nonce, attempt,
//          revision), not that the job told the truth; policy decides whether hosted evidence may
//          gate (TRUST D14).

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// The E705 reason keys.
const (
	ReasonEvent         = "run.event"
	ReasonPath          = "run.path"
	ReasonRef           = "run.ref"
	ReasonRerun         = "run.rerun"
	ReasonNonce         = "run.nonce"
	ReasonAmbiguous     = "run.ambiguous"
	ReasonTemplate      = "template.unknown"
	ReasonStale         = "template.stale"
	ReasonEdited        = "workflow.edited"
	ReasonSpec          = "spec.digest"
	ReasonTooLarge      = "artifact.too_large"
	ReasonMissing       = "artifact.missing"
	ReasonArtifact      = "artifact.invalid"
	ReasonEvidence      = "evidence.invalid"
	ReasonAttempt       = "evidence.attempt"
	ReasonRevision      = "evidence.revision"
	ReasonProducerStale = "producer.stale"
)

// MaxArtifactBytes bounds both the artifact zip and the evidence document inside it.
const MaxArtifactBytes = 16 << 20

// EvidenceFile is the one file the artifact holds.
const EvidenceFile = "evidence.json"

// Run is what the provider API says about the run.
type Run struct {
	ID           string
	Event        string
	Path         string
	HeadBranch   string // the ref the run was dispatched on
	HeadSHA      string // the workflow's own commit, not the revision under test
	DisplayTitle string
	RunAttempt   int // the provider's own re-run counter; 1 for a first run
}

// Want is what the coordinator dispatched the run with.
type Want struct{ Event, Path, Ref string }

// VerifyInput is everything Verify needs.
type VerifyInput struct {
	Provider string
	Run      Run
	Want     Want
	// WorkflowBytes is the workflow file as the provider has it at Run.HeadSHA.
	WorkflowBytes []byte
	// Rendering returns the coordinator's rendering of the workflow for a template version.
	Rendering func(version int) ([]byte, error)
	// TemplateVersion reads the version a workflow declares (negative: none); nil uses the "# template: vN" header.
	TemplateVersion func([]byte) int
	MinTemplate     int
	Nonce           string
	AttemptID       string
	AttemptN        int
	Revision        string
	SpecDigest      string
	Spec            []byte
	MinAgent        string
	// ArtifactID and ArtifactSize come from the artifact listing; EvidenceZip is the download.
	ArtifactID   string
	ArtifactSize int64
	EvidenceZip  []byte
	Now          time.Time
}

func reject(reason string) error {
	return providers.NewError(providers.Config, "E705", reason, 0, time.Time{})
}

var headerVersion = regexp.MustCompile(`(?m)^# template: v([0-9]+)$`)

// declaredVersion reads the "# template: vN" header; -1 means the workflow declares none.
func declaredVersion(b []byte) int {
	m := headerVersion.FindSubmatch(b[:min(len(b), 2048)])
	if m == nil {
		return -1
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return -1
	}
	return n
}

// Verify applies the Epic D5 acceptance checks and returns the attestation.
func Verify(_ context.Context, in VerifyInput) (model.ProviderAttestation, error) {
	if err := checkRun(in); err != nil {
		return model.ProviderAttestation{}, err
	}
	version, err := checkWorkflow(in)
	if err != nil {
		return model.ProviderAttestation{}, err
	}
	if err = checkSpec(in); err != nil {
		return model.ProviderAttestation{}, err
	}
	doc, err := readEvidence(in)
	if err != nil {
		return model.ProviderAttestation{}, err
	}
	if err = checkEvidence(in, doc); err != nil {
		return model.ProviderAttestation{}, err
	}
	wf := sha256.Sum256(in.WorkflowBytes)
	zs := sha256.Sum256(in.EvidenceZip)
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	return model.ProviderAttestation{
		Provider: in.Provider, RunID: in.Run.ID, RunAttempt: in.AttemptN, Ref: in.Revision,
		WorkflowPath: in.Run.Path, WorkflowSHA: in.Run.HeadSHA, WorkflowDigest: "sha256:" + hex.EncodeToString(wf[:]),
		TemplateVersion: "v" + strconv.Itoa(version), Nonce: in.Nonce, ArtifactID: in.ArtifactID,
		ArtifactSHA256: "sha256:" + hex.EncodeToString(zs[:]), VerifiedAt: now.UTC().Format(time.RFC3339),
	}, nil
}

func checkRun(in VerifyInput) error {
	switch {
	case in.Run.Event != in.Want.Event || in.Want.Event == "":
		return reject(ReasonEvent)
	case in.Run.Path != in.Want.Path || in.Want.Path == "":
		return reject(ReasonPath)
	case in.Run.HeadBranch != in.Want.Ref || in.Want.Ref == "":
		return reject(ReasonRef)
	case in.Run.RunAttempt != 1:
		return reject(ReasonRerun)
	case in.Nonce == "" || !strings.Contains(in.Run.DisplayTitle, in.Nonce):
		return reject(ReasonNonce)
	}
	return nil
}

// checkWorkflow compares the workflow at the run's commit with the coordinator's rendering of the
// version it declares, and returns that version.
func checkWorkflow(in VerifyInput) (int, error) {
	read := in.TemplateVersion
	if read == nil {
		read = declaredVersion
	}
	version := read(in.WorkflowBytes)
	if version < 0 {
		return 0, reject(ReasonTemplate)
	}
	if version < in.MinTemplate {
		return 0, reject(ReasonStale)
	}
	if in.Rendering == nil {
		return 0, reject(ReasonTemplate)
	}
	want, err := in.Rendering(version)
	if err != nil {
		return 0, reject(ReasonTemplate)
	}
	if !bytes.Equal(want, in.WorkflowBytes) {
		return 0, reject(ReasonEdited)
	}
	return version, nil
}

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
