package render

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// httpSection parses the http section of a rendered document (parse() cannot read the tls list).
func httpSection(t *testing.T, out []byte) map[string]map[string]any {
	t.Helper()
	var doc struct {
		HTTP map[string]map[string]any `yaml:"http"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.HTTP
}

// Fix round 8 (D-0397): nginx compares Content-Length with client_max_body_size in find-config,
// before any location's return, so a router that answers without an upstream leads with the body
// limit: the default server's root and health routers, each route's catch-all 404 and its https
// redirect (which takes the limit of its "/" return location).

// leadLimit is the maxRequestBodyBytes of the first middleware of a router spec, or -1 when the
// chain is empty or does not start with a buffering middleware.
func leadLimit(t *testing.T, out []byte, spec map[string]any) int {
	t.Helper()
	chain, _ := spec["middlewares"].([]any)
	if len(chain) == 0 {
		return -1
	}
	def, _ := httpSection(t, out)["middlewares"][chain[0].(string)].(map[string]any)
	if b, ok := def["buffering"].(map[string]any); ok {
		return b["maxRequestBodyBytes"].(int)
	}
	return -1
}

// bodyRouters lists the routers whose name says they answer without an upstream: the default
// server's root and health routers, every catch-all and every redirect.
func bodyRouters(t *testing.T, out []byte) map[string]map[string]any {
	t.Helper()
	got := map[string]map[string]any{}
	for name, v := range httpSection(t, out)["routers"] {
		if strings.HasPrefix(name, "default-root-") || strings.HasPrefix(name, "default-health-") ||
			strings.Contains(name, "-catchall-") || strings.Contains(name, "-redirect-") {
			got[name] = v.(map[string]any)
		}
	}
	return got
}

func TestBodyLimitLeadsDefaultServerAndCatchAll(t *testing.T) {
	for _, name := range []string{"http-full", "tls-full"} {
		out, err := renderFixture(t, name)
		if err != nil {
			t.Fatal(err)
		}
		want := int(fixture(t, name).Defaults.MaxBodyBytes)
		rs := bodyRouters(t, out)
		var roots, health, catch int
		for rn, spec := range rs {
			switch {
			case strings.HasPrefix(rn, "default-root-"):
				roots++
			case strings.HasPrefix(rn, "default-health-"):
				health++
			case strings.Contains(rn, "-catchall-"):
				catch++
			}
			if got := leadLimit(t, out, spec); got != want {
				t.Errorf("%s/%s: first middleware limit %d, want %d", name, rn, got, want)
			}
		}
		if roots == 0 || health == 0 || catch < len(fixture(t, name).Routes) {
			t.Errorf("%s: only %d root, %d health, %d catch-all routers checked", name, roots, health, catch)
		}
	}
	// tls-full has the HTTPS default server: its root and health routers on websecure lead too.
	out, _ := renderFixture(t, "tls-full")
	rs := bodyRouters(t, out)
	for _, n := range []string{"default-root-web", "default-root-tls", "default-health-web", "default-health-tls"} {
		if rs[n] == nil {
			t.Errorf("tls-full: router %s missing", n)
		}
	}
}

// The https redirect leads with the limit of the "/" return location, the default when that
// location sets none, and no buffering at all when the limit is 0 or the default is 0.
func TestRedirectBodyLimitFollowsTheReturnLocation(t *testing.T) {
	size := int64(1048576)
	zero := int64(0)
	def := int(fixture(t, "plugin-modelled").Defaults.MaxBodyBytes)
	for _, c := range []struct {
		name string
		own  *int64
		want int
	}{{"own limit", &size, int(size)}, {"default limit", nil, def}, {"own zero", &zero, -1}} {
		root := loc(t, "proxy", "/", "prefix")
		root.Upstream, root.Return, root.MaxBodyBytes = nil, redirectTo(301), c.own
		clearProxyOnly(&root)
		r := mkRoute(t, "hand:r#1", []string{"r.example.test"}, root)
		r.HTTPToHTTPSRedirect = true
		out := mustRender(t, r)
		rs := bodyRouters(t, out)
		var seen int
		for rn, spec := range rs {
			switch {
			case strings.Contains(rn, "-redirect-"):
				seen++
				if got := leadLimit(t, out, spec); got != c.want {
					t.Errorf("%s: redirect %s limit %d, want %d", c.name, rn, got, c.want)
				}
			case strings.Contains(rn, "-catchall-"):
				if got := leadLimit(t, out, spec); got != def {
					t.Errorf("%s: catch-all %s limit %d, want the default %d", c.name, rn, got, def)
				}
			}
		}
		if seen == 0 {
			t.Errorf("%s: no redirect router rendered", c.name)
		}
	}
	// no "/" return location at all: the redirect is a 301 under the default limit
	r := mkRoute(t, "hand:n#1", []string{"n.example.test"}, loc(t, "proxy", "/api/", "prefix"))
	r.HTTPToHTTPSRedirect = true
	out := mustRender(t, r)
	for rn, spec := range bodyRouters(t, out) {
		if strings.Contains(rn, "-redirect-") && leadLimit(t, out, spec) != def {
			t.Errorf("redirect without a return location: %s limit %d, want %d", rn, leadLimit(t, out, spec), def)
		}
	}
}

// defaults.max_body_bytes 0 is "no limit" in nginx: none of these chains buffers.
func TestZeroDefaultBodyLimitAddsNoBufferingToServerRouters(t *testing.T) {
	m := fixture(t, "tls-full")
	m.Defaults.MaxBodyBytes = 0
	r := mkRoute(t, "hand:z#1", []string{"z.example.test"}, loc(t, "proxy", "/", "prefix"))
	r.HTTPToHTTPSRedirect = true
	m.Routes = append(m.Routes, r)
	out, err := Render(m, Options{SSLRoot: sslTree(t, m), CertRoot: "/nself/ssl"})
	if err != nil {
		t.Fatal(err)
	}
	rs := bodyRouters(t, out)
	if len(rs) < 5 {
		t.Fatalf("only %d routers checked", len(rs))
	}
	for rn, spec := range rs {
		if got := leadLimit(t, out, spec); got != -1 {
			t.Errorf("%s: buffering %d with a zero default limit", rn, got)
		}
	}
}
