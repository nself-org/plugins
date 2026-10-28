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
