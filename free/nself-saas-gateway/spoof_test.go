package main

// spoof_test.go — regression tests for the P0 tenant-spoof fix.
//
// Purpose: lock the invariant that tenant identity on the public /v1 surface
//   comes ONLY from a verified credential (nsk_ API key or HS256 JWT). A
//   client-supplied X-Hasura-Tenant-Id header must be stripped at the edge
//   and never reach tenant resolution or the upstream plugins.
// Covers: spoofed header ignored (JWT and API-key paths), header-only
//   requests rejected 401, invalid JWT rejected 401 even with a header.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

const victimTenant = "99999999-8888-4777-8666-555555555555"

// spyUpstream records the tenant header each upstream request carried.
func spyUpstream(tenants *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*tenants = append(*tenants, r.Header.Get("X-Hasura-Tenant-Id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"targets":[],"results":[]}`))
	}))
}

// TestSpoofedHeaderIgnoredWithJWT: a valid JWT for tenant A plus a spoofed
// X-Hasura-Tenant-Id naming tenant B must resolve to A — the header is dead.
func TestSpoofedHeaderIgnoredWithJWT(t *testing.T) {
	var seen []string
	upstream := spyUpstream(&seen)
	defer upstream.Close()

	g := newTestGateway(nil, upstream.URL, upstream.URL, upstream.URL)
	req := httptest.NewRequest(http.MethodGet, "/v1/monitors/", nil)
	req.Header.Set("Authorization", "Bearer "+mintTestJWT(t, testTenant))
	req.Header.Set("X-Hasura-Tenant-Id", victimTenant) // spoof attempt
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("monitors with JWT = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(seen) == 0 {
		t.Fatal("upstream never called")
	}
	for _, tid := range seen {
		if tid != testTenant {
			t.Errorf("upstream tenant = %q, want the JWT's tenant %q (spoofed header leaked)", tid, testTenant)
		}
	}
}

// TestSpoofedHeaderAloneRejected: X-Hasura-Tenant-Id with no credential must
// be a hard 401 — the pre-fix behaviour granted full access to any tenant.
func TestSpoofedHeaderAloneRejected(t *testing.T) {
	g := newTestGateway(nil, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for _, path := range []string{"/v1/monitors/", "/v1/incidents/", "/v1/me", "/v1/alerts/channels/", "/v1/status-pages/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Hasura-Tenant-Id", victimTenant)
		rec := httptest.NewRecorder()
		g.router().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s with header-only auth = %d, want 401", path, rec.Code)
		}
	}
}

// TestInvalidJWTWithSpoofedHeaderRejected: a garbage JWT must 401 even when a
// spoofed tenant header is present — no fall-through to header resolution.
func TestInvalidJWTWithSpoofedHeaderRejected(t *testing.T) {
	g := newTestGateway(nil, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodGet, "/v1/monitors/", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.eyJ0ZW5hbnRfaWQiOiJ4In0.bogussignature")
	req.Header.Set("X-Hasura-Tenant-Id", victimTenant)
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid JWT + spoofed header = %d, want 401", rec.Code)
	}
}

// TestAPIKeyResolvesOwnTenantSpoofIgnored: an nsk_ key must resolve to ITS
// tenant; a spoofed header naming another tenant must not override it.
func TestAPIKeyResolvesOwnTenantSpoofIgnored(t *testing.T) {
	var seen []string
	upstream := spyUpstream(&seen)
	defer upstream.Close()

	db, mock := newSQLMock(t)
	rawKey, _, err := saas.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	mock.ExpectQuery(`SELECT id, tenant_id, scopes, revoked_at`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "scopes", "revoked_at"}).
			AddRow("key-1", testTenant, "full", nil))
	mock.ExpectExec(`UPDATE np_saas_api_keys SET last_used_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// EffectiveLimits → GetTenant: no row ⇒ free-tier default (read-only API,
	// GET is still permitted).
	mock.ExpectQuery(`SELECT tier, quota_overrides, stripe_customer_id, created_at`).
		WillReturnError(sql.ErrNoRows)

	g := newTestGateway(db, upstream.URL, upstream.URL, upstream.URL)
	req := httptest.NewRequest(http.MethodGet, "/v1/monitors/", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("X-Hasura-Tenant-Id", victimTenant) // spoof attempt
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("monitors with nsk key = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	for _, tid := range seen {
		if tid != testTenant {
			t.Errorf("upstream tenant = %q, want the key's tenant %q (spoofed header leaked)", tid, testTenant)
		}
	}
}
