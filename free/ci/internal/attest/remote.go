package attest

// Purpose: the seam for verifying runner signatures. P7-TRUST-20 registers
// the real verifier (pinned node keys, replay set); this Ticket needs none.
// Inputs:  RegisterRemoteVerifier(v) from an init() or a test.
// Outputs: verifySignature, used by Verify and GateAccept.
// Constraints: fail closed: with no verifier registered, signed-remote
// evidence is never accepted; a verifier that errors or panics is a refusal.

import (
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// RemoteVerifier checks the runner signature of a signed-remote document. doc
// is the raw document when known, or the re-marshalled evidence when a gate
// holds only the decoded form (fields the model does not know are then absent
// and a signature over them fails, which is the safe direction).
type RemoteVerifier interface {
	VerifyEvidence(doc []byte, ev model.Evidence) error
}

var (
	remoteMu sync.RWMutex
	remote   RemoteVerifier
)

// RegisterRemoteVerifier installs the remote verifier; nil removes it.
func RegisterRemoteVerifier(v RemoteVerifier) {
	remoteMu.Lock()
	defer remoteMu.Unlock()
	remote = v
}

func verifySignature(doc []byte, ev model.Evidence) (out []Reason) {
	remoteMu.RLock()
	v := remote
	remoteMu.RUnlock()
	if v == nil {
		return []Reason{ReasonSignatureUnverifiable}
	}
	defer func() {
		if recover() != nil {
			out = []Reason{ReasonSignatureUnverifiable}
		}
	}()
	if err := v.VerifyEvidence(doc, ev); err != nil {
		return []Reason{ReasonSignatureUnverifiable}
	}
	return nil
}
