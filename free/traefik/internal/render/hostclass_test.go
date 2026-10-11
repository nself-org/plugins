package render

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// Fixtures for the host-class tests: routes built from the plugin-modelled notes route.
func loc(t *testing.T, kind, path, match string) contract.Location {
	t.Helper()
	n := noteRoute(t, fixture(t, "plugin-modelled"))
	var l contract.Location
	switch kind {
	case "deny":
		l = n.Locations[0]
	case "proxy":
		l = n.Locations[1]
	default:
		t.Fatal("bad kind " + kind)
	}
	l.Path, l.Match = path, match
	return l
}

func mkRoute(t *testing.T, id string, names []string, locs ...contract.Location) contract.Route {
	t.Helper()
	r := noteRoute(t, fixture(t, "plugin-modelled"))
	r.ID, r.ServerNames, r.Locations = id, names, locs
	return r
}

func renderRoutes(t *testing.T, rs ...contract.Route) ([]byte, error) {
	t.Helper()
	m := fixture(t, "plugin-modelled")
	m.Routes = rs
	return Render(m, Options{})
}

func mustRender(t *testing.T, rs ...contract.Route) []byte {
	t.Helper()
	out, err := renderRoutes(t, rs...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func owner(name string, ids ...string) string {
	for _, id := range ids {
		if strings.HasPrefix(name, rname(id)+"-") {
			return id
		}
	}
	return "(" + name + ")"
}

// Opus round 2 N2 / lens 1: a route that mixes an exact and a wildcard name must keep its exact
// host above another wildcard route that has a longer path (nginx picks the server by name first).
func TestMixedExactAndWildcardNamesKeepTheExactHostFirst(t *testing.T) {
	a := mkRoute(t, "hand:a#1", []string{"a.example.test", "*.a.example.test"}, loc(t, "proxy", "/", "prefix"), loc(t, "deny", "/admin", "prefix"))
	b := mkRoute(t, "hand:b#1", []string{"*.example.test"}, loc(t, "proxy", "/", "prefix"), loc(t, "proxy", "/admin/x", "prefix"))
	out := mustRender(t, a, b)
	for _, host := range []string{"a.example.test", "x.a.example.test"} {
		name, spec := winner(t, out, "web", reqIn{host: host, path: "/admin/x"})
		if owner(name, a.ID, b.ID) != a.ID || answers(t, out, spec) != 403 {
			t.Errorf("%s/admin/x: want route A's deny (403), got %s answering %d", host, name, answers(t, out, spec))
		}
	}
	if name, _ := winner(t, out, "web", reqIn{host: "other.example.test", path: "/admin/x"}); owner(name, a.ID, b.ID) != b.ID {
		t.Errorf("other.example.test/admin/x must reach route B, got %s", name)
	}
}

// Opus N2 repro P1: exact name plus an unrelated wildcard in one route, deny_all on /internal/.
func TestMixedNamesRouteDeniesAgainstALongerWildcardPath(t *testing.T) {
	a := mkRoute(t, "hand:notes#1", []string{"notes.example.test", "*.notes-alt.test"}, loc(t, "proxy", "/", "prefix"), loc(t, "deny", "/internal/", "prefix"))
	w := mkRoute(t, "hand:wild#1", []string{"*.example.test"}, loc(t, "proxy", "/", "prefix"), loc(t, "proxy", "/internal/x", "prefix"))
	out := mustRender(t, a, w)
	name, spec := winner(t, out, "web", reqIn{host: "notes.example.test", path: "/internal/x"})
	if owner(name, a.ID, w.ID) != a.ID || answers(t, out, spec) != 403 {
		t.Fatalf("notes.example.test/internal/x must be denied by its own route, got %s (%d)", name, answers(t, out, spec))
	}
}

// Opus N2 repro P2 and second-opinion 2: nested wildcards rank by suffix length, not by path length.
func TestNestedWildcardsRankBySuffixLength(t *testing.T) {
	for _, outer := range []string{"*.corp.example.test", ".corp.example.test"} {
		a := mkRoute(t, "hand:corp#1", []string{outer}, loc(t, "proxy", "/", "prefix"), loc(t, "deny", "/internal/", "prefix"))
		w := mkRoute(t, "hand:wild#1", []string{"*.example.test"}, loc(t, "proxy", "/", "prefix"), loc(t, "proxy", "/internal/x", "prefix"))
		out := mustRender(t, a, w)
		name, spec := winner(t, out, "web", reqIn{host: "a.corp.example.test", path: "/internal/x"})
		if owner(name, a.ID, w.ID) != a.ID || answers(t, out, spec) != 403 {
			t.Errorf("%s: a.corp.example.test/internal/x must be denied by the longer suffix, got %s (%d)", outer, name, answers(t, out, spec))
		}
		if outer[0] == '.' { // the bare name belongs to the dot route as an exact name
			if name, _ := winner(t, out, "web", reqIn{host: "corp.example.test", path: "/internal/x"}); owner(name, a.ID, w.ID) != a.ID {
				t.Errorf("corp.example.test must be served by the dot route, got %s", name)
			}
		}
	}
}

// Opus N3: a wildcard route's https redirect must not outrank an exact host or its ACME challenge.
func TestWildcardRedirectDoesNotOutrankExactHosts(t *testing.T) {
	exact := mkRoute(t, "core:auth#1", []string{"auth.example.test"}, loc(t, "proxy", "/", "prefix"))
	acme := loc(t, "proxy", "/.well-known/acme-challenge/", "prefix")
	root := "/acme"
	acme.Upstream, acme.StaticRoot = nil, &root
	exact.Locations = append(exact.Locations, acme)
	wild := mkRoute(t, "hand:redir#1", []string{"*.example.test"}, loc(t, "proxy", "/", "prefix"))
	wild.HTTPToHTTPSRedirect = true
	out := mustRender(t, exact, wild)
	for _, p := range []string{"/", "/.well-known/acme-challenge/tok"} {
		name, _ := winner(t, out, "web", reqIn{host: "auth.example.test", path: p})
		if owner(name, exact.ID, wild.ID) != exact.ID {
			t.Errorf("auth.example.test%s must stay with its exact route, got %s", p, name)
		}
	}
	name, _ := winner(t, out, "web", reqIn{host: "other.example.test", path: "/x"})
	if !strings.HasSuffix(name, "-redirect-web") || owner(name, wild.ID) != wild.ID {
		t.Errorf("other hosts must still be redirected by the wildcard route, got %s", name)
	}
	// and the redirect still outranks its own route's locations, as a server-level return does
	if name, _ := winner(t, out, "web", reqIn{host: "other.example.test", path: "/"}); !strings.HasSuffix(name, "-redirect-web") {
		t.Errorf("the redirect must beat the wildcard route's own / location, got %s", name)
	}
}

// Opus N4 / lens 4: names compare and render in lower case; ".x" claims "x" and "*.x".
func TestServerNamesAreCaseInsensitive(t *testing.T) {
	a := mkRoute(t, "hand:a#1", []string{"a.example.test"}, loc(t, "proxy", "/", "prefix"))
	b := mkRoute(t, "hand:b#1", []string{"A.EXAMPLE.TEST"}, loc(t, "proxy", "/", "prefix"))
	if _, err := renderRoutes(t, a, b); err == nil || !strings.Contains(err.Error(), "already served by route hand:a#1") {
		t.Errorf("A.EXAMPLE.TEST must collide with a.example.test, got %v", err)
	}
	for _, c := range []struct{ x, y string }{{".dup.example.test", "*.dup.example.test"}, {"*.DUP.example.test", ".dup.example.test"}, {".dup.example.test", "dup.example.test"}} {
		a := mkRoute(t, "hand:a#1", []string{c.x}, loc(t, "proxy", "/", "prefix"))
		b := mkRoute(t, "hand:b#1", []string{c.y}, loc(t, "proxy", "/", "prefix"))
		if _, err := renderRoutes(t, a, b); err == nil || !strings.Contains(err.Error(), "already served by route") {
			t.Errorf("%s and %s name the same servers and must collide, got %v", c.x, c.y, err)
		}
	}
	out := mustRender(t, mkRoute(t, "hand:mixed#1", []string{"Mixed.Example.TEST", "*.Wild.Example.TEST"}, loc(t, "proxy", "/", "prefix")))
	for _, r := range parse(t, out)["http"]["routers"] {
		rule := r.(map[string]any)["rule"].(string)
		if strings.ContainsAny(rule, "MWET") { // the only upper-case letters left would come from the names
			t.Errorf("host rule keeps upper case: %s", rule)
		}
	}
	if name, _ := winner(t, out, "web", reqIn{host: "MIXED.example.test", path: "/"}); owner(name, "hand:mixed#1") != "hand:mixed#1" {
		t.Errorf("an upper-case Host header must reach the route, got %s", name)
	}
}

// Lens 3: an exact host without a "/" location answers 404 for other paths; it never falls
// through to a wildcard route or to the default server.
func TestExactHostWithoutRootLocationDoesNotFallThrough(t *testing.T) {
	a := mkRoute(t, "hand:a#1", []string{"a.example.test"}, loc(t, "proxy", "/api", "prefix"))
	w := mkRoute(t, "hand:w#1", []string{"*.example.test"}, loc(t, "proxy", "/", "prefix"))
	out := mustRender(t, a, w)
	name, spec := winner(t, out, "web", reqIn{host: "a.example.test", path: "/other"})
	if owner(name, a.ID, w.ID) != a.ID || answers(t, out, spec) != 404 {
		t.Fatalf("a.example.test/other must be a 404 from its own route, got %s (%d)", name, answers(t, out, spec))
	}
	if name, _ := winner(t, out, "web", reqIn{host: "a.example.test", path: "/api/x"}); !strings.Contains(name, "-l0-") {
		t.Errorf("a.example.test/api/x must reach its upstream location, got %s", name)
	}
}

// Lens 5: nginx forces GET on a health probe, Traefik cannot: only GET and HEAD pass, the rest is 403.
func TestHealthProbeAcceptsOnlyGetAndHead(t *testing.T) {
	hp := loc(t, "proxy", "/healthz", "exact")
	hp.HealthProbe, hp.Methods = true, []string{"GET"}
	out := mustRender(t, mkRoute(t, "hand:h#1", []string{"h.example.test"}, loc(t, "proxy", "/", "prefix"), hp))
	for m, want := range map[string]int{"GET": 0, "HEAD": 0, "POST": 403, "DELETE": 403, "PUT": 403} {
		name, spec := winner(t, out, "web", reqIn{host: "h.example.test", path: "/healthz", method: m})
		if got := answers(t, out, spec); got != want || (want == 0 && !strings.Contains(name, "-l1-")) {
			t.Errorf("%s /healthz: want %d (0 = upstream), got %d via %s", m, want, got, name)
		}
	}
	hp.Methods = nil // no methods listed: still GET and HEAD only
	out = mustRender(t, mkRoute(t, "hand:h#1", []string{"h.example.test"}, loc(t, "proxy", "/", "prefix"), hp))
	if _, spec := winner(t, out, "web", reqIn{host: "h.example.test", path: "/healthz", method: "POST"}); answers(t, out, spec) != 403 {
		t.Error("a health probe without a methods list must still refuse POST")
	}
	hp.Methods = []string{"POST"}
	if _, err := renderRoutes(t, mkRoute(t, "hand:h#1", []string{"h.example.test"}, loc(t, "proxy", "/", "prefix"), hp)); err == nil || !strings.Contains(err.Error(), "health_probe") {
		t.Errorf("a health probe restricted to POST must be refused, got %v", err)
	}
}

// Lens 6: return <status> accepts what the responder can answer.
func TestReturnStatusRange(t *testing.T) {
	for st, ok := range map[int]bool{0: false, 100: false, 199: false, 200: true, 204: false, 205: false, 304: false, 403: true, 410: true, 444: false, 503: true, 599: true, 600: false, -1: false} {
		l := loc(t, "proxy", "/gone", "prefix")
		l.Upstream = nil
		l.Return = &struct {
			Status int     `json:"status"`
			To     *string `json:"to"`
		}{Status: st}
		_, err := renderRoutes(t, mkRoute(t, "hand:r#1", []string{"r.example.test"}, loc(t, "proxy", "/", "prefix"), l))
		if ok && err != nil || !ok && (err == nil || !strings.Contains(err.Error(), "return.status")) {
			t.Errorf("return %d: accepted=%v, error %v", st, ok, err)
		}
	}
}

// Lens 7: a path long enough to cross the next host tier is refused, and the longest accepted one
// still ranks below the tier step.
func TestLongPathsAreRefusedBeforeCrossingATier(t *testing.T) {
	for _, c := range []struct {
		match string
		n     int
		ok    bool
	}{{"exact", 21000, false}, {"exact", 19000, true}, {"prefix", 60000, false}, {"prefix", 48000, true}, {"prefix", 4000, true}} {
		_, err := renderRoutes(t, mkRoute(t, "hand:p#1", []string{"p.example.test"}, loc(t, "proxy", "/", "prefix"), loc(t, "proxy", "/"+strings.Repeat("a", c.n), c.match)))
		if (err == nil) != c.ok || (err != nil && !strings.Contains(err.Error(), "too long")) {
			t.Errorf("%s path of %d characters: accepted=%v, error %v", c.match, c.n, c.ok, err)
		}
	}
	// the accepted maximum (exact, 19000) is below one tier step even with the method split's +1
	out := mustRender(t, mkRoute(t, "hand:p#1", []string{"p.example.test"}, loc(t, "proxy", "/"+strings.Repeat("a", 19000), "exact")))
	for _, r := range parse(t, out)["http"]["routers"] {
		spec := r.(map[string]any)
		if strings.Contains(spec["rule"].(string), "Host(`p.example.test`)") && spec["priority"].(int)-exactTier >= redirectPri {
			t.Errorf("router priority %v crosses the tier step", spec["priority"])
		}
	}
}
