package main

// handlers_errors_test.go — /v1/errors + /v1/rum (sqlmock): list/detail/
// resolve shapes and the P0 tenant-isolation invariants (every query is
// parameterized by the VERIFIED tenant; cross-tenant ids 404).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// plainRequest builds an unauthenticated request + recorder pair.
func plainRequest(method, path string) (*http.Request, *httptest.ResponseRecorder) {
	return httptest.NewRequest(method, path, nil), httptest.NewRecorder()
}

const otherTenantGroupID = "99999999-8888-4777-a666-555555555544"

func errorGroupRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "project_id", "title", "level", "status",
		"event_count", "first_seen", "last_seen", "resolved_at"}).
		AddRow(otherTenantGroupID, "web", "TypeError: x is undefined", "error",
			"unresolved", int64(42), time.Now(), time.Now(), nil)
}

func TestListErrorsTenantScoped(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`to_regclass`).WithArgs("np_errors_groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// ISOLATION: the query must be parameterized by the VERIFIED tenant.
	mock.ExpectQuery(`FROM np_errors_groups\s+WHERE tenant_id = \$1`).WithArgs(testTenant).
		WillReturnRows(errorGroupRows())

	rec := doReq(t, g, http.MethodGet, "/v1/errors", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list errors = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Errors []errorGroupDTO `json:"errors"`
	}
	decodeBody(t, rec, &env)
	if len(env.Errors) != 1 || env.Errors[0].Title != "TypeError: x is undefined" {
		t.Errorf("errors = %+v", env.Errors)
	}
	if env.Errors[0].EventCount != 42 || env.Errors[0].Status != "unresolved" {
		t.Errorf("group fields wrong: %+v", env.Errors[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestListErrorsStatusFilterValidated(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectQuery(`to_regclass`).WithArgs("np_errors_groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	rec := doReq(t, g, http.MethodGet, "/v1/errors?status=drop%20table", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status filter = %d, want 422", rec.Code)
	}
}

func TestListErrorsMissingTableEmpty(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectQuery(`to_regclass`).WithArgs("np_errors_groups").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	rec := doReq(t, g, http.MethodGet, "/v1/errors", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("missing table = %d, want 200", rec.Code)
	}
	var env struct {
		Errors []errorGroupDTO `json:"errors"`
	}
	decodeBody(t, rec, &env)
	if len(env.Errors) != 0 {
		t.Errorf("expected empty list, got %+v", env.Errors)
	}
}

// TestGetErrorCrossTenant404 — ISOLATION: a group id owned by another tenant
// (the mock returns no rows for OUR tenant) must 404 exactly like a missing
// one.
func TestGetErrorCrossTenant404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`FROM np_errors_groups\s+WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(otherTenantGroupID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // no rows for THIS tenant

	rec := doReq(t, g, http.MethodGet, "/v1/errors/"+otherTenantGroupID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant group = %d, want 404", rec.Code)
	}
}

func TestGetErrorNonUUID404(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodGet, "/v1/errors/not-a-uuid", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-uuid id = %d, want 404 (no DB round trip)", rec.Code)
	}
}

// TestResolveErrorCrossTenant404 — ISOLATION: resolving another tenant's
// group updates zero rows → 404, never a silent success.
func TestResolveErrorCrossTenant404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`UPDATE np_errors_groups`).
		WithArgs(otherTenantGroupID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // no rows updated

	rec := doReq(t, g, http.MethodPost, "/v1/errors/"+otherTenantGroupID+"/resolve", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant resolve = %d, want 404", rec.Code)
	}
}

func TestResolveErrorOwnGroup(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	rows := sqlmock.NewRows([]string{
		"id", "project_id", "title", "level", "status",
		"event_count", "first_seen", "last_seen", "resolved_at"}).
		AddRow(otherTenantGroupID, "web", "TypeError", "error", "resolved",
			int64(7), time.Now(), time.Now(), time.Now())
	mock.ExpectQuery(`UPDATE np_errors_groups`).
		WithArgs(otherTenantGroupID, testTenant).WillReturnRows(rows)

	rec := doReq(t, g, http.MethodPost, "/v1/errors/"+otherTenantGroupID+"/resolve", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		ErrorGroup errorGroupDTO `json:"error_group"`
	}
	decodeBody(t, rec, &env)
	if env.ErrorGroup.Status != "resolved" || env.ErrorGroup.ResolvedAt == "" {
		t.Errorf("resolved group = %+v", env.ErrorGroup)
	}
}

// TestRUMSummaryTenantScoped — ISOLATION: every aggregate is parameterized
// by the verified tenant.
func TestRUMSummaryTenantScoped(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`FROM np_saas_usage`).
		WithArgs(testTenant, "rum_pageviews", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(int64(1234)))
	mock.ExpectQuery(`to_regclass`).WithArgs("np_rum_sessions").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_rum_sessions WHERE tenant_id = \$1`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(9)))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_rum_events WHERE tenant_id = \$1$`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(100)))
	mock.ExpectQuery(`event_type = 'error'`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(5)))
	for range rumVitalKeys {
		mock.ExpectQuery(`FROM np_rum_events`).
			WithArgs(testTenant, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"avg"}).AddRow(2.5))
	}

	rec := doReq(t, g, http.MethodGet, "/v1/rum", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("rum = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		RUM struct {
			PageviewsMonth int64              `json:"pageviews_month"`
			SessionsTotal  int64              `json:"sessions_total"`
			ErrorRate      float64            `json:"error_rate"`
			WebVitals      map[string]float64 `json:"web_vitals"`
		} `json:"rum"`
	}
	decodeBody(t, rec, &env)
	if env.RUM.PageviewsMonth != 1234 || env.RUM.SessionsTotal != 9 {
		t.Errorf("rum summary = %+v", env.RUM)
	}
	if env.RUM.ErrorRate != 0.05 {
		t.Errorf("error_rate = %v, want 0.05", env.RUM.ErrorRate)
	}
	if env.RUM.WebVitals["lcp_ms_avg"] != 2.5 {
		t.Errorf("web_vitals = %+v", env.RUM.WebVitals)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestErrorsUnauthenticated401 — no credential → 401 before any DB access.
func TestErrorsUnauthenticated401(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for _, path := range []string{"/v1/errors", "/v1/rum"} {
		req, rec := plainRequest(http.MethodGet, path)
		g.router().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s unauthenticated = %d, want 401", path, rec.Code)
		}
	}
}
