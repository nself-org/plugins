package run

// Purpose: the default PolicyResolver (TRUST delta 2). Until P7-TRUST-27
// registers the real one, the policy is the built-in defaults and the digest
// is the sha256 of their canonical JSON, so the same defaults always give the
// same digest.
// Inputs:  a PolicyRequest. Outputs: a PolicyResult.
// Constraints: no I/O; the defaults are a value, never read from the repo.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// defaultPolicy are the built-in local-run defaults.
func defaultPolicy() map[string]any {
	return map[string]any{
		"trust.privacy_zone":  "local-only",
		"trust.isolation_min": "process",
		"trust.network":       "internet",
		"trust.secrets":       []any{},
	}
}

type defaultResolver struct{}

func (defaultResolver) Resolve(context.Context, PolicyRequest) (PolicyResult, error) {
	b, err := trust.Canonical(defaultPolicy())
	if err != nil {
		return PolicyResult{}, err
	}
	sum := sha256.Sum256(b)
	return PolicyResult{Digest: hex.EncodeToString(sum[:])}, nil
}
