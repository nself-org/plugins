package main

import (
	"fmt"
	"strings"
)

// bothFn compares nginx and traefik for one declared-length request and requires status want.
type bothFn func(label, scheme, host, method, path string, n int64, want int)

// redirLimit is the body limit of the "/" location whose return is https://$host$request_uri
// (the server-level redirect of the CLI), else the default; ok is false when it is 0 (none).
func redirLimit(rt route, def int64) (int64, bool) {
	limit := def
	for _, l := range rt.Locations {
		if l.Path == "/" && l.Match == "prefix" && l.Return != nil && l.Return.To != nil && *l.Return.To == "https://$host$request_uri" && l.MaxBodyBytes != nil {
			limit = *l.MaxBodyBytes
		}
	}
	return limit, limit > 0
}

// unmatchedPath returns a path no prefix location of rt covers, or "" when a "/" prefix location
// (or any location that matches everything) leaves none.
func unmatchedPath(rt route) string {
	const p = "/parity-no-location-matches"
	for _, l := range rt.Locations {
		if l.Match == "prefix" && strings.HasPrefix(p, l.Path) {
			return ""
		}
	}
	return p
}

// serverBodyOrder covers the routers that answer without any location: the default server (an
// unknown Host on http, and on https when the fixture has TLS), the https redirect of a route, and
// the per-route catch-all for a path no location matches. Each must answer 413 for a declared
// body over its limit on both proxies. It returns how many checks it ran.
func serverBodyOrder(r *runner, m routes, both bothFn) int {
	def := m.Defaults.MaxBodyBytes
	if def <= 0 {
		return 0
	}
	n, hasTLS := 0, false
	for _, rt := range m.Routes {
		hasTLS = hasTLS || rt.Listen.HTTPS
	}
	schemes := []string{"http"}
	if hasTLS {
		schemes = append(schemes, "https")
	}
	for _, sc := range schemes {
		for _, p := range []string{"/health", "/"} {
			both(fmt.Sprintf("unknown host %s POST %s over the default body limit", sc, p), sc, "unknown.invalid", "POST", p, def+1, 413)
			n++
		}
	}
	for _, rt := range m.Routes {
		if rt.HTTPToHTTPSRedirect && rt.Listen.HTTP {
			if limit, ok := redirLimit(rt, def); ok {
				both(rt.ID+" POST / on the https redirect over its body limit", "http", hostOf(rt), "POST", "/", limit+1, 413)
				n++
			}
		}
		if p := unmatchedPath(rt); p != "" && (rt.Listen.HTTP || rt.Listen.HTTPS) {
			both(rt.ID+" POST "+p+" (catch-all) over the default body limit", scheme(rt), hostOf(rt), "POST", p, def+1, 413)
			n++
		}
	}
	return n
}
