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
)

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
	// A priority past the tier step would let this location outrank another route's host class.
	if (class+len(l.Path))*2+1 > maxLocPri {
		b.refuse(r.ID, field, fmt.Sprintf("path of %d characters is too long to rank below the next host tier", len(l.Path)))
		return
	}
	var mws []string
	add := func(n string) {
		if n != "" {
			mws = append(mws, n)
		}
	}
	svc := ""
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
	for _, ln := range ls {
		b.addRouter(fmt.Sprintf("%s-l%d", b.pfx, i), rule, (class+len(l.Path))*2, ln, mws, svc)
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
	limit := b.m.Defaults.MaxBodyBytes
	if l.MaxBodyBytes != nil {
		limit = *l.MaxBodyBytes
	}
	if limit < 0 {
		b.refuse(r.ID, field+".max_body_bytes", fmt.Sprintf("%d is negative", limit))
		return false
	}
	if limit > 0 {
		add(b.bufferMW(limit))
	}
	add(b.reqHeaders(r.ID, field, l))
	passHost := l.ForwardedHeaders || l.HeadersSet["Host"] == "$host"
	svc := b.service(r.ID, field, l, passHost)
	if svc == "" {
		return false
	}
	pri := (class + len(l.Path)) * 2
	name := fmt.Sprintf("%s-l%d", b.pfx, i)
	// nginx forces GET on a health probe and drops the body; Traefik cannot, so only GET (and
	// HEAD) pass and every other method is a 403 (an accepted difference, in the wiki).
	methods := l.Methods
	if l.HealthProbe {
		for _, m := range methods {
			if m != "GET" && m != "HEAD" {
				b.refuse(r.ID, field+".methods", fmt.Sprintf("health_probe with method %q: nginx would still send GET upstream", m))
				return false
			}
		}
		methods = []string{"GET"}
	}
	for _, ln := range ls {
		if len(methods) == 0 {
			b.addRouter(name, rule, pri, ln, mws, svc)
			continue
		}
		var ms []string
		for _, m := range methods {
			if !methodRE.MatchString(m) {
				b.refuse(r.ID, field+".methods", fmt.Sprintf("method %q", m))
				return false
			}
			ms = append(ms, "Method(`"+m+"`)")
			if m == "GET" {
				ms = append(ms, "Method(`HEAD`)")
			}
		}
		b.addRouter(name, rule+" && ("+strings.Join(ms, " || ")+")", pri+1, ln, mws, svc)
		deny := []string{}
		if sec != "" {
			deny = append(deny, sec)
		}
		b.addRouter(name+"-deny", rule, pri, ln, append(deny, b.respondMW(403)), b.responder())
	}
	return true
}

// returnStatusOK reports whether the responder can answer a fixed status the way
// nginx `return <code>` does: 204, 205 and 304 carry no body, 444 closes the
// connection, and anything under 200 or over 599 is not a final status.
func returnStatusOK(st int) bool {
	return st >= 200 && st <= 599 && st != 204 && st != 205 && st != 304 && st != 444
}

var methodRE = regexp.MustCompile(`^[A-Z]+$`)

// blocked renders blocked_paths: nginx regex locations that answer a fixed status.
func (b *builder) blocked(r contract.Route, host string, ls []listener, sec string) {
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
		var mws []string
		if sec != "" {
			mws = append(mws, sec)
		}
		mws = append(mws, b.respondMW(bp.Status))
		for _, ln := range ls {
			b.addRouter(fmt.Sprintf("%s-b%d", b.pfx, i), host+" && PathRegexp(`"+bp.Regex+"`)", classBlocked*2, ln, mws, b.responder())
		}
	}
}
