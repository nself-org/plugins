package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// Fix round 5 (second opinion SF1 to SF5, Opus r3 S1, S2): priority classes, blocked-path order,
// security headers on the server's own answers, the redirect status, trailing-dot hosts, raw names.

func blockedPaths(t *testing.T, r *contract.Route, regs ...string) {
	t.Helper()
	r.BlockedPaths = nil
	for i, rx := range regs {
		st := 403
		if i == 0 {
			st = 404 // the first entry answers 404, every later one 403
		}
		r.BlockedPaths = append(r.BlockedPaths, struct {
			Regex  string `json:"regex"`
			Status int    `json:"status"`
		}{rx, st})
	}
}

// hasHeadersMW reports whether the router spec carries a headers middleware with response headers.
func hasHeadersMW(t *testing.T, out []byte, spec map[string]any) bool {
	t.Helper()
	mws := parse(t, out)["http"]["middlewares"]
	for _, n := range spec["middlewares"].([]any) {
		if h, ok := mws[n.(string)].(map[string]any)["headers"].(map[string]any); ok && h["customResponseHeaders"] != nil {
			return true
		}
	}
	return false
}

func redirectTo(status int) *struct {
	Status int     `json:"status"`
	To     *string `json:"to"`
} {
	to := "https://$host$request_uri"
	return &struct {
		Status int     `json:"status"`
		To     *string `json:"to"`
	}{status, &to}
}

// SF4: a prefix location long enough to rank at or above blocked_paths is refused; the longest
// accepted one still loses to the blocked regex.
func TestOverLongPrefixCannotOutrankBlockedPaths(t *testing.T) {
	build := func(n int) contract.Route {
		r := mkRoute(t, "hand:p#1", []string{"p.example.test"}, loc(t, "proxy", "/", "prefix"), loc(t, "proxy", "/"+strings.Repeat("a", n), "prefix"))
		blockedPaths(t, &r, `\.env`)
		return r
	}
	if _, err := renderRoutes(t, build(9000)); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("a 9001-character prefix must be refused, got %v", err)
	}
	for _, n := range []int{8999, 9500, 20000} {
		if _, err := renderRoutes(t, build(n)); err == nil {
			t.Errorf("prefix of %d characters ranks at or above blocked_paths and must be refused", n)
		}
	}
	out := mustRender(t, build(8997))
	name, spec := winner(t, out, "web", reqIn{host: "p.example.test", path: "/" + strings.Repeat("a", 8997) + "/.env"})
	if !strings.Contains(name, "-b0-") || answers(t, out, spec) != 404 {
		t.Errorf("the longest accepted prefix must lose to blocked \\.env, got %s answering %d", name, answers(t, out, spec))
	}
}

// SF1: blocked_paths keep the nginx file order (the first matching regex wins).
func TestBlockedPathsKeepFileOrder(t *testing.T) {
	for _, regs := range [][]string{{`\.env`, `^/x`}, {`^/x`, `\.env`}} {
		r := mkRoute(t, "hand:b#1", []string{"b.example.test"}, loc(t, "proxy", "/", "prefix"))
		blockedPaths(t, &r, regs...)
		out := mustRender(t, r)
		name, spec := winner(t, out, "web", reqIn{host: "b.example.test", path: "/x.env"})
		if got := answers(t, out, spec); got != 404 {
			t.Errorf("%v: %s answers %d, want the first regex (404)", regs, name, got)
		}
		seen := map[int]bool{}
		for _, spec := range parse(t, out)["http"]["routers"] {
			s := spec.(map[string]any)
			if strings.Contains(s["rule"].(string), "PathRegexp") {
				p := s["priority"].(int) - exactTier
				if seen[p] {
					t.Errorf("%v: two blocked routers share priority %d", regs, p)
				}
				seen[p] = true
			}
		}
	}
	// the count is capped below the static class, and the cap itself still renders
	big := mkRoute(t, "hand:big#1", []string{"big.example.test"}, loc(t, "proxy", "/", "prefix"))
	regs := make([]string, maxBlocked+1)
	for i := range regs {
		regs[i] = fmt.Sprintf("^/z%d$", i)
	}
	blockedPaths(t, &big, regs...)
	if _, err := renderRoutes(t, big); err == nil || !strings.Contains(err.Error(), "blocked_paths") {
		t.Errorf("%d blocked paths must be refused, got %v", len(regs), err)
	}
	blockedPaths(t, &big, regs[:maxBlocked]...)
	out := mustRender(t, big)
	if !strings.Contains(string(out), fmt.Sprintf("priority: %d\n", exactTier+classStatic*2-1)) {
		t.Error("the first of maxBlocked blocked paths must sit one below the static class")
	}
}

// SF2 and SF3: the catch-all 404 and the server-level redirect carry the security headers, and the
// redirect answers with the status of the "/" location that returns it.
func TestCatchAllAndRedirectCarrySecurityHeaders(t *testing.T) {
	r := mkRoute(t, "hand:a#1", []string{"a.example.test"}, loc(t, "proxy", "/api/", "prefix"))
	r.SecurityHeaders = map[string]string{"X-Frame-Options": "DENY"}
	out := mustRender(t, r)
	name, spec := winner(t, out, "web", reqIn{host: "a.example.test", path: "/other"})
	if !strings.Contains(name, "-catchall-") || answers(t, out, spec) != 404 || !hasHeadersMW(t, out, spec) {
		t.Errorf("/other must be a 404 with the security headers, got %s (%d, headers %v)", name, answers(t, out, spec), hasHeadersMW(t, out, spec))
	}
	r.HTTPToHTTPSRedirect = true
	out = mustRender(t, r)
	name, spec = winner(t, out, "web", reqIn{host: "a.example.test", path: "/api/x"})
	if !strings.HasSuffix(name, "-redirect-web") || !hasHeadersMW(t, out, spec) {
		t.Errorf("the redirect must carry the security headers, got %s (headers %v)", name, hasHeadersMW(t, out, spec))
	}
}

func TestRedirectStatusFollowsTheRootLocation(t *testing.T) {
	for _, c := range []struct {
		status    int
		permanent bool
		ok        bool
	}{{301, true, true}, {302, false, true}, {307, false, false}, {308, false, false}} {
		root := loc(t, "proxy", "/", "prefix")
		root.Upstream, root.Return = nil, redirectTo(c.status)
		r := mkRoute(t, "hand:r#1", []string{"r.example.test"}, root)
		r.HTTPToHTTPSRedirect = true
		out, err := renderRoutes(t, r)
		if (err == nil) != c.ok {
			t.Errorf("status %d: accepted=%v, error %v", c.status, err == nil, err)
			continue
		}
		if !c.ok {
			continue
		}
		name, spec := winner(t, out, "web", reqIn{host: "r.example.test", path: "/x"})
		mws := parse(t, out)["http"]["middlewares"]
		got, found := false, false
		for _, n := range spec["middlewares"].([]any) {
			if rr, ok := mws[n.(string)].(map[string]any)["redirectRegex"].(map[string]any); ok {
				got, found = rr["permanent"].(bool), true
			}
		}
		if !strings.HasSuffix(name, "-redirect-web") || !found || got != c.permanent {
			t.Errorf("status %d: %s redirect permanent=%v (found %v), want %v", c.status, name, got, found, c.permanent)
		}
	}
}

// Opus S2 and SF5: a wildcard rule accepts an absolute host name with a trailing dot.
func TestWildcardHostRegexpAcceptsTrailingDot(t *testing.T) {
	for _, n := range []string{"*.example.test", ".example.test"} {
		r := mkRoute(t, "hand:w#1", []string{n}, loc(t, "proxy", "/", "prefix"))
		out := mustRender(t, r)
		for _, h := range []string{"x.example.test", "x.example.test."} {
			if name, _ := winner(t, out, "web", reqIn{host: h, path: "/"}); owner(name, r.ID) != r.ID {
				t.Errorf("%s: Host %q must reach the route, got %s", n, h, name)
			}
		}
		if name, _ := winner(t, out, "web", reqIn{host: "x.example.test.evil", path: "/"}); owner(name, r.ID) == r.ID {
			t.Errorf("%s: x.example.test.evil must not match", n)
		}
	}
}

// Opus S1: the raw name is validated, so a name that only becomes ASCII by lowercasing is refused.
func TestNonASCIIServerNamesAreRefusedBeforeLowercasing(t *testing.T) {
	for _, n := range []string{"K.example.test", "*.Kelvin.test", ".Kelvin.test", "aK.example.test"} {
		r := mkRoute(t, "hand:k#1", []string{n}, loc(t, "proxy", "/", "prefix"))
		if _, err := renderRoutes(t, r); err == nil || !strings.Contains(err.Error(), "not a plain") {
			t.Errorf("%+q must be refused, got %v", n, err)
		}
	}
}
