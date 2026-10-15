package render

// location_split: method routing, body-size limits, and validation helpers for location rules.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// oneKind refuses a location the renderer would otherwise render only in part: nginx applies
// every directive of a location (an access rule before the content handler), the switch in
// location picks one kind. More than one of static_root, static_file, deny_all, return and upstream is
// refused, and so is a field that has no meaning on a location that is not an upstream.
// max_body_bytes is allowed: the CLI emits it on deny_all and return locations, and nginx
// checks it against Content-Length before the deny or return runs, so location() renders it.
func (b *builder) oneKind(r contract.Route, l contract.Location, field string) bool {
	var kinds []string
	for _, k := range []struct {
		name string
		set  bool
	}{{"static_root", l.StaticRoot != nil}, {"static_file", l.StaticFile != nil}, {"deny_all", l.DenyAll}, {"return", l.Return != nil}, {"upstream", l.Upstream != nil}} {
		if k.set {
			kinds = append(kinds, k.name)
		}
	}
	if len(kinds) > 1 {
		b.refuse(r.ID, field, fmt.Sprintf("more than one of static_root, static_file, deny_all, return and upstream is set (%s); nginx applies them together, this renderer would render one", strings.Join(kinds, ", ")))
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
// and every other method to a 403 at pri. buf is the body-limit middleware ("" when unlimited).
func (b *builder) methodSplit(id, field, name, rule string, pri int, ln listener, mws []string, svc, buf, sec string, methods []string) bool {
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
	// The 403 router answers after the same body-limit check as the allowed methods (nginx 413
	// comes before limit_except), so the limit leads its chain; sec follows.
	deny := []string{}
	if buf != "" {
		deny = append(deny, buf)
	}
	if sec != "" {
		deny = append(deny, sec)
	}
	b.addRouter(name+"-deny", rule, pri, ln, append(deny, b.respondMW(403)), b.responder())
	return true
}

// bodyLimit is the client_max_body_size of a location: its own max_body_bytes, else the default
// (0 is unlimited, as in nginx). A negative value is refused.
func (b *builder) bodyLimit(r contract.Route, l contract.Location, field string) (int64, bool) {
	limit := b.m.Defaults.MaxBodyBytes
	if l.MaxBodyBytes != nil {
		limit = *l.MaxBodyBytes
	}
	if limit < 0 {
		b.refuse(r.ID, field+".max_body_bytes", fmt.Sprintf("%d is negative", limit))
		return 0, false
	}
	return limit, true
}

// bufferIf returns the body-limit middleware for a positive limit, "" for none.
func (b *builder) bufferIf(limit int64) string {
	if limit > 0 {
		return b.bufferMW(limit)
	}
	return ""
}

// returnStatusOK reports whether the responder can answer a fixed status the way
// nginx `return <code>` does: 204, 205 and 304 carry no body, 444 closes the
// connection, and anything under 200 or over 599 is not a final status.
func returnStatusOK(st int) bool {
	return st >= 200 && st <= 599 && st != 204 && st != 205 && st != 304 && st != 444
}

var methodRE = regexp.MustCompile(`^[A-Z]+$`)

// contractExtras refuses the optional v1 fields this renderer cannot carry (fail closed). A
// static_file and headers_add are supported only for the core docs files.
// The cli's nginx uses a variable proxy_pass ("set $up_x <target>; proxy_pass $up_x;"),
// so an upstream path or query REPLACES THE WHOLE REQUEST URI (not the matched prefix), and
// Traefik's service URL cannot do that.
func (b *builder) contractExtras(r contract.Route, l contract.Location, field string) bool {
	ok := true
	if l.StaticFile != nil && !docsFile(r, l) {
		b.refuse(r.ID, field+".static_file", "only core:api-docs files under /opt/nself/.dist are served")
		ok = false
	}
	if len(l.HeadersAdd) > 0 && !docsFile(r, l) {
		b.refuse(r.ID, field+".headers_add", "response headers require a supported core:api-docs static file")
		ok = false
	}
	if l.Upstream != nil {
		if l.Upstream.Path != "" {
			b.refuse(r.ID, field+".upstream.path", fmt.Sprintf("upstream path %q: nginx sends this path instead of the request URI; traefik cannot; remove the path from INTERNAL_ROUTE_N_TARGET", l.Upstream.Path))
			ok = false
		}
		if l.Upstream.Query != nil {
			b.refuse(r.ID, field+".upstream.query", fmt.Sprintf("upstream query %q: nginx sends this query instead of the request query; traefik cannot; remove the query from INTERNAL_ROUTE_N_TARGET", *l.Upstream.Query))
			ok = false
		}
	}
	return ok
}
