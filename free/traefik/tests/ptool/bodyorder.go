package main

import "fmt"

// bodyOrder: nginx compares Content-Length with client_max_body_size in its find-config phase,
// before return (rewrite) and before deny or limit_except (access). A request that declares too
// large a body is a 413 on every location, including the ones that never read a body. Traefik
// gets the same answer from a buffering middleware placed first on those routers.
// Accepted difference (wiki): a chunked request with no Content-Length is not measured by nginx
// on a location that answers without reading the body (deny_all, return); it is not tested here.
func bodyOrder(r *runner, m routes) {
	both := func(label, scheme, host, method, path string, n int64, want int) {
		a, ea := declared(r.n, scheme, host, method, path, n)
		b, eb := declared(r.t, scheme, host, method, path, n)
		if ea != nil || eb != nil || a != want || b != want {
			r.fail(label, fmt.Sprintf("nginx=%d (%v) traefik=%d (%v), want %d", a, ea, b, eb, want))
			return
		}
		r.ok(fmt.Sprintf("%s: %d on both", label, want))
	}
	def := m.Defaults.MaxBodyBytes
	n := 0
	for _, rt := range m.Routes {
		if !rt.Listen.HTTP && !rt.Listen.HTTPS {
			continue
		}
		sc, host := scheme(rt), hostOf(rt)
		for _, l := range rt.Locations {
			if l.Upstream != nil || l.StaticRoot != nil || (!l.DenyAll && l.Return == nil) {
				continue
			}
			limit := def
			if l.MaxBodyBytes != nil {
				limit = *l.MaxBodyBytes
			}
			if limit <= 0 {
				continue
			}
			kind := "return"
			if l.DenyAll {
				kind = "deny_all"
			}
			both(fmt.Sprintf("%s POST %s %s over its body limit", rt.ID, kind, sample(l.Path, l.Match)), sc, host, "POST", sample(l.Path, l.Match), limit+1, 413)
			n++
		}
		if len(rt.BlockedPaths) > 0 && def > 0 {
			both(rt.ID+" POST blocked /.env over the default body limit", sc, host, "POST", "/.env", def+1, 413)
			n++
		}
		if rt.Listen.HTTP && hasACME(rt) && def > 0 {
			both(rt.ID+" POST ACME over the default body limit", "http", host, "POST", "/.well-known/acme-challenge/parity-token", def+1, 413)
			n++
		}
	}
	n += serverBodyOrder(r, m, both)
	if n == 0 {
		r.fail("body limit order", "no deny_all, return, blocked or ACME location in the fixture")
	}
}

// acmeMethods: a method outside GET and HEAD on the ACME prefix is a 403 whatever the path, for a
// known Host (per-route location) and for an unknown one (default server).
func acmeMethods(r *runner, m routes) {
	paths := []string{"/.well-known/acme-challenge/a/b", "/.well-known/acme-challenge/parity-token"}
	check := func(label, host string) {
		for _, p := range paths {
			a, ea := declared(r.n, "http", host, "POST", p, 0)
			b, eb := declared(r.t, "http", host, "POST", p, 0)
			if ea != nil || eb != nil || a != 403 || b != 403 {
				r.fail(label+" POST "+p, fmt.Sprintf("nginx=%d (%v) traefik=%d (%v), want 403", a, ea, b, eb))
			} else {
				r.ok(label + " POST " + p + ": 403 on both")
			}
		}
	}
	n := 0
	for _, rt := range m.Routes {
		if rt.Listen.HTTP && hasACME(rt) {
			check(rt.ID+" ACME known host", hostOf(rt))
			n++
			break
		}
	}
	if m.DefaultServer.HTTP.ACMEWebroot != nil {
		check("unknown host ACME", "unknown.invalid")
		n++
	}
	if n == 0 {
		r.fail("ACME methods", "no ACME location or default-server webroot in the fixture")
	}
}
