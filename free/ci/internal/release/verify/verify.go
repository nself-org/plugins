package verify

// Purpose: Release, the verifier of a release directory (Epic CACHE D16): the
// one library `nself ci artifacts verify`, P7-CACHE-17/18 and P7-DEPL-19 call.
// Inputs:  the release directory and Options (trust roots, the repository the
// release came from, the pinned default-branch commit, a quarantine check).
// Outputs: a Result: verified, or the first failing check with its reason key.
// Constraints: the checks run in Epic D16's order and stop at the first
// failure: payloadType, signature over PAE under ci-release (trust roots are
// the operator scope and the pinned tip of the protected default branch, never
// the working tree or the released revision), payload decode and schema major,
// SHA256SUMS and every file digest, evidence digest and revision, provenance
// signature and subjects, SBOM links, then the evidence verdict at the released
// revision. Nothing runs a process but git through the exec funnel; nothing
// writes. The claimed level is exactly Build L1 and Result carries no other.

import (
	"context"
	"errors"
	"os"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

// ManifestFile is the signed manifest's name inside a release directory.
const ManifestFile = "release-manifest.dsse.json"

// Options say whom to trust and where the released revision lives.
type Options struct {
	// Root is the repository the release was built from. The released
	// revision's inputs are read from its git objects; empty means
	// revision_absent.
	Root string
	// Capture runs git (the exec funnel); nil uses the default.
	Capture evidence.Capture
	// OperatorKeys and OperatorRevoked are the operator-scope trust files
	// (~/.nself/ci/trust/ci-release.{keys,revoked}); a missing file holds no keys.
	OperatorKeys, OperatorRevoked string
	// Pin is the protected default branch's pinned tip (P7-TRUST-27); the trust
	// files are read from that commit. Empty reads none.
	Pin string
	// PinError, when set, says the pin could not be resolved; the verification
	// fails closed (signature) rather than trust the operator files alone, since
	// the branch may hold a revocation.
	PinError string
	// PolicyDigest and RequiredJobs are the protected policy's requirements for
	// the evidence (P7-DEPL-19 supplies them). When empty, the evidence is judged
	// against the requirements it states itself, which a Result warning says.
	PolicyDigest string
	RequiredJobs []string
	// Revoked is an extra revocation check (P7-TRUST-20); nil revokes nothing.
	Revoked func(keyID string) bool
	// Quarantined tells whether the evidence was quarantined; nil quarantines
	// nothing. An error fails closed as quarantined.
	Quarantined func(ctx context.Context, ev model.Evidence) (bool, error)
}

// Result is the outcome of Release; it is what `artifacts verify --json` prints.
type Result struct {
	Verified bool `json:"verified"`
	// Exit is the `artifacts verify` status: 0 verified, 11 invalid, 1 failing or quarantined evidence.
	Exit int `json:"exit_code"`
	// SLSABuildLevel is always "L1" (D14).
	SLSABuildLevel string   `json:"slsa_build_level"`
	Code           string   `json:"code,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Message        string   `json:"message,omitempty"`
	Revision       string   `json:"revision,omitempty"`
	RepoID         string   `json:"repo_id,omitempty"`
	RunID          string   `json:"run_id,omitempty"`
	KeyID          string   `json:"key_id,omitempty"`
	Artifacts      int      `json:"artifacts"`
	Passed         []string `json:"passed"`
	// Evidence lists the evidence reasons behind a stale, unbound or failing verdict.
	Evidence []string `json:"evidence_reasons,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// run is the state of one verification.
type run struct {
	ctx      context.Context
	dir      string
	opt      Options
	res      *Result
	verifier *signing.Verifier
	payload  []byte
	man      manifest.Manifest
	files    *fileSet
	ev       model.Evidence
	evDoc    []byte
	stmt     provenance.Statement
}

// Release verifies the release directory dir.
func Release(ctx context.Context, dir string, opt Options) Result {
	res := Result{SLSABuildLevel: provenance.Level, Passed: []string{}}
	r := &run{ctx: ctx, dir: dir, opt: opt, res: &res}
	steps := []struct {
		name string
		fn   func() *Failure
	}{
		{"signature", r.signature}, {"manifest", r.decode}, {"checksum", r.checksums}, {"evidence_binding", r.evidenceBinding},
		{"provenance", r.provenance}, {"sbom_link", r.sbomLinks}, {"evidence_verdict", r.evidenceVerdict},
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return res.fail(fail(ReasonReleaseMissing, "%s is not a release directory", dir))
	}
	for _, s := range steps {
		if f := s.fn(); f != nil {
			return res.fail(f)
		}
		res.Passed = append(res.Passed, s.name)
	}
	res.Verified, res.Exit = true, ExitVerified
	return res
}

func (res Result) fail(f *Failure) Result {
	res.Verified, res.Exit, res.Code, res.Reason, res.Message = false, f.ExitCode(), Code, f.Reason, f.Message
	return res
}

// reasons turns attest reasons into the strings a Result lists.
func reasons(list []attest.Reason) []string {
	out := make([]string, 0, len(list))
	for _, r := range list {
		out = append(out, string(r))
	}
	return out
}

// errIs reports whether err matches any target.
func errIs(err error, targets ...error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}
