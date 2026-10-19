package session

import (
	"encoding/base64"
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

// signEvidence signs the evidence bytes exactly as they travel in the result
// frame. The value is unpadded standard base64 of the raw signature.
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
