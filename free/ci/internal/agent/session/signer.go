package session

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Signer signs a result's evidence. The node key (nodekey.Key) is one; P7-TRUST-20
// registers the production one from cmd/agent/sign_hook.go.
type Signer interface {
	KeyID() string
	Sign([]byte) ([]byte, error)
}

var (
	signerMu sync.RWMutex
	signer   Signer
)

// SetResultSigner sets the process-wide result signer; nil clears it. Without
// one, results carry an empty signature and the session still completes.
func SetResultSigner(s Signer) {
	signerMu.Lock()
	defer signerMu.Unlock()
	signer = s
}

func currentSigner(override Signer) Signer {
	if override != nil {
		return override
	}
	signerMu.RLock()
	defer signerMu.RUnlock()
	return signer
}

// canonicalJSON re-encodes a JSON document the way protocol.Encoder does when it
// writes the frame (keys sorted, compact, HTML-escaped, numbers kept as
// written), so the bytes the agent signs are the bytes the coordinator receives.
func canonicalJSON(raw []byte) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after the evidence document")
	}
	if v == nil {
		return nil, errors.New("evidence is null")
	}
	return json.Marshal(v)
}

// signEvidence signs the evidence bytes exactly as they travel in the result
// frame (the caller passes canonicalJSON output). The value is unpadded standard base64 of the raw signature.
func signEvidence(s Signer, evidence []byte) (protocol.Signature, error) {
	if s == nil {
		return protocol.Signature{}, nil
	}
	raw, err := s.Sign(evidence)
	if err != nil {
		return protocol.Signature{}, err
	}
	return protocol.Signature{KeyID: s.KeyID(), Value: base64.RawStdEncoding.EncodeToString(raw)}, nil
}
