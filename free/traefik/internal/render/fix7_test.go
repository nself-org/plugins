package render

import (
	"strings"
	"testing"
)

// mwKinds lists the middleware kinds of a router spec in chain order (buffering, headers, ...).
func mwKinds(t *testing.T, out []byte, spec map[string]any) []string {
	t.Helper()
	defs := parse(t, out)["http"]["middlewares"]
	var kinds []string
	for _, n := range spec["middlewares"].([]any) {
		for k := range defs[n.(string)].(map[string]any) {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

func bufferingLimit(t *testing.T, out []byte, spec map[string]any) int {
	t.Helper()
	defs := parse(t, out)["http"]["middlewares"]
	for _, n := range spec["middlewares"].([]any) {
		if b, ok := defs[n.(string)].(map[string]any)["buffering"].(map[string]any); ok {
			return b["maxRequestBodyBytes"].(int)
		}
	}
	return -1
}

// nginx checks Content-Length against client_max_body_size before return, deny and limit_except,
// so on every non-proxied router the body limit is the first middleware of the chain.
func TestBodyLimitLeadsEveryNonProxiedRouter(t *testing.T) {
	root := "/acme"
	acme := loc(t, "proxy", "/.well-known/acme-challenge/", "prefix")
	acme.Upstream, acme.StaticRoot = nil, &root
	clearProxyOnly(&acme)
	acme.Methods, acme.MaxBodyBytes = []string{"GET"}, nil
	size := int64(1048576)
	deny := loc(t, "deny", "/internal", "exact")
	deny.MaxBodyBytes = &size
	ret := loc(t, "proxy", "/old/", "prefix")
	ret.Upstream, ret.Return = nil, redirectTo(301)
	clearProxyOnly(&ret)
	ret.MaxBodyBytes = &size
	status := loc(t, "proxy", "/gone/", "prefix")
	status.Upstream, status.Return = nil, redirectTo(301)
	status.Return.To = nil
	status.Return.Status, status.MaxBodyBytes = 410, nil
	clearProxyOnly(&status)
	r := mkRoute(t, "hand:o#1", []string{"o.example.test"}, loc(t, "proxy", "/", "prefix"), acme, deny, ret, status)
	r.SecurityHeaders = map[string]string{"X-Frame-Options": "DENY"}
	blockedPaths(t, &r, `\.env$`)
	out := mustRender(t, r)
	def := int(fixture(t, "plugin-modelled").Defaults.MaxBodyBytes)
	for _, c := range []struct {
		name, method, path string
		limit              int
		want               int // status the router answers; 0 = reaches the webroot
	}{
		{"deny_all", "POST", "/internal", int(size), 403},
		{"return redirect", "POST", "/old/x", int(size), 0},
		{"return status", "POST", "/gone/x", def, 410},
		{"blocked path", "POST", "/a/.env", def, 404},
		{"ACME non-GET (deny router)", "POST", "/.well-known/acme-challenge/tok", def, 403},
		{"ACME GET", "GET", "/.well-known/acme-challenge/tok", def, 0},
	} {
		_, spec := winner(t, out, "web", reqIn{host: "o.example.test", path: c.path, method: c.method})
		if spec == nil || spec["middlewares"] == nil {
			t.Errorf("%s: no router with middlewares wins", c.name)
			continue
		}
		kinds := mwKinds(t, out, spec)
		if len(kinds) == 0 || kinds[0] != "buffering" {
			t.Errorf("%s: the body limit must be the first middleware, chain is %s", c.name, strings.Join(kinds, ","))
		}
		if got := bufferingLimit(t, out, spec); got != c.limit {
			t.Errorf("%s: body limit %d, want %d", c.name, got, c.limit)
		}
		if got := answers(t, out, spec); got != c.want && c.want != 0 {
			t.Errorf("%s: answers %d, want %d", c.name, got, c.want)
		}
	}
}

// A zero limit is "no limit" in nginx: no buffering middleware on a non-proxied router either.
func TestZeroBodyLimitAddsNoBufferingToDeny(t *testing.T) {
	zero := int64(0)
	deny := loc(t, "deny", "/internal", "exact")
	deny.MaxBodyBytes = &zero
	out := mustRender(t, mkRoute(t, "hand:z#1", []string{"z.example.test"}, loc(t, "proxy", "/", "prefix"), deny))
	_, spec := winner(t, out, "web", reqIn{host: "z.example.test", path: "/internal", method: "POST"})
	for _, k := range mwKinds(t, out, spec) {
		if k == "buffering" {
			t.Errorf("a zero limit must not buffer, got chain with buffering")
		}
	}
}

// The unknown-host ACME router splits by method like a per-route one: GET and HEAD reach the
// webroot helper, every other method is a 403, both behind the body limit.
func TestDefaultACMESplitsByMethod(t *testing.T) {
	m := fixture(t, "plugin-modelled")
	m.Routes = nil
	out, err := Render(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	p := "/.well-known/acme-challenge/a/b"
	for _, c := range []struct {
		method string
		want   int
	}{{"GET", 0}, {"HEAD", 0}, {"POST", 403}, {"PUT", 403}, {"DELETE", 403}} {
		name, spec := winner(t, out, "web", reqIn{host: "unknown.invalid", path: p, method: c.method})
		if !strings.HasPrefix(name, "default-acme") {
			t.Errorf("%s: winner %s, want a default-acme router", c.method, name)
			continue
		}
		if got := answers(t, out, spec); got != c.want {
			t.Errorf("%s: answers %d, want %d", c.method, got, c.want)
		}
		if kinds := mwKinds(t, out, spec); len(kinds) == 0 || kinds[0] != "buffering" {
			t.Errorf("%s: body limit must lead the chain, got %v", c.method, kinds)
		}
	}
}
