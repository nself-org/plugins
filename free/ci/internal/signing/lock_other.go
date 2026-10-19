//go:build !unix

package signing

// Purpose: the non-unix key-management lock: none (the signer is unix-only in v1.5).
// Inputs: none. Outputs: an unlock func that does nothing.
// Constraints: see stdio_other.go.

import "context"

// Lock implements SecretStore.
func (s *CLIStore) Lock(context.Context) (func(), error) {
	return nil, Failf(ReasonSignerFailed, "the release signer needs a unix host")
}
