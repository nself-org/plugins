package render

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

var rateRE = regexp.MustCompile(`^([0-9]+)r/([sm])$`)

// rateMW converts an nginx limit_req zone plus burst into a Traefik rateLimit
// (token bucket; nginx nodelay admits 1+burst requests at once).
func (b *builder) rateMW(route, field, zone string, burst int, nodelay bool) string {
	z, ok := b.zones[zone]
	if !ok || z.Kind != "req" || z.Rate == nil {
		b.refuse(route, field, fmt.Sprintf("zone %q is not a modelled request zone with a rate", zone))
		return ""
	}
	if !nodelay {
		b.refuse(route, field, "limit_req without nodelay queues requests; Traefik cannot delay")
		return ""
	}
	m := rateRE.FindStringSubmatch(*z.Rate)
	if m == nil {
		b.refuse(route, field, fmt.Sprintf("zone rate %q is not <n>r/s or <n>r/m", *z.Rate))
		return ""
	}
	var avg int
	if _, err := fmt.Sscan(m[1], &avg); err != nil || avg <= 0 {
		b.refuse(route, field, fmt.Sprintf("zone rate %q is zero or unreadable; Traefik treats average 0 as no limit while nginx refuses to load it", *z.Rate))
		return ""
	}
	if burst < 0 {
		b.refuse(route, field, fmt.Sprintf("burst %d is negative", burst))
		return ""
	}
	period := "1s"
	if m[2] == "m" {
		period = "1m"
	}
	spec := map[string]any{"average": avg, "period": period, "burst": burst + 1}
	switch z.Key {
	case "$binary_remote_addr":
	case "$http_authorization", "$http_x_tenant_id":
		// nginx does not count a request whose key is empty; Traefik would put every request without
		// the header into one shared bucket, so one anonymous client could throttle all of them.
		b.refuse(route, field, fmt.Sprintf("zone key %q: requests without the header are unlimited on nginx but share one bucket on Traefik", z.Key))
		return ""
	default:
		b.refuse(route, field, fmt.Sprintf("zone key %q has no Traefik source criterion", z.Key))
		return ""
	}
	return b.mw("rateLimit", spec)
}

func (b *builder) connMW(n int) string {
	// limit_conn counts per client address ($binary_remote_addr). Traefik groups by Host unless a
	// sourceCriterion is set; depth 0 is the remote address (an empty ipStrategy fails to load).
	return b.mw("inFlightReq", map[string]any{"amount": n, "sourceCriterion": map[string]any{"ipStrategy": map[string]any{"depth": 0}}})
}

func (b *builder) bufferMW(limit int64) string {
	return b.mw("buffering", map[string]any{"maxRequestBodyBytes": limit})
}

// compressMW mirrors `gzip on` (nginx always adds text/html to gzip_types; default min length 20).
func (b *builder) compressMW() string {
	g := b.m.Defaults.Gzip
	if !g.Enabled {
		return ""
	}
	types := append([]string{"text/html"}, g.Types...)
	seen := map[string]bool{}
	var uniq []string
	for _, t := range types {
		if !seen[t] {
			seen[t] = true
			uniq = append(uniq, t)
		}
	}
	min := 20
	if g.MinLength != nil {
		min = *g.MinLength
	}
	return b.mw("compress", map[string]any{"encodings": []string{"gzip"}, "includedContentTypes": uniq, "minResponseBodyBytes": min})
}

var stdReq = map[string]string{"Host": "$host", "X-Real-IP": "$remote_addr", "X-Forwarded-For": "$proxy_add_x_forwarded_for",
	"X-Forwarded-Proto": "$scheme", "Upgrade": "$http_upgrade", "Connection": "$connection_upgrade"}

// reqHeaders maps headers_set: the standard forwarded and websocket headers are
// Traefik defaults; other literals become customRequestHeaders; variables are refused.
func (b *builder) reqHeaders(route, field string, l contract.Location) string {
	custom := map[string]string{}
	for k, v := range l.HeadersSet {
		if std, ok := stdReq[k]; ok && (v == std || (k == "Connection" && strings.EqualFold(v, "upgrade"))) {
			continue
		}
		if strings.Contains(v, "$") {
			b.refuse(route, field+".headers_set."+k, fmt.Sprintf("value %q holds an nginx variable", v))
			continue
		}
		if !headerOK(k, v) {
			b.refuse(route, field+".headers_set."+k, "header name is not a token or the value holds a control character")
			continue
		}
		custom[k] = v
	}
	if len(custom) == 0 {
		return ""
	}
	return b.mw("headers", map[string]any{"customRequestHeaders": custom})
}

const redirRE = `^https?://([^/:]+)(?::[0-9]+)?(.*)$`

// redirectMW is nginx `return 301|302 https://$host$request_uri` (host without port, original URI).
func (b *builder) redirectMW(route, field string, status int) string {
	if status != 301 && status != 302 {
		b.refuse(route, field, fmt.Sprintf("redirect status %d is not 301 or 302", status))
		return ""
	}
	return b.mw("redirectRegex", map[string]any{"regex": redirRE, "replacement": "https://${1}${2}", "permanent": status == 301})
}

// responder is the plugin-owned helper service (traefik-acme) that answers fixed
// statuses; no project upstream is ever reached through it.
func (b *builder) responder() string {
	b.services["responder"] = map[string]any{"loadBalancer": map[string]any{"servers": []any{map[string]any{"url": b.o.Helper}}, "passHostHeader": false}}
	return "responder"
}

// respondMW rewrites the request path to one of the helper's fixed answers.
func (b *builder) respondMW(status int) string {
	p := "/__nself/ok"
	if status != 0 {
		p = fmt.Sprintf("/__nself/status/%d", status)
	}
	return b.mw("replacePath", map[string]any{"path": p})
}
