package attest

// Purpose: verifies the signature of a result frame (D-1103). The agent signs the frame's evidence under
// the signed-remote payload (SignDocumentRaw) and sends the key id and signature beside it, not inside
// it; the coordinator rebuilds the signed-remote document and checks it with RunnerVerifier, so the key,
// purpose, revocation, lease holder, eligibility and replay rules are the evidence gate's own.
// Inputs:  a ledger, the evidence bytes of the frame, and the frame's key id and signature text.
// Outputs: nil, or a *RemoteError whose Reason says why (code E686).
// Constraints: both signature fields are required (an unsigned frame is the caller's policy, not
//          decided here); fail closed on every ledger error; an accepted signature is consumed.

import (
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// VerifyFrame checks the signature value under keyID over the evidence of a result frame.
func VerifyFrame(led func() Ledger, evidence []byte, keyID, value string) error {
	if keyID == "" || value == "" {
		return remoteFail(RemoteSignatureMissing, "no runner signature", nil)
	}
	obj, err := decodeObject(evidence)
	if err != nil {
		return remoteFail(RemoteSignatureInvalid, "no signing payload: "+err.Error(), nil)
	}
	producer, _ := obj["producer"].(map[string]any)
	if producer == nil {
		return remoteFail(RemoteSignatureInvalid, "evidence has no producer", nil)
	}
	producer["kind"] = "signed-remote"
	obj["signature"] = map[string]any{"kind": "runner-ed25519", "key_id": keyID, "value": value}
	doc, err := json.Marshal(obj)
	if err != nil {
		return remoteFail(RemoteSignatureInvalid, "no signing payload: "+err.Error(), nil)
	}
	var ev model.Evidence
	if err = json.Unmarshal(doc, &ev); err != nil {
		return remoteFail(RemoteSignatureInvalid, "the evidence does not decode: "+err.Error(), nil)
	}
	return (&RunnerVerifier{Ledger: led}).VerifyEvidence(doc, ev)
}
