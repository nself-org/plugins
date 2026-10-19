package main

// Purpose: install the evidence signer of the agent session (P7-TRUST-20, Epic D13): a result frame's
// evidence is signed with the node key over attest's payload ("nself-ci-evidence/v1\n" + canonical JSON
// without signature and freshness, producer kind signed-remote), under the key id ci-node-<16 hex>. The
// coordinator turns the frame into signed-remote evidence and verifies it with attest.RunnerVerifier.
// Inputs:  the agent state dir: the node key (node.key) and the marker file `signing-eligible`.
// Outputs: session.SetResultSigner(...) when the node is signing eligible; nothing otherwise.
// Constraints: registered as a PreStart hook so serve.go stays as it is; only `serve` signs. The marker
//          is created by the operator or by provisioning (P7-NODE-06) on a node whose jobs run under a
//          separate UID, user namespace or container, so a job cannot read the node key; it must be a
//          regular file, not a symlink, with no group or other write bit. Without it results carry no
//          signature. The coordinator's registry record (signing_eligible) decides regardless.

import (
	"context"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/attest"
)

// signingMarker is the file that makes a node sign its results.
const signingMarker = "signing-eligible"

// setResultSigner is session.SetResultSigner; tests replace it.
var setResultSigner = session.SetResultSigner

func init() { must(RegisterPreStart("sign-evidence", signPreStart)) }

// evidenceSigner signs result evidence with the node key.
type evidenceSigner struct{ key nodekey.Signer }

// KeyID is the evidence key id.
func (s evidenceSigner) KeyID() string { return attest.EvidenceKeyID(s.key.KeyID()) }

// Sign signs the evidence bytes of a result frame.
func (s evidenceSigner) Sign(evidence []byte) ([]byte, error) {
	return attest.SignDocumentRaw(evidence, s.key)
}

// signingEligible reports whether the state dir carries a usable marker.
func signingEligible(state string) bool {
	info, err := os.Lstat(filepath.Join(state, signingMarker))
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0
}

// newEvidenceSigner returns the signer of a signing-eligible state dir; ok is false without the marker.
func newEvidenceSigner(state string) (s evidenceSigner, ok bool, err error) {
	if !signingEligible(state) {
		return s, false, nil
	}
	key, err := nodekey.LoadOrCreate(state)
	if err != nil {
		return s, false, err
	}
	return evidenceSigner{key: key}, true, nil
}

func signPreStart(_ context.Context, command string) error {
	if command != "serve" {
		return nil
	}
	state, err := stateDir()
	if err != nil {
		return err
	}
	s, ok, err := newEvidenceSigner(state)
	if err != nil {
		return err
	}
	if ok {
		setResultSigner(s)
	}
	return nil
}
