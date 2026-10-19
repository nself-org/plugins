// Package publish turns a finished PERSONAL release run into the signed release
// directory (Epic CACHE D15, cap:ci.release-publish).
package publish

// Purpose: the dependencies, the result and the error of the publication of one
// run: SHA256SUMS, the provenance and the manifest, both signed in the signer,
// self-verified, kept under <home>/releases/<run> and recorded.
// Inputs:  Deps (the run's read-only store view, the blobs, the signer, the
// release ledger) and a run id.
// Outputs: Release (the directory, state and digest) or an *Error.
// Constraints: the evidence is never changed; every file is a function of the
// run's stored facts and the signer's deterministic Ed25519 signatures, so a
// second publication of a run produces byte-identical files; the key is only
// ever in the signer; a TEAM coordinator (Deps.Team) never calls the signer and
// resolves no secret. Errors carry E717 (publication/signing) or E719
// (self-verification) and a stable reason key.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/release/verify"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Ledger records releases and reads the coordinator of an attempt.
type Ledger interface {
	RecordRelease(ctx context.Context, r store.ReleaseRow) error
	ReleaseFor(ctx context.Context, runID string) (store.ReleaseRow, bool, error)
	AttemptCoordinator(ctx context.Context, attemptID string) (string, error)
}

// Signer is the signer client (*signing.Client).
type Signer interface {
	Sign(ctx context.Context, runID string, items []signing.Item) ([]sdk.Envelope, error)
	PublicKey(ctx context.Context) (signing.KeyInfo, error)
}

// VerifyFunc is verify.Release; tests replace it.
type VerifyFunc func(ctx context.Context, dir string, opt verify.Options) verify.Result

// Deps are what a publication needs.
type Deps struct {
	Src    signing.Source
	Blobs  signing.Blobs
	Ledger Ledger
	Signer Signer
	// Home is the state directory (NSELF_CI_HOME): releases live in Home/releases/<run>.
	Home string
	// Root is the repository of the run: the self-verification reads the released
	// revision's inputs from its git objects.
	Root       string
	Visibility attest.Visibility
	Capture    evidence.Capture
	Verify     VerifyFunc
	// Team is set on a TEAM coordinator, which holds secret references only (Epic X8).
	Team bool
}

// Release states (the ledger's).
const (
	Published = store.ReleasePublished
	Awaiting  = store.ReleaseAwaiting
	Failed    = store.ReleaseFailed
)

// Release is the outcome of a publication.
type Release struct {
	RunID          string `json:"run_id"`
	State          string `json:"state"`
	Dir            string `json:"dir,omitempty"`
	ManifestSHA256 string `json:"manifest_sha256,omitempty"`
	KeyID          string `json:"key_id,omitempty"`
	// Message is the line to print: the recovery command when not published.
	Message string `json:"message,omitempty"`
}

// ErrNotRelease says the run has nothing to publish: no release job, or one
// that did not pass. It is not a failure and records nothing.
var ErrNotRelease = errors.New("the run has no passing release job: nothing to publish")

// Error is a refused publication.
type Error struct{ ErrCode, Reason, Msg string }

func (e *Error) Error() string { return e.ErrCode + " " + e.Reason + ": " + e.Msg }

// Code is the E-code of the refusal.
func (e *Error) Code() string { return e.ErrCode }

// ExitCode makes `nself ci` exit 1 when the run's own hook returns it.
func (e *Error) ExitCode() int { return 1 }

func refuse(reason, format string, a ...any) error {
	return &Error{ErrCode: signing.CodeSigning, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// failedRecord is the reason text stored for a failed publication: the error
// without any path, cut to one short line.
func failedRecord(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// RecoveryCommand is what a failed run prints.
func RecoveryCommand(runID string) string { return "nself ci artifacts sign " + runID }

// TeamCommand is what a TEAM coordinator's operator runs (P7-CACHE-17 adds --coordinator).
func TeamCommand(runID string) string {
	return "nself ci artifacts sign " + runID + " --coordinator <url>"
}
