package netscope

// Purpose: the host allowlist of the restricted scope (package registries plus declared hosts).
// Inputs:  a scope, a host name from a CONNECT target or an absolute-form request, and extra declared hosts.
// Outputs: whether the name may be dialled; the address checks of Allowed still apply afterwards.
// Constraints: exact names only (no suffix or wildcard match), case-insensitive, one trailing dot ignored; an
// IP literal is never on the list; internet allows any name, none and the LAN scopes allow nothing here.

import (
	"net/netip"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// RestrictedHosts are the package registries a restricted job may reach.
var RestrictedHosts = []string{
	"proxy.golang.org", "sum.golang.org",
	"registry.npmjs.org", "registry.yarnpkg.com",
	"crates.io", "static.crates.io", "index.crates.io",
	"pypi.org", "files.pythonhosted.org",
}

// NormalizeHost lowercases a host and drops one trailing dot; "" for an unusable name.
func NormalizeHost(h string) string {
	h = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
	if h == "" || strings.ContainsAny(h, " \t\r\n/\\@") {
		return ""
	}
	return h
}

// HostAllowed reports whether scope allows connecting to the name host. extra are the hosts the operator's
// policy declared on top of RestrictedHosts.
func HostAllowed(scope model.NetworkScope, host string, extra []string) bool {
	h := NormalizeHost(host)
	if h == "" {
		return false
	}
	switch scope {
	case "internet":
		return true
	case "restricted":
		if _, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
			return false // an address literal skips the name policy
		}
		for _, list := range [][]string{RestrictedHosts, extra} {
			for _, allowed := range list {
				if NormalizeHost(allowed) == h {
					return true
				}
			}
		}
	}
	return false
}

// PortAllowed reports whether the scope allows a destination port. restricted serves package registries over
// HTTPS and plain HTTP only, so an allowlisted host cannot be used to reach another service on the same name;
// internet allows any port of a public host.
func PortAllowed(scope model.NetworkScope, port string) bool {
	if scope == "restricted" {
		return port == "443" || port == "80"
	}
	return true
}
