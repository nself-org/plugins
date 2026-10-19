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

// PolicyResult carries the digest stored on the pipeline row and in evidence, and the secrets allowlist.
type PolicyResult struct {
	Digest string
	// IgnoredRun lists the per-run values the policy did not take, one line each
	// ("key: reason"); a run reports them, it never drops them silently (P7-AUTO-29).
	IgnoredRun []string
	// Ignored lists what the resolver did not apply outside the per-run values
	// ("key: reason"), such as a protected branch it could not fetch (D-1125).
	Ignored []string
	// Fetch is how fresh the protected-branch tip was: fresh, stale or none; empty when unresolved.
	Fetch string
	SecretsInfo
}

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

// ResolvePolicy resolves the effective policy the way a run does: the
// registered resolver, else the built-in defaults (`ci evidence verify` uses
// it to refuse evidence recorded under another policy).
func ResolvePolicy(ctx context.Context, root, pipeline string) (PolicyResult, error) {
	var r PolicyResolver = defaultResolver{}
	if p := Registered.Policy; p != nil {
		r = p
	}
	return r.Resolve(ctx, PolicyRequest{Root: root, Pipeline: pipeline})
}

// policyNotes turns what the resolver did not apply (an unreachable protected
// branch, a stale tip) into run notes (D-1125).
func policyNotes(p PolicyResult) []string {
	var out []string
	for _, line := range p.Ignored {
		out = append(out, "policy not fully applied: "+line)
	}
	if p.Fetch == "stale" {
		out = append(out, "protected-branch policy read from a stale tip (the fetch failed; the cached tip was used)")
	}
	return out
}

// ignoredRunNotes turns the per-run policy values the policy refused into run
// notes ("key: reason" lines).
func ignoredRunNotes(lines []string) []string {
	var out []string
	for _, line := range lines {
		out = append(out, "per-run policy value ignored: "+line)
	}
	return out
}
