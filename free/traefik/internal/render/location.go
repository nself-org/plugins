package render

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// Priority classes follow nginx location selection: exact beats ^~ prefix
// (the ACME webroot) beats regex (blocked paths) beats the longest prefix.
// Router priorities are doubled so a method-restricted router can sit at +1.
const (
	classPrefix  = 1000
	classBlocked = 10000
	classStatic  = 20000
	classExact   = 30000
	acmePrefix   = "/.well-known/acme-challenge/"

	// maxBlocked is how many blocked_paths fit between the blocked class and the static class
	// (priorities classBlocked*2+1 .. classBlocked*2+n, each below classStatic*2).
	maxBlocked = (classStatic-classBlocked)*2 - 1
)

// classCeil is the first priority the next class owns. A location of class c must rank
// strictly below it (method split included), or it would outrank a class nginx tries first:
// an over-long prefix would otherwise beat the blocked-path regexes.
func classCeil(class int) int {
	switch class {
	case classPrefix:
		return classBlocked * 2
	case classBlocked:
		return classStatic * 2
	case classStatic:
		return classExact * 2
	}
	return maxLocPri + 1
}

func (b *builder) location(r contract.Route, i int, l contract.Location, host string, ls []listener, sec string) {
	field := fmt.Sprintf("locations[%d](%s)", i, l.Path)
	b.noLog = !l.AccessLog
	defer func() { b.noLog = false }()
	if strings.ContainsAny(l.Path, "`") || !strings.HasPrefix(l.Path, "/") || (l.Match != "prefix" && l.Match != "exact") {
		b.refuse(r.ID, field, "path or match is not expressible")
		return
	}
	class, rule := classPrefix, host+" && PathPrefix(`"+l.Path+"`)"
	if l.Match == "exact" {
		class, rule = classExact, host+" && Path(`"+l.Path+"`)"
	}
	// A priority at the next class would let this location outrank a class nginx tries first
	// (blocked_paths for a prefix, the next host tier for an exact path).
	if (class+len(l.Path))*2+1 >= classCeil(class) {
		b.refuse(r.ID, field, fmt.Sprintf("path of %d characters is too long to rank below the next priority class", len(l.Path)))
		return
	}
	if !b.contractExtras(r, l, field) {
		return
	}
	var mws []string
	add := func(n string) {
		if n != "" {
			mws = append(mws, n)
		}
	}
	if !b.oneKind(r, l, field) {
		return
	}
	svc, buf := "", ""
	if l.Upstream == nil {
		// nginx compares Content-Length with client_max_body_size in its find-config phase, before
		// return (rewrite) and before deny or limit_except (access): an oversized request is a 413
		// on every location, so the limit is the first middleware of every non-proxied router.
		limit, ok := b.bodyLimit(r, l, field)
		if !ok {
			return
		}
		buf = b.bufferIf(limit)
		add(buf)
	}
	switch {
	case l.StaticRoot != nil:
		if l.Path != acmePrefix || l.Match != "prefix" {
			b.refuse(r.ID, field+".static_root", "only the ACME challenge prefix is served from a webroot")
			return
		}
		class, svc = classStatic, b.responder()
		add(sec)
	case l.DenyAll:
		add(sec)
		add(b.mw("ipAllowList", map[string]any{"sourceRange": []string{"0.0.0.0/32"}}))
		svc = b.responder()
		add(b.respondMW(403))
	case l.Return != nil:
		add(sec)
		if l.Return.To == nil {
			if !returnStatusOK(l.Return.Status) {
				b.refuse(r.ID, field+".return.status", fmt.Sprintf("%d: only 200-599 except 204, 205, 304 and 444 can be answered with a fixed status", l.Return.Status))
				return
			}
			add(b.respondMW(l.Return.Status))
		} else if *l.Return.To == "https://$host$request_uri" {
			add(b.redirectMW(r.ID, field+".return", l.Return.Status))
		} else {
			b.refuse(r.ID, field+".return.to", fmt.Sprintf("target %q is not https://$host$request_uri", *l.Return.To))
			return
		}
		svc = b.responder()
	case l.Upstream != nil:
		if !b.proxied(r, i, l, field, class, rule, host, ls, sec) {
			return
		}
		return
	default:
		b.refuse(r.ID, field, "location has no upstream, return, static_root or deny_all")
		return
	}
	name, pri := fmt.Sprintf("%s-l%d", b.pfx, i), (class+len(l.Path))*2
	for _, ln := range ls {
		if l.StaticRoot != nil && len(l.Methods) > 0 {
			// The CLI emits the ACME webroot with methods ["GET"] (limit_except GET): other methods are a 403.
			if !b.methodSplit(r.ID, field, name, rule, pri, ln, mws, svc, buf, sec, l.Methods) {
				return
			}
			continue
		}
		b.addRouter(name, rule, pri, ln, mws, svc)
	}
}

// proxied renders an upstream location, splitting it by method when methods is set.
func (b *builder) proxied(r contract.Route, i int, l contract.Location, field string, class int, rule, host string, ls []listener, sec string) bool {
	var mws []string
	add := func(n string) {
		if n != "" {
			mws = append(mws, n)
		}
	}
	add(sec)
	add(b.compressMW())
	if lr := l.RateLimit; lr != nil {
		add(b.rateMW(r.ID, field+".rate_limit", lr.Zone, lr.Burst, lr.Nodelay))
	} else if rl := r.RateLimit; rl != nil && l.Path == "/" && l.Match == "prefix" {
		add(b.rateMW(r.ID, "rate_limit", rl.Zone, rl.Burst, true))
	}
	if rl := r.RateLimit; rl != nil && rl.ConnLimit < 0 {
		b.refuse(r.ID, "rate_limit.conn_limit", fmt.Sprintf("%d is negative", rl.ConnLimit))
		return false
	}
	if rl := r.RateLimit; rl != nil && rl.ConnLimit > 0 && l.Path == "/" && l.Match == "prefix" {
		add(b.connMW(rl.ConnLimit))
	}
	limit, ok := b.bodyLimit(r, l, field)
	if !ok {
		return false
	}
	buf := b.bufferIf(limit)
	add(buf)
	add(b.reqHeaders(r.ID, field, l))
	passHost := l.ForwardedHeaders || l.HeadersSet["Host"] == "$host"
	svc := b.service(r.ID, field, l, passHost)
	if svc == "" {
		return false
	}
	pri := (class + len(l.Path)) * 2
	name := fmt.Sprintf("%s-l%d", b.pfx, i)
	// nginx forces GET on a health probe and drops the body; Traefik cannot, so only GET (and
	// HEAD) pass and every other method is a 403 (an accepted difference, in the wiki). A caller's
	// subset of GET/HEAD is kept: limit_except HEAD admits HEAD only, so HEAD alone must not add GET.
	methods := l.Methods
	if l.HealthProbe {
		for _, m := range methods {
			if m != "GET" && m != "HEAD" {
				b.refuse(r.ID, field+".methods", fmt.Sprintf("health_probe with method %q: nginx would still send GET upstream", m))
				return false
			}
		}
		if len(methods) == 0 {
			methods = []string{"GET"}
		}
	}
	for _, ln := range ls {
		if len(methods) == 0 {
			b.addRouter(name, rule, pri, ln, mws, svc)
			continue
		}
		if !b.methodSplit(r.ID, field, name, rule, pri, ln, mws, svc, buf, sec, methods) {
			return false
		}
	}
	return true
}

// blocked renders blocked_paths: nginx regex locations that answer a fixed status.
// nginx takes the first matching regex in file order, so the priority falls with the index.
func (b *builder) blocked(r contract.Route, host string, ls []listener, sec string) {
	if len(r.BlockedPaths) > maxBlocked {
		b.refuse(r.ID, "blocked_paths", fmt.Sprintf("%d entries; at most %d keep their file order below the static class", len(r.BlockedPaths), maxBlocked))
		return
	}
	for i, bp := range r.BlockedPaths {
		field := fmt.Sprintf("blocked_paths[%d]", i)
		if _, err := regexp.Compile(bp.Regex); err != nil || strings.Contains(bp.Regex, "`") {
			b.refuse(r.ID, field+".regex", "not a RE2 expression Traefik can match")
			continue
		}
		if bp.Status < 100 || bp.Status > 599 {
			b.refuse(r.ID, field+".status", fmt.Sprintf("%d", bp.Status))
			continue
		}
		// The body limit leads: nginx answers 413 on Content-Length before a regex location's return.
		var mws []string
		if buf := b.bufferIf(b.m.Defaults.MaxBodyBytes); buf != "" {
			mws = append(mws, buf)
		}
		if sec != "" {
			mws = append(mws, sec)
		}
		mws = append(mws, b.respondMW(bp.Status))
		for _, ln := range ls {
			b.addRouter(fmt.Sprintf("%s-b%d", b.pfx, i), host+" && PathRegexp(`"+bp.Regex+"`)", classBlocked*2+len(r.BlockedPaths)-i, ln, mws, b.responder())
		}
	}
}
