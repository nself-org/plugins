package main

// handlers_statuspublic_test.go — GET /v1/status/public/{slug}.
//
// Locks the fail-closed rules: public components only, private/internal
// probes absent, unknown + unpublished slugs → identical generic 404, no
// caller credential ever forwarded upstream, cross-tenant slugs resolve to
// their OWN tenant regardless of what the caller sends.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

const otherTenant = "99999999-8888-4777-8666-555555555555"

// fakeUptimeAndIncidents serves the uptime + incident plugin surfaces and
// records the tenant + Authorization each request carried.
func fakeUptimeAndIncidents(t *testing.T, targets, results, incidents string, gotTenants *[]string, gotAuths *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotTenants = append(*gotTenants, r.Header.Get("X-Hasura-Tenant-Id"))
		*gotAuths = append(*gotAuths, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/targets"):
			_, _ = w.Write([]byte(targets))
		case strings.HasPrefix(r.URL.Path, "/api/v1/results"):
			_, _ = w.Write([]byte(results))
		case strings.HasPrefix(r.URL.Path, "/incidents"):
			_, _ = w.Write([]byte(incidents))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

const testPageID = "0f0f0f0f-1111-4222-8333-444444444444"

func expectSlugLookup(mock sqlmock.Sqlmock, slug, tenantID, title string) {
	mock.ExpectQuery(`SELECT id::text, tenant_id::text, name`).WithArgs(slug).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name"}).
			AddRow(testPageID, tenantID, title))
	// No curation rows → the legacy host-shape heuristic applies.
	mock.ExpectQuery(`FROM np_saas_status_page_components`).WithArgs(testPageID, tenantID).
		WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "name", "public"}))
}

// TestPublicStatusRendersPublicComponentsOnly — the core render: public
// monitors appear with status + uptime, internal-shaped and name-marked
// private monitors are absent, URLs/tenant ids never leak, and NO caller
// credential reaches the plugins (a minted service JWT for the REGISTRY
// tenant does).
func TestPublicStatusRendersPublicComponentsOnly(t *testing.T) {
	targets := `{"targets":[
		{"id":"t-pub","name":"Website","url":"https://example.com","protocol":"https","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"},
		{"id":"t-ip","name":"DB probe","url":"http://10.0.0.5:5432","protocol":"http","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"},
		{"id":"t-svc","name":"redis","url":"redis:6379","protocol":"tcp","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"},
		{"id":"t-marked","name":"[private] Admin API","url":"https://admin.example.com","protocol":"https","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"},
		{"id":"t-optin","name":"[public] Queue","url":"http://192.168.1.9:8080","protocol":"http","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"}
	]}`
	results := `{"results":[
		{"id":"r1","target_id":"t-pub","checked_at":"2026-07-02T00:03:00Z","status":"up","region":"default"},
		{"id":"r2","target_id":"t-pub","checked_at":"2026-07-02T00:02:00Z","status":"down","region":"default"},
		{"id":"r3","target_id":"t-pub","checked_at":"2026-07-02T00:01:00Z","status":"up","region":"default"},
		{"id":"r4","target_id":"t-pub","checked_at":"2026-07-02T00:00:00Z","status":"up","region":"default"},
		{"id":"r5","target_id":"t-ip","checked_at":"2026-07-02T00:00:00Z","status":"down","region":"default"}
	]}`
	incidents := `{"incidents":[
		{"id":"i-1","title":"Elevated latency","severity":"minor","state":"open","created_at":"2026-07-01T12:00:00Z"},
		{"id":"i-old","title":"Ancient outage","severity":"major","state":"resolved","created_at":"2026-01-01T00:00:00Z","resolved_at":"2026-01-02T00:00:00Z"}
	]}`

	var gotTenants, gotAuths []string
	upstream := fakeUptimeAndIncidents(t, targets, results, incidents, &gotTenants, &gotAuths)
	defer upstream.Close()

	db, mock := newSQLMock(t)
	g := newTestGateway(db, upstream.URL, upstream.URL, upstream.URL)
	expectSlugLookup(mock, "acme-status", testTenant, "Acme Status")

	// Unauthenticated request carrying a HOSTILE credential + spoof header —
	// none of it may matter or be forwarded.
	req := httptest.NewRequest(http.MethodGet, "/v1/status/public/acme-status", nil)
	req.Header.Set("Authorization", "Bearer nsk_hostilekey")
	req.Header.Set("X-Hasura-Tenant-Id", otherTenant)
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("public status = %d: %s", rec.Code, rec.Body.String())
	}

	var env struct {
		StatusPage struct {
			Title         string               `json:"title"`
			Slug          string               `json:"slug"`
			OverallStatus string               `json:"overall_status"`
			Components    []publicComponentDTO `json:"components"`
			Incidents     []publicIncidentDTO  `json:"incidents"`
			GeneratedAt   string               `json:"generated_at"`
		} `json:"status_page"`
	}
	decodeBody(t, rec, &env)
	p := env.StatusPage

	if p.Title != "Acme Status" || p.Slug != "acme-status" || p.GeneratedAt == "" {
		t.Errorf("page header wrong: %+v", p)
	}
	if len(p.Components) != 2 {
		t.Fatalf("components = %d, want 2 (public + opted-in): %+v", len(p.Components), p.Components)
	}
	if p.Components[0].ID != "t-pub" || p.Components[0].Status != "operational" {
		t.Errorf("public component wrong: %+v", p.Components[0])
	}
	if p.Components[0].UptimePercent == nil || *p.Components[0].UptimePercent != 75 {
		t.Errorf("uptime = %v, want 75", p.Components[0].UptimePercent)
	}
	if p.Components[1].ID != "t-optin" || p.Components[1].Name != "Queue" || p.Components[1].Status != "unknown" {
		t.Errorf("opted-in component wrong: %+v", p.Components[1])
	}
	if p.OverallStatus != "operational" {
		t.Errorf("overall = %q, want operational (no public comp down)", p.OverallStatus)
	}
	if len(p.Incidents) != 1 || p.Incidents[0].Title != "Elevated latency" || p.Incidents[0].Status != "open" {
		t.Errorf("incidents wrong (want active only, old resolved dropped): %+v", p.Incidents)
	}

	// Leak checks: private component ids/names, monitor URLs, tenant ids,
	// internal hosts must be absent from the raw body.
	raw := rec.Body.String()
	for _, secret := range []string{"t-ip", "t-svc", "t-marked", "10.0.0.5", "redis", "Admin API",
		"example.com", "192.168.1.9", testTenant, otherTenant, "[public]"} {
		if strings.Contains(raw, secret) {
			t.Errorf("public payload leaks %q: %s", secret, raw)
		}
	}

	// Upstream credential rules: the hostile key must never be forwarded;
	// every plugin call carries the REGISTRY tenant + a service JWT for it.
	if len(gotTenants) == 0 {
		t.Fatal("no upstream calls recorded")
	}
	for i, tid := range gotTenants {
		if tid != testTenant {
			t.Errorf("upstream call %d tenant = %q, want registry tenant %q", i, tid, testTenant)
		}
		auth := strings.TrimPrefix(gotAuths[i], "Bearer ")
		if auth == "nsk_hostilekey" {
			t.Fatalf("hostile caller credential forwarded upstream")
		}
		if got, err := saas.VerifyJWTTenant(auth, []byte(testUpstreamSecret)); err != nil || got != testTenant {
			t.Errorf("upstream call %d service JWT tenant = %q err=%v, want %q", i, got, err, testTenant)
		}
	}
}

// TestPublicStatusUnknownSlug404 — unknown and unpublished slugs return the
// SAME generic 404 (no enumeration), as does a malformed slug (no DB hit).
func TestPublicStatusUnknownSlug404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	// Unknown slug (or public=false — the WHERE clause makes them identical).
	mock.ExpectQuery(`SELECT id::text, tenant_id::text, name`).WithArgs("no-such-page").
		WillReturnError(sql.ErrNoRows)
	req := httptest.NewRequest(http.MethodGet, "/v1/status/public/no-such-page", nil)
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown slug = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	unknownBody := rec.Body.String()

	// Malformed slug → identical 404 without touching the DB.
	req = httptest.NewRequest(http.MethodGet, "/v1/status/public/NOT%20A%20SLUG!", nil)
	rec = httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("malformed slug = %d, want 404", rec.Code)
	}
	if rec.Body.String() != unknownBody {
		t.Errorf("404 bodies differ (enumeration signal): %q vs %q", rec.Body.String(), unknownBody)
	}
}

// TestPublicStatusCrossTenantIsolation — slug B renders tenant B's data only:
// the upstream calls carry tenant B, never tenant A (even when the caller
// presents tenant A's session JWT).
func TestPublicStatusCrossTenantIsolation(t *testing.T) {
	var gotTenants, gotAuths []string
	upstream := fakeUptimeAndIncidents(t,
		`{"targets":[{"id":"b-1","name":"B Site","url":"https://b.example.org","protocol":"https","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"}]}`,
		`{"results":[]}`, `{"incidents":[]}`, &gotTenants, &gotAuths)
	defer upstream.Close()

	db, mock := newSQLMock(t)
	g := newTestGateway(db, upstream.URL, upstream.URL, upstream.URL)
	expectSlugLookup(mock, "tenant-b-page", otherTenant, "Tenant B")

	// Caller is authenticated as testTenant (A) — must not matter.
	req := httptest.NewRequest(http.MethodGet, "/v1/status/public/tenant-b-page", nil)
	req.Header.Set("Authorization", "Bearer "+mintTestJWT(t, testTenant))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("cross-tenant public page = %d: %s", rec.Code, rec.Body.String())
	}
	for i, tid := range gotTenants {
		if tid != otherTenant {
			t.Errorf("upstream call %d tenant = %q, want page owner %q", i, tid, otherTenant)
		}
		got, err := saas.VerifyJWTTenant(strings.TrimPrefix(gotAuths[i], "Bearer "), []byte(testUpstreamSecret))
		if err != nil || got != otherTenant {
			t.Errorf("service JWT tenant = %q err=%v, want %q", got, err, otherTenant)
		}
	}
	if !strings.Contains(rec.Body.String(), "B Site") {
		t.Errorf("page missing owner's component: %s", rec.Body.String())
	}
}

// TestMonitorPublicVisibility covers the visibility resolver directly.
func TestMonitorPublicVisibility(t *testing.T) {
	cases := []struct {
		name, url string
		want      bool
	}{
		{"Website", "https://example.com", true},
		{"API", "https://api.example.com/health", true},
		{"Public IP", "http://203.0.113.7", true},
		{"loopback", "http://127.0.0.1:8080", false},
		{"rfc1918", "http://10.1.2.3", false},
		{"rfc1918-172", "https://172.16.0.9", false},
		{"link-local", "http://169.254.1.1", false},
		{"dotless", "postgres:5432", false},
		{"local suffix", "http://nas.local", false},
		{"internal suffix", "https://vault.internal", false},
		{"localhost", "http://localhost:3000", false},
		{"empty", "", false},
		{"[private] marker", "https://example.com", false},
		{"[public] optin", "http://10.0.0.1", true},
	}
	for _, c := range cases {
		got := monitorPublic(upstreamTarget{Name: c.name, URL: c.url})
		if got != c.want {
			t.Errorf("monitorPublic(%q, %q) = %v, want %v", c.name, c.url, got, c.want)
		}
	}
}

// TestOverallStatusDerivation covers the banner rule.
func TestOverallStatusDerivation(t *testing.T) {
	op := publicComponentDTO{Status: "operational"}
	down := publicComponentDTO{Status: "down"}
	unknown := publicComponentDTO{Status: "unknown"}
	cases := []struct {
		comps []publicComponentDTO
		want  string
	}{
		{nil, "operational"},
		{[]publicComponentDTO{op, op}, "operational"},
		{[]publicComponentDTO{op, unknown}, "operational"},
		{[]publicComponentDTO{op, down}, "degraded"},
		{[]publicComponentDTO{down, down}, "down"},
		{[]publicComponentDTO{down, unknown}, "down"},
	}
	for i, c := range cases {
		if got := overallStatus(c.comps); got != c.want {
			t.Errorf("case %d overall = %q, want %q", i, got, c.want)
		}
	}
}

// TestPublicStatusCuratedComponents — when the page HAS component rows,
// exactly the public=true ones render (fail-closed): a public-shaped monitor
// with no row is hidden, a private-marked monitor stays hidden even when its
// row says public, and name overrides apply.
func TestPublicStatusCuratedComponents(t *testing.T) {
	targets := `{"targets":[
		{"id":"t-a","name":"Website","url":"https://example.com","protocol":"https","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"},
		{"id":"t-b","name":"API","url":"https://api.example.com","protocol":"https","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"},
		{"id":"t-c","name":"[private] Admin","url":"https://admin.example.com","protocol":"https","interval_secs":60,"enabled":true,"created_at":"2026-06-01T00:00:00Z"}
	]}`
	var gotTenants, gotAuths []string
	upstream := fakeUptimeAndIncidents(t, targets, `{"results":[]}`, `{"incidents":[]}`, &gotTenants, &gotAuths)
	defer upstream.Close()

	db, mock := newSQLMock(t)
	g := newTestGateway(db, upstream.URL, upstream.URL, upstream.URL)

	mock.ExpectQuery(`SELECT id::text, tenant_id::text, name`).WithArgs("curated").
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name"}).
			AddRow(testPageID, testTenant, "Curated Status"))
	// Curation rows: t-a public, t-b private, t-c public-but-marked-private.
	// (t-b hidden by public=false; a monitor with NO row would also hide.)
	mock.ExpectQuery(`FROM np_saas_status_page_components`).WithArgs(testPageID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"monitor_id", "name", "public"}).
			AddRow("t-a", "Main Site", true).
			AddRow("t-b", "", false).
			AddRow("t-c", "", true))

	req := httptest.NewRequest(http.MethodGet, "/v1/status/public/curated", nil)
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("curated public status = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		StatusPage struct {
			Components []publicComponentDTO `json:"components"`
		} `json:"status_page"`
	}
	decodeBody(t, rec, &env)
	if len(env.StatusPage.Components) != 1 {
		t.Fatalf("components = %+v, want exactly the one curated-public monitor",
			env.StatusPage.Components)
	}
	c := env.StatusPage.Components[0]
	if c.ID != "t-a" || c.Name != "Main Site" {
		t.Errorf("curated component = %+v, want t-a renamed to Main Site", c)
	}
}
