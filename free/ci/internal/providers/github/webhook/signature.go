package webhook

// Purpose: the one place an X-Hub-Signature-256 header is checked (Epic D8, X3). It wraps NODE-10's
// exported constant-time verifier and adds the two rules the v1.4 verifier lacks: an empty secret
// never verifies (HMAC with an empty key is computable by anyone) and a header of the wrong length
// is refused before any hashing.
// Inputs:  the secret, the header value and the raw body.
// Outputs: nil when the header is the HMAC-SHA256 of the body under the secret, else an error.
// Constraints: the error text carries no header, secret or body bytes.

import (
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/serve"
)

const sigPrefix = "sha256="

var errBadSignature = errors.New("signature does not match")

// checkSignature compares in constant time through serve.VerifySignature (hmac.Equal).
func checkSignature(secret, header string, body []byte) error {
	if secret == "" || len(header) != len(sigPrefix)+64 {
		return errBadSignature
	}
	if serve.VerifySignature(secret, header, body) != nil {
		return errBadSignature
	}
	return nil
}
