package netscope

// Purpose: the scope rules that do not need a runtime: which scope a job runs under and whether a runner's
// probed scopes make it free of a LAN route.
// Inputs:  a trust class, the declared scope (possibly empty) and the scopes a runner proved it enforces.
// Outputs: the effective scope, or an E680 refusal; the derived NoLANRoute fact (never declared).
// Constraints: a scope is never raised or silently lowered: an untrusted job that declares lan or privileged
// is refused, an undeclared scope takes the trust class default (untrusted: restricted).

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Effective returns the scope an attempt of class runs under.
func Effective(class model.TrustClass, declared model.NetworkScope) (model.NetworkScope, error) {
	if declared == "" {
		return trust.DefaultNetwork(class), nil
	}
	if !declared.Valid() {
		return "", scopeFailed("unknown network scope %q", declared)
	}
	if trust.DecodeTrust(string(class)) == "untrusted" && trust.RankNetwork(declared) >= trust.RankNetwork("lan") {
		return "", scopeFailed("an untrusted job cannot run under the %s network scope", declared)
	}
	return declared, nil
}

// Enforceable lists the scopes this package can ever enforce on a runner: lan and privileged are raw network
// access and need no enforcement, so they are not in the list.
func Enforceable() []model.NetworkScope {
	return []model.NetworkScope{"none", "restricted", "internet"}
}

// NoLANRoute derives the runner fact for a job's scope: true only when the runner proved it enforces that
// scope (it is in enforced) and the scope itself offers no LAN route. It is never declared by the runner.
func NoLANRoute(scope model.NetworkScope, enforced []model.NetworkScope) bool {
	if trust.RankNetwork(scope) < 0 || trust.RankNetwork(scope) >= trust.RankNetwork("lan") {
		return false
	}
	for _, s := range enforced {
		if s == scope {
			return true
		}
	}
	return false
}

// Apply sets the derived NoLANRoute of d for a job of the given scope. SCHED calls it before Admissible.
func Apply(d *trust.Declaration, scope model.NetworkScope, enforced []model.NetworkScope) {
	d.NoLANRoute = NoLANRoute(scope, enforced)
}

func scopeFailed(format string, args ...any) error {
	return &Error{Code: CodeScopeFailed, Message: fmt.Sprintf(format, args...)}
}
