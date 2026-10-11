package render

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

func parse(t *testing.T, b []byte) map[string]map[string]map[string]any {
	t.Helper()
	var doc map[string]map[string]map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// clone deep-copies a route through JSON (the model is plain data).
func clone(t *testing.T, r contract.Route) contract.Route {
	t.Helper()
	b, _ := json.Marshal(r)
	var out contract.Route
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func noteRoute(t *testing.T, m *contract.Model) contract.Route {
	t.Helper()
	for _, r := range m.Routes {
		if r.ID == "plugin:notes/notes.conf#1" {
			return clone(t, r)
		}
	}
	t.Fatal("notes route missing from the plugin-modelled fixture")
	return contract.Route{}
}

// M1: nginx limit_conn counts per client address; Traefik groups by Host unless a sourceCriterion is set.
func TestConnLimitIsPerClientIP(t *testing.T) {
	out, err := renderFixture(t, "http-full")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for name, mw := range parse(t, out)["http"]["middlewares"] {
		spec, ok := mw.(map[string]any)["inFlightReq"].(map[string]any)
		if !ok {
			continue
		}
		n++
		sc, _ := spec["sourceCriterion"].(map[string]any)
		ip, _ := sc["ipStrategy"].(map[string]any)
		if depth, ok := ip["depth"]; !ok || depth != 0 {
			t.Errorf("%s: inFlightReq lacks sourceCriterion.ipStrategy.depth 0 (limit would be per Host): %v", name, spec)
		}
	}
	if n == 0 {
		t.Fatal("no inFlightReq middleware rendered: the test checked nothing")
	}
}

// M3: ids that safe() maps to one name must not share routers.
func TestRouterNamesAreInjective(t *testing.T) {
	m := fixture(t, "plugin-modelled")
	base, err := Render(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	notes := noteRoute(t, m)
	if safe(notes.ID) != safe("plugin:notes-notes/conf#1") {
		t.Fatalf("test premise: %q and the clash id must share safe(): %q", notes.ID, safe(notes.ID))
	}
	other := clone(t, notes)
	other.ID, other.ServerNames = "plugin:notes-notes/conf#1", []string{"other.example.test"}
	m.Routes = append(m.Routes, other)
	out, err := Render(m, Options{})
	if err != nil {
		t.Fatalf("two distinct ids must render: %v", err)
	}
	before, after := parse(t, base)["http"]["routers"], parse(t, out)["http"]["routers"]
	for name, r := range before {
		got, ok := after[name]
		if !ok || got.(map[string]any)["rule"] != r.(map[string]any)["rule"] {
			t.Errorf("router %s was overwritten or dropped by the second route: %v", name, got)
		}
	}
	if len(after) <= len(before) {
		t.Errorf("the second route added no routers: %d -> %d", len(before), len(after))
	}
}

func TestSameRouteIDTwiceIsRefusedAndWritesNothing(t *testing.T) {
	m := fixture(t, "plugin-modelled")
	dup := noteRoute(t, m)
	dup.ServerNames = []string{"other.example.test"}
	m.Routes = append(m.Routes, dup)
	out, err := Render(m, Options{})
	if err == nil || out != nil || !strings.Contains(err.Error(), "already used by another route") {
		t.Fatalf("want a name-collision refusal and no output, got %v / %d bytes", err, len(out))
	}
}

func TestDuplicateServerNameOnOneListenerIsRefused(t *testing.T) {
	m := fixture(t, "plugin-modelled")
	dup := noteRoute(t, m)
	dup.ID = "plugin:second/second.conf#1"
	m.Routes = append(m.Routes, dup)
	out, err := Render(m, Options{})
	if err == nil || out != nil || !strings.Contains(err.Error(), "already served by route plugin:notes/notes.conf#1") {
		t.Fatalf("want a duplicate-host refusal naming the first route, got %v", err)
	}
	// the same name on another listener is allowed (one route per scheme)
	m = fixture(t, "plugin-modelled")
	dup = noteRoute(t, m)
	dup.ID = "plugin:second/second.conf#1"
	dup.Listen.HTTP = false
	dup.Listen.HTTPS = true
	dup.TLS = nil
	m.Routes = append(m.Routes, dup)
	_, err = Render(m, Options{})
	if err == nil || strings.Contains(err.Error(), "already served by route") {
		t.Fatalf("a different listener must not trip the duplicate check: %v", err)
	}
}

// Should-fix and nit refusals: each mutates one field of tls-full and expects the refusal to name it.
func TestRefusesHardeningCases(t *testing.T) {
	limit := func(zone string, burst int) func(m *contract.Model) {
		return func(m *contract.Model) {
			m.Routes[0].Locations[0].RateLimit = &struct {
				Zone    string `json:"zone"`
				Burst   int    `json:"burst"`
				Nodelay bool   `json:"nodelay"`
			}{zone, burst, true}
		}
	}
	setZone := func(name string, f func(z *contract.Zone)) func(m *contract.Model) {
		return func(m *contract.Model) {
			for i := range m.Zones {
				if m.Zones[i].Name == name {
					f(&m.Zones[i])
				}
			}
		}
	}
	both := func(fs ...func(m *contract.Model)) func(m *contract.Model) {
		return func(m *contract.Model) {
			for _, f := range fs {
				f(m)
			}
		}
	}
	neg := int64(-1)
	cases := []struct {
		name, want string
		mut        func(m *contract.Model)
	}{
		{"zero rate", "zero or unreadable", both(limit("api", 1), setZone("api", func(z *contract.Zone) { r := "0r/s"; z.Rate = &r }))},
		{"zero rate per minute", "zero or unreadable", both(limit("api", 1), setZone("api", func(z *contract.Zone) { r := "0r/m"; z.Rate = &r }))},
		{"negative burst", "burst -2 is negative", limit("api", -2)},
		{"negative location body limit", "max_body_bytes", func(m *contract.Model) { m.Routes[0].Locations[0].MaxBodyBytes = &neg }},
		{"negative default body limit", "defaults.max_body_bytes", func(m *contract.Model) { m.Defaults.MaxBodyBytes = -1 }},
		{"negative conn_limit", "conn_limit", func(m *contract.Model) { m.Routes[0].RateLimit.ConnLimit = -1 }},
		{"authorization zone key", "share one bucket", both(limit("api", 1), setZone("api", func(z *contract.Zone) { z.Key = "$http_authorization" }))},
		{"tenant zone key", "share one bucket", both(limit("api", 1), setZone("api", func(z *contract.Zone) { z.Key = "$http_x_tenant_id" }))},
		{"header value CRLF", "control character", func(m *contract.Model) { m.Routes[0].Locations[0].HeadersSet["X-Req"] = "a\r\nSet-Cookie: x=1" }},
		{"header name not a token", "not a token", func(m *contract.Model) { m.Routes[0].Locations[0].HeadersSet["X Bad:"] = "v" }},
		{"security header CRLF", "control character", func(m *contract.Model) { m.Routes[0].SecurityHeaders["X-Evil"] = "a\nb" }},
		{"no listener", "neither http nor https", func(m *contract.Model) { m.Routes[0].Listen.HTTP, m.Routes[0].Listen.HTTPS = false, false }},
		{"only DHE ciphers", "no cipher Go implements", func(m *contract.Model) {
			c := "DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384"
			m.Routes[len(m.Routes)-1].TLS.Ciphers = &c
		}},
		{"wildcard with custom TLS option", "Host() rules only", func(m *contract.Model) {
			r := &m.Routes[len(m.Routes)-1]
			c := "ECDHE-RSA-AES128-GCM-SHA256"
			r.TLS.Ciphers, r.ServerNames = &c, []string{"*.wild.example.org"}
		}},
	}
	for _, c := range cases {
		m := fixture(t, "tls-full")
		root := sslTree(t, m)
		if m.Routes[0].SecurityHeaders == nil {
			m.Routes[0].SecurityHeaders = map[string]string{}
		}
		c.mut(m)
		out, err := Render(m, Options{SSLRoot: root})
		if err == nil || out != nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want refusal containing %q, got %v", c.name, c.want, err)
		}
	}
}

// Second-opinion must-fix 1: the exact pair that safe() folded together.
func TestHandRouteIDsThatSafeFoldTogether(t *testing.T) {
	m := fixture(t, "plugin-modelled")
	a := noteRoute(t, m)
	a.ID, a.ServerNames = "hand:a.b#1", []string{"ab-dot.example.test"}
	b := clone(t, a)
	b.ID, b.ServerNames = "hand:a-b#1", []string{"ab-dash.example.test"}
	if safe(a.ID) != safe(b.ID) {
		t.Fatalf("test premise: safe() must fold %q and %q together", a.ID, b.ID)
	}
	m.Routes = []contract.Route{a, b}
	out, err := Render(m, Options{})
	if err != nil {
		t.Fatalf("both routes must render: %v", err)
	}
	rules := ""
	for _, r := range parse(t, out)["http"]["routers"] {
		rules += r.(map[string]any)["rule"].(string) + "\n"
	}
	for _, h := range []string{"ab-dot.example.test", "ab-dash.example.test"} {
		if !strings.Contains(rules, "Host(`"+h+"`)") {
			t.Errorf("no router for %s: one route was dropped", h)
		}
	}
}

func priorityOf(t *testing.T, out []byte, rule string) int {
	t.Helper()
	for _, r := range parse(t, out)["http"]["routers"] {
		spec := r.(map[string]any)
		if spec["rule"] == rule {
			return spec["priority"].(int)
		}
	}
	t.Fatalf("no router with rule %s", rule)
	return 0
}

// Second-opinion must-fix 2: nginx picks the server by exact name first, so an
// exact host's "/" must outrank a wildcard host's longer /api/.
func TestExactHostBeatsWildcardWithLongerPath(t *testing.T) {
	out, err := renderFixture(t, "host-tier")
	if err != nil {
		t.Fatal(err)
	}
	exact := priorityOf(t, out, "Host(`notes.example.test`) && PathPrefix(`/`)")
	wild := priorityOf(t, out, "HostRegexp(`^.+\\.example\\.test$`) && PathPrefix(`/api/`)")
	if exact <= wild {
		t.Fatalf("exact host / priority %d must exceed wildcard /api/ priority %d", exact, wild)
	}
}

func TestLeadingDotServerNameMatchesNameAndSubdomains(t *testing.T) {
	m := fixture(t, "plugin-modelled")
	r := noteRoute(t, m)
	r.ID, r.ServerNames = "plugin:dot/dot.conf#1", []string{".dot.example.test"}
	m.Routes = append(m.Routes, r)
	out, err := Render(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "(Host(`dot.example.test`) || HostRegexp(`^.+\\.dot\\.example\\.test$`)) && PathPrefix(`/`)"
	priorityOf(t, out, want)
}

// Second-opinion should-fix 9: access_log off must not be dropped silently.
func TestQuietLocationsSkipAccessLog(t *testing.T) {
	out, err := renderFixture(t, "host-tier")
	if err != nil {
		t.Fatal(err)
	}
	quietSeen := false
	for name, r := range parse(t, out)["http"]["routers"] {
		spec := r.(map[string]any)
		quiet := strings.Contains(spec["rule"].(string), "/quiet/")
		_, has := spec["observability"]
		quietSeen = quietSeen || quiet
		if quiet != has {
			t.Errorf("%s: access_log off must set observability.accessLogs false exactly on /quiet/ (quiet=%v observability=%v)", name, quiet, has)
		}
	}
	if !quietSeen {
		t.Fatal("no /quiet/ router: the test checked nothing")
	}
}
