package attest

// Purpose: runner-key signing of an evidence document (Epic D13). The signed payload is
// "nself-ci-evidence/v1\n" followed by trust.Canonical of the document's JSON object, decoded with
// UseNumber, without `signature` and `freshness`; unknown fields stay in, so a field a newer producer
// added is covered. The key id is "ci-node-" plus the node key id (16 hex of sha256 of the public key).
// Inputs:  a model.Evidence or the raw document, and a nodekey.Signer.
// Outputs: the signed evidence (signed-remote producer, runner-ed25519 signature) or the raw signature.
// Constraints: only Ed25519 through the node key; the signature value is the unpadded standard base64 of
//          the raw signature (the form the agent session puts in a result frame); a document with a
//          duplicate key, a float, trailing data or a non-object top level has no payload.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// PayloadPrefix is the domain separation label of an evidence signature.
const PayloadPrefix = "nself-ci-evidence/v1\n"

// KeyIDPrefix is the purpose prefix of an evidence key id (sdk purpose ci-node).
const KeyIDPrefix = "ci-node-"

// EvidenceKeyID is the evidence key id of a node key id.
func EvidenceKeyID(nodeKeyID string) string { return KeyIDPrefix + nodeKeyID }

// decodeObject parses a document into its top-level object, numbers kept as written.
func decodeObject(doc []byte) (map[string]any, error) {
	if hasDuplicateKey(doc) {
		return nil, errors.New("duplicate object key")
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after the evidence document")
	}
	if obj == nil {
		return nil, errors.New("evidence is not a JSON object")
	}
	return obj, nil
}

func payloadOf(obj map[string]any) ([]byte, error) {
	delete(obj, "signature")
	delete(obj, "freshness")
	c, err := trust.Canonical(obj)
	if err != nil {
		return nil, err
	}
	return append([]byte(PayloadPrefix), c...), nil
}

// SigningPayload is the byte string a runner signs for the document doc.
func SigningPayload(doc []byte) ([]byte, error) {
	obj, err := decodeObject(doc)
	if err != nil {
		return nil, err
	}
	return payloadOf(obj)
}

// SignDocumentRaw signs the raw document as a signed-remote one: the payload is built with the producer
// kind set to signed-remote, which is what the coordinator stores once it accepts the signature. It
// returns the raw Ed25519 signature.
func SignDocumentRaw(doc []byte, s nodekey.Signer) ([]byte, error) {
	if s == nil {
		return nil, errors.New("no signer")
	}
	obj, err := decodeObject(doc)
	if err != nil {
		return nil, err
	}
	producer, _ := obj["producer"].(map[string]any)
	if producer == nil {
		return nil, errors.New("evidence has no producer")
	}
	producer["kind"] = "signed-remote"
	payload, err := payloadOf(obj)
	if err != nil {
		return nil, err
	}
	return s.Sign(payload)
}

// Sign returns ev as signed-remote evidence signed with the node key.
func Sign(ev model.Evidence, s nodekey.Signer) (model.Evidence, error) {
	if s == nil {
		return ev, errors.New("no signer")
	}
	ev.Producer.Kind = "signed-remote"
	ev.Signature = model.Signature{Kind: "runner-ed25519"}
	doc, err := json.Marshal(ev)
	if err != nil {
		return ev, err
	}
	raw, err := SignDocumentRaw(doc, s)
	if err != nil {
		return ev, err
	}
	id, val := EvidenceKeyID(s.KeyID()), base64.RawStdEncoding.EncodeToString(raw)
	ev.Signature = model.Signature{Kind: "runner-ed25519", KeyID: &id, Value: &val}
	return ev, nil
}
