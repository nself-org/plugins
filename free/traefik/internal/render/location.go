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
	var mws []string
	add := func(n string) {
		if n != "" {
			mws = append(mws, n)
		}
	}
	if !b.oneKind(r, l, field) {
		return
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
	name, pri := fmt.Sprintf("%s-l%d", b.pfx, i), (class+len(l.Path))*2
	for _, ln := range ls {
		if l.StaticRoot != nil && len(l.Methods) > 0 {
			// The CLI emits the ACME webroot with methods ["GET"] (limit_except GET): other methods are a 403.
			if !b.methodSplit(r.ID, field, name, rule, pri, ln, mws, svc, sec, l.Methods) {
				return
			}
			continue
		}
		b.addRouter(name, rule, pri, ln, mws, svc)
	}
}

// oneKind refuses a location the renderer would otherwise render only in part: nginx applies
// every directive of a location (an access rule before the content handler), the switch in
// location picks one kind. More than one of static_root, deny_all, return and upstream is
// refused, and so is a field that has no meaning on a location that is not an upstream.
// max_body_bytes is not refused: the CLI emits it on deny_all and return locations, where
// nginx answers before reading the body, so dropping it changes nothing a client sees.
func (b *builder) oneKind(r contract.Route, l contract.Location, field string) bool {
	var kinds []string
	for _, k := range []struct {
		name string
		set  bool
	}{{"static_root", l.StaticRoot != nil}, {"deny_all", l.DenyAll}, {"return", l.Return != nil}, {"upstream", l.Upstream != nil}} {
		if k.set {
			kinds = append(kinds, k.name)
		}
	}
	if len(kinds) > 1 {
		b.refuse(r.ID, field, fmt.Sprintf("more than one of static_root, deny_all, return and upstream is set (%s); nginx applies them together, this renderer would render one", strings.Join(kinds, ", ")))
		return false
	}
	if len(kinds) == 0 || l.Upstream != nil {
		return true
	}
	ok := true
	drop := func(f string, set bool) {
		if set {
			b.refuse(r.ID, field+"."+f, fmt.Sprintf("set on a %s location, where it would not be rendered", kinds[0]))
			ok = false
		}
	}
	drop("methods", len(l.Methods) > 0 && l.StaticRoot == nil)
	drop("health_probe", l.HealthProbe)
	drop("rate_limit", l.RateLimit != nil)
	drop("timeouts", l.Timeouts.ConnectS != nil || l.Timeouts.ReadS != nil || l.Timeouts.SendS != nil)
	drop("websocket", l.WebSocket)
	return ok
}

// methodSplit routes the listed methods (GET also admits HEAD, as nginx does) to svc at pri+1
// and every other method to a 403 at pri.
func (b *builder) methodSplit(id, field, name, rule string, pri int, ln listener, mws []string, svc, sec string, methods []string) bool {
	var ms []string
	for _, m := range methods {
		if !methodRE.MatchString(m) {
			b.refuse(id, field+".methods", fmt.Sprintf("method %q", m))
			return false
		}
		ms = append(ms, "Method(`"+m+"`)")
		if m == "GET" {
			ms = append(ms, "Method(`HEAD`)")
		}
	}
	b.addRouter(name, rule+" && ("+strings.Join(ms, " || ")+")", pri+1, ln, mws, svc)
	var deny []string
	if sec != "" {
		deny = append(deny, sec)
	}
	b.addRouter(name+"-deny", rule, pri, ln, append(deny, b.respondMW(403)), b.responder())
	return true
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
		if !b.methodSplit(r.ID, field, name, rule, pri, ln, mws, svc, sec, methods) {
			return false
		}
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
		var mws []string
		if sec != "" {
			mws = append(mws, sec)
		}
		mws = append(mws, b.respondMW(bp.Status))
		for _, ln := range ls {
			b.addRouter(fmt.Sprintf("%s-b%d", b.pfx, i), host+" && PathRegexp(`"+bp.Regex+"`)", classBlocked*2+len(r.BlockedPaths)-i, ln, mws, b.responder())
		}
	}
}
