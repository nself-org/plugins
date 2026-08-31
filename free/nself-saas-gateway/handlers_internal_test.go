package main

// handlers_internal_test.go — /internal/tenants/{id} purge auth + flow.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const purgeTenant = "44444444-3333-4222-8111-000000000000"

func internalReq(method, path, key string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	return req
}

func TestDeleteTenantRequiresInternalKey(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, internalReq(http.MethodDelete, "/internal/tenants/"+purgeTenant, ""))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no key = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	g.router().ServeHTTP(rec, internalReq(http.MethodDelete, "/internal/tenants/"+purgeTenant, "wrong-key"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong key = %d, want 401", rec.Code)
	}

	// A tenant credential (JWT) is NOT an internal key.
	rec = doReq(t, g, http.MethodDelete, "/internal/tenants/"+purgeTenant, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("tenant JWT on internal route = %d, want 401", rec.Code)
	}
}

func TestDeleteTenantPurges(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery(`FROM information_schema.columns`).
		WillReturnRows(sqlmock.NewRows([]string{"table"}).
			AddRow("public.np_saas_api_keys").
			AddRow("public.np_uptime_targets"))
	mock.ExpectExec(`DELETE FROM public.np_saas_api_keys WHERE tenant_id`).
		WithArgs(purgeTenant).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM public.np_uptime_targets WHERE tenant_id`).
		WithArgs(purgeTenant).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM np_saas_tenants WHERE tenant_id`).
		WithArgs(purgeTenant).WillReturnResult(sqlmock.NewResult(0, 1))

	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, internalReq(http.MethodDelete, "/internal/tenants/"+purgeTenant, "test-internal-secret"))
	if rec.Code != http.StatusOK {
		t.Fatalf("purge = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		TenantID string           `json:"tenant_id"`
		Deleted  map[string]int64 `json:"deleted"`
	}
	decodeBody(t, rec, &resp)
	if resp.Deleted["public.np_saas_api_keys"] != 2 || resp.Deleted["np_saas_tenants"] != 1 {
		t.Errorf("deleted = %+v", resp.Deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDeleteTenantRejectsNonUUID(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, internalReq(http.MethodDelete, "/internal/tenants/not-a-uuid", "test-internal-secret"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("non-uuid = %d, want 422", rec.Code)
	}
}
