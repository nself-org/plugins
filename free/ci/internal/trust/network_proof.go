package trust

// Purpose: the shared network proof predicate used by placement and network scopes.
// Inputs: effective scope and scopes the runner proved. Output: whether LAN access is excluded.
// Constraints: no proof means false; a scope at or above lan never excludes LAN.

import "github.com/nself-org/plugins/free/ci/internal/model"

// NoLANRoute requires an enforced scope below lan.
func NoLANRoute(scope model.NetworkScope, enforced []model.NetworkScope) bool {
	if RankNetwork(scope) < 0 || RankNetwork(scope) >= RankNetwork("lan") {
		return false
	}
	for _, s := range enforced {
		if s == scope {
			return true
		}
	}
	return false
}

// NetworkProofMatters reports whether a runner's proven network scopes (and so its NoLANRoute fact) can change
// any decision for a job of the class: only untrusted admission reads NoLANRoute (Admissible); MinIsolation and
// the secret gate do not. An unknown or empty class decodes as untrusted, so it matters.
func NetworkProofMatters(class model.TrustClass) bool {
	return DecodeTrust(string(class)) == "untrusted"
}
