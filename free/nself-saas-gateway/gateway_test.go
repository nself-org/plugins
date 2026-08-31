package main

// gateway_test.go — shared test harness: a gateway wired to httptest fake
// plugin backends + sqlmock, exercised through the real router (tenant
// resolved via a VERIFIED HS256 session JWT — the header path is dead).

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

const testTenant = "11111111-2222-4333-8444-555555555555"

// testUpstreamSecret signs gateway→plugin service JWTs in tests (distinct
// from testJWTSecret so a leaked session JWT can never pass as one).
const testUpstreamSecret = "test-upstream-plugin-secret"

// newTestGateway builds a gateway whose upstreams are the given fake
// servers. db may be nil for proxy-only tests.
func newTestGateway(db *sql.DB, uptime, incident, alert string) *gateway {
	return &gateway{
		cfg: config{
			Port:              "0",
			Cloud:             true,
			UptimeURL:         uptime,
			IncidentURL:       incident,
			AlertURL:          alert,
			StatusPageBaseURL: "https://sentry.nself.org/s",
			InternalAPIKey:    "test-internal-secret",
			JWTSecret:         []byte(testJWTSecret),
			UpstreamJWTSecret: []byte(testUpstreamSecret),
		},
		db: db,
	}
}

// mintTestJWT signs a session JWT for a tenant with the test secret —
// the ONLY way tests authenticate now that the spoofable header is dead.
func mintTestJWT(t *testing.T, tenantID string) string {
	t.Helper()
	now := time.Now().UTC()
	token, err := saas.SignHS256(map[string]any{
		"sub":       tenantID,
		"tenant_id": tenantID,
		"iat":       now.Unix(),
		"exp":       now.Add(time.Hour).Unix(),
	}, []byte(testJWTSecret))
	if err != nil {
		t.Fatalf("mint test jwt: %v", err)
	}
	return token
}

// doReq runs one request through the full router authenticated as testTenant
// via a verified session JWT.
func doReq(t *testing.T, g *gateway, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Authorization", "Bearer "+mintTestJWT(t, testTenant))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}

func newSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	return db, mock
}

func TestHealthNoAuth(t *testing.T) {
	g := newTestGateway(nil, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health = %d, want 200", rec.Code)
	}
}

func TestV1RequiresTenant(t *testing.T) {
	g := newTestGateway(nil, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodGet, "/v1/monitors/", nil) // no credential
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /v1 = %d, want 401", rec.Code)
	}
}

func TestErrorEnvelopeShape(t *testing.T) {
	// A failing upstream must produce {"error":{code,message}}.
	g := newTestGateway(nil, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodGet, "/v1/monitors/", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("dead upstream = %d, want 502", rec.Code)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeBody(t, rec, &env)
	if env.Error.Code == "" || env.Error.Message == "" {
		t.Fatalf("error envelope incomplete: %s", rec.Body.String())
	}
}

// TestProxyMintsServiceJWT locks the dashboard seam: a request authenticated
// with a session JWT must NOT forward that JWT upstream (plugins verify with
// a different secret). Instead the gateway mints a short-lived service JWT
// signed with the UPSTREAM secret carrying the resolved tenant, and injects
// X-Hasura-Tenant-Id. Regression guard for the "dashboard shows mock data" /
// incident-mgmt 401 bugs.
func TestProxyMintsServiceJWT(t *testing.T) {
	var gotAuth, gotTenant string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotTenant = r.Header.Get("X-Hasura-Tenant-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"targets":[]}`))
	}))
	defer upstream.Close()

	g := newTestGateway(nil, upstream.URL, upstream.URL, upstream.URL)
	sessionJWT := mintTestJWT(t, testTenant)
	req := httptest.NewRequest(http.MethodGet, "/v1/monitors/", nil)
	req.Header.Set("Authorization", "Bearer "+sessionJWT)
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("proxied monitors = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if gotTenant != testTenant {
		t.Errorf("upstream tenant = %q, want %q", gotTenant, testTenant)
	}
	forwarded := strings.TrimPrefix(gotAuth, "Bearer ")
	if forwarded == sessionJWT {
		t.Fatal("session JWT was forwarded upstream — must be replaced by a service JWT")
	}
	if forwarded == "" {
		t.Fatal("no upstream Authorization — expected a minted service JWT")
	}
	// The service JWT verifies with the UPSTREAM secret and carries the tenant.
	tid, err := saas.VerifyJWTTenant(forwarded, []byte(testUpstreamSecret))
	if err != nil || tid != testTenant {
		t.Errorf("service JWT tenant = %q err=%v, want %q", tid, err, testTenant)
	}
	// And it must NOT verify with the session secret (different key).
	if _, err := saas.VerifyJWTTenant(forwarded, []byte(testJWTSecret)); err == nil {
		t.Error("service JWT verifies with the session secret — secrets must differ")
	}
}

// TestProxyForwardsAPIKey confirms an nsk_ key IS forwarded upstream (the
// plugin authenticates it directly) — the JWT strip must not touch API keys.
func TestProxyForwardsAPIKey(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"targets":[]}`))
	}))
	defer upstream.Close()

	// callUpstream is the unit under test; drive it directly with a resolved
	// tenant context so we don't need the API-key DB lookup path.
	g := newTestGateway(nil, upstream.URL, "", "")
	req := httptest.NewRequest(http.MethodGet, "/v1/monitors/", nil)
	req.Header.Set("Authorization", "Bearer nsk_livekey")
	req = req.WithContext(saas.WithTenant(req.Context(), testTenant))
	status, _, err := g.callUpstream(req.Context(), req, http.MethodGet, g.cfg.UptimeURL, "/api/v1/targets/", nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("callUpstream = %d, err=%v", status, err)
	}
	if gotAuth != "Bearer nsk_livekey" {
		t.Errorf("upstream Authorization = %q, want the nsk_ key forwarded", gotAuth)
	}
}

func TestCodeForStatus(t *testing.T) {
	cases := map[int]string{
		401: "unauthorized", 403: "forbidden", 404: "not_found",
		402: "quota_exceeded", 429: "quota_exceeded",
		400: "invalid_request", 422: "invalid_request",
		500: "upstream_error", 503: "upstream_error",
	}
	for status, want := range cases {
		if got := codeForStatus(status); got != want {
			t.Errorf("codeForStatus(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestUpstreamMessage(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{"error":"db error"}`, "db error"},
		{`{"error":"quota_exceeded","message":"Your free tier allows 10 monitors."}`,
			"Your free tier allows 10 monitors."},
		{`plain text`, "plain text"},
		{``, "upstream error"},
	}
	for _, c := range cases {
		if got := upstreamMessage([]byte(c.body)); got != c.want {
			t.Errorf("upstreamMessage(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}
