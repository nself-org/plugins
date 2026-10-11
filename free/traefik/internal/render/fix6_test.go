package render

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// Fix round 6 (second opinion r5 must-fix 1 and should-fix 2, Opus r4 S1): a server name with a
// trailing dot or an empty label is refused, a location that sets two kinds or a proxy-only
// field on a non-upstream kind is refused, and the ACME webroot keeps its methods.

// A trailing dot names one host to Traefik and a different one to nginx; a deny_all route and an
// upstream route for the twin names would tie and the upstream could win.
func TestTrailingDotServerNameIsRefused(t *testing.T) {
	deny := mkRoute(t, "hand:a#1", []string{"a.example.test"}, loc(t, "deny", "/", "prefix"))
	up := mkRoute(t, "hand:b#1", []string{"a.example.test."}, loc(t, "proxy", "/", "prefix"))
	if _, err := renderRoutes(t, deny, up); err == nil || !strings.Contains(err.Error(), "hand:b#1") || !strings.Contains(err.Error(), "server_names") {
		t.Errorf("the trailing-dot twin of a denied host must be refused naming route and server_names, got %v", err)
	}
	for _, n := range []string{"a.example.test.", "a..example.test", "*.x.", ".x.", "*.a..x", ".a..x", "a.", "", "a-"} {
		r := mkRoute(t, "hand:n#1", []string{n}, loc(t, "proxy", "/", "prefix"))
		if _, err := renderRoutes(t, r); err == nil || !strings.Contains(err.Error(), "server_names") {
			t.Errorf("server name %q: want a server_names refusal, got %v", n, err)
		}
	}
	for _, n := range []string{"a.example.test", "*.example.test", ".example.test", "a-b.example.test", "x"} {
		if _, err := renderRoutes(t, mkRoute(t, "hand:n#1", []string{n}, loc(t, "proxy", "/", "prefix"))); err != nil {
			t.Errorf("server name %q must stay accepted, got %v", n, err)
		}
	}
}

// Every field is rendered or refused: a location with two kinds, or a field that only an upstream
// location renders, is refused instead of dropped.
func TestLocationKindsAndProxyOnlyFieldsAreRefused(t *testing.T) {
	root := "/acme"
	acme := func() contract.Location {
		l := loc(t, "proxy", "/.well-known/acme-challenge/", "prefix")
		l.Upstream, l.StaticRoot = nil, &root
		clearProxyOnly(&l)
		return l
	}
	deny := func() contract.Location { l := loc(t, "deny", "/x", "prefix"); clearProxyOnly(&l); return l }
	ret := func() contract.Location {
		l := loc(t, "proxy", "/r", "prefix")
		l.Upstream = nil
		clearProxyOnly(&l)
		l.Return = redirectTo(301)
		return l
	}
	secs := 5
	for _, c := range []struct {
		name, want string
		mut        func() contract.Location
	}{
		{"static_root and deny_all", "more than one", func() contract.Location { l := acme(); l.DenyAll = true; l.Methods = []string{"GET"}; return l }},
		{"static_root and upstream", "more than one", func() contract.Location { l := acme(); l.Upstream = loc(t, "proxy", "/", "prefix").Upstream; return l }},
		{"deny_all and return", "more than one", func() contract.Location { l := deny(); l.Return = redirectTo(301); return l }},
		{"deny_all and upstream", "more than one", func() contract.Location { l := deny(); l.Upstream = loc(t, "proxy", "/", "prefix").Upstream; return l }},
		{"return and upstream", "more than one", func() contract.Location { l := ret(); l.Upstream = loc(t, "proxy", "/", "prefix").Upstream; return l }},
		{"deny_all methods", ".methods", func() contract.Location { l := deny(); l.Methods = []string{"GET"}; return l }},
		{"return methods", ".methods", func() contract.Location { l := ret(); l.Methods = []string{"GET"}; return l }},
		{"deny_all health_probe", ".health_probe", func() contract.Location { l := deny(); l.HealthProbe = true; return l }},
		{"return health_probe", ".health_probe", func() contract.Location { l := ret(); l.HealthProbe = true; return l }},
		{"static_root health_probe", ".health_probe", func() contract.Location { l := acme(); l.HealthProbe = true; return l }},
		{"deny_all rate_limit", ".rate_limit", func() contract.Location {
			l := deny()
			l.RateLimit = &struct {
				Zone    string `json:"zone"`
				Burst   int    `json:"burst"`
				Nodelay bool   `json:"nodelay"`
			}{"api", 5, true}
			return l
		}},
		{"return timeouts", ".timeouts", func() contract.Location { l := ret(); l.Timeouts.ReadS = &secs; return l }},
		{"static_root timeouts", ".timeouts", func() contract.Location { l := acme(); l.Timeouts.ConnectS = &secs; return l }},
		{"deny_all websocket", ".websocket", func() contract.Location { l := deny(); l.WebSocket = true; return l }},
		{"static_root websocket", ".websocket", func() contract.Location { l := acme(); l.WebSocket = true; return l }},
	} {
		l := c.mut()
		_, err := renderRoutes(t, mkRoute(t, "hand:k#1", []string{"k.example.test"}, loc(t, "proxy", "/", "prefix"), l))
		if err == nil || !strings.Contains(err.Error(), "hand:k#1") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want a refusal naming the route and %q, got %v", c.name, c.want, err)
		}
	}
}

// What the CLI emits stays accepted: the ACME webroot with methods ["GET"], and max_body_bytes on
// deny_all and return locations. The webroot keeps limit_except GET: other methods get a 403.
func TestCLIEmittedNonUpstreamCombinationsRender(t *testing.T) {
	root := "/acme"
	acme := loc(t, "proxy", "/.well-known/acme-challenge/", "prefix")
	acme.Upstream, acme.StaticRoot, acme.Methods = nil, &root, []string{"GET"}
	clearProxyOnly(&acme)
	acme.Methods = []string{"GET"}
	size := int64(1048576)
	deny := loc(t, "deny", "/internal", "exact")
	deny.MaxBodyBytes = &size
	ret := loc(t, "proxy", "/old/", "prefix")
	ret.Upstream, ret.Return = nil, redirectTo(301)
	clearProxyOnly(&ret)
	ret.MaxBodyBytes = &size
	out, err := renderRoutes(t, mkRoute(t, "hand:c#1", []string{"c.example.test"}, loc(t, "proxy", "/", "prefix"), acme, deny, ret))
	if err != nil {
		t.Fatalf("CLI-shaped locations refused: %v", err)
	}
	p := "/.well-known/acme-challenge/tok"
	for _, c := range []struct {
		method string
		deny   bool
	}{{"GET", false}, {"HEAD", false}, {"POST", true}, {"PUT", true}} {
		_, spec := winner(t, out, "web", reqIn{host: "c.example.test", path: p, method: c.method})
		got := 0
		if spec["middlewares"] != nil {
			got = answers(t, out, spec)
		}
		if (got == 403) != c.deny {
			t.Errorf("%s %s: answers %d, want 403=%v", c.method, p, got, c.deny)
		}
	}
}
