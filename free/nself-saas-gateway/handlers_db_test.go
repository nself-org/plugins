package main

// handlers_db_test.go — DB-backed surfaces (/v1/me, /v1/status-pages,
// /v1/signup, /internal/billing/tenant-tier) with sqlmock.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMeReportsTierAndQuotas(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	// GetTenant
	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("bundle", []byte(`{}`), nil, time.Now()))
	// owner_email
	mock.ExpectQuery(`SELECT COALESCE\(owner_email`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"owner_email"}).AddRow("a@b.co"))
	// monitors: to_regclass probe + count
	mock.ExpectQuery(`to_regclass`).WithArgs("np_uptime_targets").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_uptime_targets`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))
	// heartbeats: table missing on this box
	mock.ExpectQuery(`to_regclass`).WithArgs("np_cron_monitors").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	// status pages
	mock.ExpectQuery(`to_regclass`).WithArgs("np_saas_status_pages").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_status_pages`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	// seats (owner + members)
	mock.ExpectQuery(`to_regclass`).WithArgs("np_saas_team_members").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_team_members`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// monthly counters
	mock.ExpectQuery(`FROM np_saas_usage`).WithArgs(testTenant, "error_events", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(1234))
	mock.ExpectQuery(`FROM np_saas_usage`).WithArgs(testTenant, "rum_pageviews", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(0))

	rec := doReq(t, g, http.MethodGet, "/v1/me", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d: %s", rec.Code, rec.Body.String())
	}
	var me struct {
		TenantID string                   `json:"tenant_id"`
		Email    string                   `json:"email"`
		Tier     string                   `json:"tier"`
		Quotas   map[string]quotaUsageDTO `json:"quotas"`
	}
	decodeBody(t, rec, &me)
	if me.TenantID != testTenant || me.Email != "a@b.co" || me.Tier != "bundle" {
		t.Errorf("identity wrong: %+v", me)
	}
	if q := me.Quotas["monitors"]; q.Used != 7 || q.Limit != 50 {
		t.Errorf("monitors quota = %+v, want 7/50", q)
	}
	if q := me.Quotas["heartbeats"]; q.Used != 0 || q.Limit != 25 {
		t.Errorf("heartbeats quota = %+v, want 0/25 (missing table = 0)", q)
	}
	if q := me.Quotas["error_events_month"]; q.Used != 1234 || q.Limit != 100_000 {
		t.Errorf("error events quota = %+v", q)
	}
	if q := me.Quotas["seats"]; q.Used != 2 || q.Limit != 3 {
		t.Errorf("seats quota = %+v, want 2/3 (owner + 1 member)", q)
	}
}

// TestUsageEndpoint — GET /v1/usage: tier + the full quota map, every
// counter scoped to the verified tenant.
func TestUsageEndpoint(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("free", []byte(`{}`), nil, time.Now()))
	for _, table := range []string{"np_uptime_targets", "np_cron_monitors",
		"np_saas_status_pages", "np_saas_team_members"} {
		mock.ExpectQuery(`to_regclass`).WithArgs(table).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ` + table).WithArgs(testTenant).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	}
	mock.ExpectQuery(`FROM np_saas_usage`).WithArgs(testTenant, "error_events", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(4999))
	mock.ExpectQuery(`FROM np_saas_usage`).WithArgs(testTenant, "rum_pageviews", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(10))

	rec := doReq(t, g, http.MethodGet, "/v1/usage", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("usage = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Usage struct {
			Tier   string                   `json:"tier"`
			Quotas map[string]quotaUsageDTO `json:"quotas"`
		} `json:"usage"`
	}
	decodeBody(t, rec, &env)
	if env.Usage.Tier != "free" {
		t.Errorf("tier = %q, want free", env.Usage.Tier)
	}
	if q := env.Usage.Quotas["error_events_month"]; q.Used != 4999 || q.Limit != 5_000 {
		t.Errorf("error events = %+v, want 4999/5000", q)
	}
	if q := env.Usage.Quotas["seats"]; q.Used != 2 || q.Limit != 1 {
		t.Errorf("seats = %+v, want 2/1 (free tier over-seat visible)", q)
	}
	if q := env.Usage.Quotas["monitors"]; q.Used != 1 || q.Limit != 10 {
		t.Errorf("monitors = %+v", q)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestCreateStatusPageQuotaExceeded(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	// EffectiveLimits → free tier (1 page)
	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("free", []byte(`{}`), nil, time.Now()))
	// CheckResourceCount → already at 1
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_status_pages`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rec := doReq(t, g, http.MethodPost, "/v1/status-pages/", `{"name":"Second","slug":"second-page"}`)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("over-quota create = %d, want 402: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "quota_exceeded") {
		t.Errorf("missing quota_exceeded marker: %s", rec.Body.String())
	}
}

func TestCreateAndListStatusPages(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("plus", []byte(`{}`), nil, time.Now()))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_status_pages`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`INSERT INTO np_saas_status_pages`).
		WithArgs(testTenant, "Prod Status", "prod-status").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "public", "created_at"}).
			AddRow("p-1", "Prod Status", "prod-status", true, time.Now()))

	rec := doReq(t, g, http.MethodPost, "/v1/status-pages/", `{"name":"Prod Status","slug":"Prod-Status"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		StatusPage statusPageDTO `json:"status_page"`
	}
	decodeBody(t, rec, &env)
	if env.StatusPage.Slug != "prod-status" ||
		env.StatusPage.URL != "https://sentry.nself.org/s/prod-status" {
		t.Errorf("page mapping wrong: %+v", env.StatusPage)
	}

	// Bad slug rejected before any DB work.
	rec = doReq(t, g, http.MethodPost, "/v1/status-pages/", `{"name":"x","slug":"UP"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad slug = %d, want 422", rec.Code)
	}
}

func TestSignupCreatesTenantAndKey(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectExec(`INSERT INTO np_saas_tenants`).
		WithArgs(sqlmock.AnyArg(), "new@user.io", "", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO np_saas_api_keys`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("k-1"))

	req := httptest.NewRequest(http.MethodPost, "/v1/signup", strings.NewReader(`{"email":"New@User.io"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req) // unauthenticated on purpose
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup = %d: %s", rec.Code, rec.Body.String())
	}
	var res struct {
		TenantID string `json:"tenant_id"`
		APIKey   string `json:"api_key"`
		Tier     string `json:"tier"`
	}
	decodeBody(t, rec, &res)
	if res.TenantID == "" || res.Tier != "free" {
		t.Errorf("signup response wrong: %+v", res)
	}
	if !strings.HasPrefix(res.APIKey, "nsk_") || len(res.APIKey) != 68 {
		t.Errorf("api key format wrong: %q", res.APIKey)
	}

	// Garbage email rejected.
	req = httptest.NewRequest(http.MethodPost, "/v1/signup", strings.NewReader(`{"email":"nope"}`))
	rec = httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad email = %d, want 422", rec.Code)
	}
}

func TestInternalTenantTierAuth(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	body := `{"tenant_id":"` + testTenant + `","tier":"bundle","stripe_customer_id":"cus_1"}`

	// Wrong key → 401.
	req := httptest.NewRequest(http.MethodPost, "/internal/billing/tenant-tier", strings.NewReader(body))
	req.Header.Set("X-Internal-Key", "wrong")
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong internal key = %d, want 401", rec.Code)
	}

	// Correct key → upsert.
	mock.ExpectExec(`INSERT INTO np_saas_tenants`).
		WithArgs(testTenant, "bundle", "cus_1", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	req = httptest.NewRequest(http.MethodPost, "/internal/billing/tenant-tier", strings.NewReader(body))
	req.Header.Set("X-Internal-Key", "test-internal-secret")
	rec = httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tier update = %d: %s", rec.Code, rec.Body.String())
	}

	// Bogus tier rejected.
	req = httptest.NewRequest(http.MethodPost, "/internal/billing/tenant-tier",
		strings.NewReader(`{"tenant_id":"`+testTenant+`","tier":"platinum"}`))
	req.Header.Set("X-Internal-Key", "test-internal-secret")
	rec = httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bogus tier = %d, want 422", rec.Code)
	}
}

// TestTenantTierHeaderAndTierAliases covers the cross-box billing path:
// the ping_api client sends X-NSentry-Internal-Key and SaaS-plan tier
// aliases ('sentry') that must normalize to the canonical 'bundle', plus the
// 'email' field alias for owner_email.
func TestTenantTierHeaderAndTierAliases(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	// 'sentry' alias + 'email' alias, presented via X-NSentry-Internal-Key.
	mock.ExpectExec(`INSERT INTO np_saas_tenants`).
		WithArgs(testTenant, "bundle", "cus_9", "buyer@x.io").
		WillReturnResult(sqlmock.NewResult(0, 1))
	body := `{"tenant_id":"` + testTenant + `","tier":"sentry","stripe_customer_id":"cus_9","email":"buyer@x.io"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/billing/tenant-tier", strings.NewReader(body))
	req.Header.Set("X-NSentry-Internal-Key", "test-internal-secret")
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sentry alias tier update = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Tier string `json:"tier"`
	}
	decodeBody(t, rec, &env)
	if env.Tier != "bundle" {
		t.Errorf("tier alias not normalized: got %q, want bundle", env.Tier)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestTenantTierIPAllowlist verifies the source-IP gate: a request from a
// non-allowlisted RemoteAddr is 403'd BEFORE the key is even checked.
func TestTenantTierIPAllowlist(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	g.cfg.InternalAllowIPs = map[string]struct{}{"5.75.235.42": {}, "127.0.0.1": {}}

	body := `{"tenant_id":"` + testTenant + `","tier":"plus"}`
	// Disallowed source IP → 403.
	req := httptest.NewRequest(http.MethodPost, "/internal/billing/tenant-tier", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.9:44444"
	req.Header.Set("X-NSentry-Internal-Key", "test-internal-secret")
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disallowed IP = %d, want 403", rec.Code)
	}
}

// TestUpdateStatusPageTenantScoped verifies a PATCH updates the caller's own
// page, and a cross-tenant id (0 rows) returns 404 without acting.
func TestUpdateStatusPageTenantScoped(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	pageID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	// Owned page → RETURNING one row.
	mock.ExpectQuery(`UPDATE np_saas_status_pages`).
		WithArgs(pageID, testTenant, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "public", "created_at"}).
			AddRow(pageID, "Renamed", "my-page", false, time.Now()))
	rec := doReq(t, g, http.MethodPatch, "/v1/status-pages/"+pageID, `{"title":"Renamed","published":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch owned page = %d: %s", rec.Code, rec.Body.String())
	}

	// Cross-tenant id → 0 rows → 404 (never acts).
	mock.ExpectQuery(`UPDATE np_saas_status_pages`).
		WithArgs(pageID, testTenant, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)
	rec = doReq(t, g, http.MethodPatch, "/v1/status-pages/"+pageID, `{"public":true}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant patch = %d, want 404", rec.Code)
	}
}

// TestDeleteStatusPageTenantScoped verifies DELETE removes the caller's page
// and a cross-tenant id (0 rows) returns 404 without acting.
func TestDeleteStatusPageTenantScoped(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	pageID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	mock.ExpectExec(`DELETE FROM np_saas_status_pages`).
		WithArgs(pageID, testTenant).WillReturnResult(sqlmock.NewResult(0, 1))
	if rec := doReq(t, g, http.MethodDelete, "/v1/status-pages/"+pageID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete own page = %d, want 204", rec.Code)
	}

	mock.ExpectExec(`DELETE FROM np_saas_status_pages`).
		WithArgs(pageID, testTenant).WillReturnResult(sqlmock.NewResult(0, 0))
	if rec := doReq(t, g, http.MethodDelete, "/v1/status-pages/"+pageID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete cross-tenant page = %d, want 404", rec.Code)
	}
}

func TestInternalDisabledWithoutSecret(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	g.cfg.InternalAPIKey = ""

	req := httptest.NewRequest(http.MethodPost, "/internal/billing/tenant-tier",
		strings.NewReader(`{"tenant_id":"x","tier":"plus"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unset secret = %d, want 503 (fail-closed)", rec.Code)
	}
}
