package evidence

// Purpose: locally signed evidence (P7-WIRE-14, D-1215). The release job of a CI-built release runs on
// another machine than the one that promotes it, so the promoting machine holds no run record and
// authenticate() refuses the document. The release job therefore signs the finished document with the
// release key (key class ci-release, the evidence-signing capability of the release job) and ships a DSSE
// envelope; verify opens it against the operator's trusted ci-release keys and accepts that producer.
// Inputs:  SignLocal: the raw coordinator-recorded document and a ci-release signer. Verify: an envelope
//          and $NSELF_CI_HOME/trust/ci-release.{keys,revoked} (the same operator files `artifacts verify` reads).
// Outputs: the envelope JSON; on verify the inner document, judged exactly as an unwrapped one.
// Constraints: fail closed: no trust file, an unknown, revoked or wrong-class key, a bad signature, a
//          payload of another type or an inner document that is not coordinator-recorded refuses the
//          document (exit 11); the signature covers the whole inner document, so any edit breaks it; the
//          envelope never makes a failing, stale or unbound document pass, it only vouches for the origin.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// LocalPayloadType is the DSSE payload type of a locally signed evidence document.
const LocalPayloadType = "application/vnd.nself.ci.evidence.v1+json"

// SignLocal wraps doc in a DSSE envelope signed by s, which must be a ci-release signer.
func SignLocal(doc []byte, s signing.Signer) ([]byte, error) {
	if s == nil {
		return nil, errors.New("no signer")
	}
	if len(doc) == 0 || len(doc) > maxLocalDoc {
		return nil, errors.New("the evidence document is empty or too large")
	}
	if !isEvidenceObject(doc) {
		return nil, errors.New("the evidence document is not a JSON object")
	}
	env, err := signing.SignEnvelope(s, LocalPayloadType, doc)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// maxLocalDoc bounds the inner document (the envelope is base64, so a little larger).
const maxLocalDoc = 4 << 20

func isEvidenceObject(doc []byte) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(doc, &m) == nil && m != nil
}

// localTrustDir is where the operator's ci-release trust files live.
func localTrustDir() string {
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return filepath.Join(h, "trust")
	}
	return ""
}

// asEnvelope reports whether doc is a DSSE envelope (an evidence document has no payloadType).
func asEnvelope(doc []byte) (signing.Envelope, bool) {
	var probe map[string]json.RawMessage
	if json.Unmarshal(doc, &probe) != nil {
		return signing.Envelope{}, false
	}
	if _, ok := probe["payloadType"]; !ok {
		return signing.Envelope{}, false
	}
	if _, ok := probe["schema"]; ok {
		return signing.Envelope{}, false
	}
	var env signing.Envelope
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.DisallowUnknownFields()
	if dec.Decode(&env) != nil {
		return signing.Envelope{}, true // an envelope-shaped file that does not decode is refused by the caller
	}
	return env, true
}

// openLocal verifies the envelope against the trusted release keys and returns the inner document.
func openLocal(env signing.Envelope) ([]byte, error) {
	dir := localTrustDir()
	if dir == "" {
		return nil, errors.New("no operator trust directory (NSELF_CI_HOME is not set)")
	}
	kf, err := os.Open(filepath.Join(dir, "ci-release.keys"))
	if err != nil {
		return nil, errors.New("no trusted ci-release keys on this machine")
	}
	defer func() { _ = kf.Close() }()
	keys, err := signing.ParseKeysFile(kf, signing.PurposeCIRelease)
	if err != nil || len(keys) == 0 {
		return nil, errors.New("the ci-release keys file is unusable")
	}
	var revoked []string
	if rf, rerr := os.Open(filepath.Join(dir, "ci-release.revoked")); rerr == nil {
		defer func() { _ = rf.Close() }()
		if revoked, err = signing.ParseRevokedFile(rf); err != nil {
			return nil, errors.New("the ci-release revoked file is unusable")
		}
	}
	v, err := signing.NewVerifier(signing.PurposeCIRelease, keys, revoked)
	if err != nil {
		return nil, errors.New("the trusted ci-release keys cannot be used")
	}
	typ, payload, _, err := signing.VerifyEnvelope(v, env)
	if err != nil {
		return nil, fmt.Errorf("the release signature does not verify: %w", err)
	}
	if typ != LocalPayloadType {
		return nil, errors.New("the signed payload is not an evidence document")
	}
	if len(payload) > maxLocalDoc || !isEvidenceObject(payload) {
		return nil, errors.New("the signed payload is not an evidence document")
	}
	return payload, nil
}
