// Purpose: Route-wiring tests for the compliance plugin's cmd/main.
// Inputs: httptest requests against the router built by newRouter, with a mocked pgx pool.
// Outputs: asserts route dispatch (200/404) and JSON health body.
// Constraints: No real Postgres, no real port binding; exercises newRouter only, never main().
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pashagolub/pgxmock/v3"

	"github.com/nself-org/nself-compliance/internal"
)

func newTestDB(t *testing.T) (*internal.DB, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return &internal.DB{Pool: mock}, mock
}

func TestNewRouter_Health(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["plugin"] != "compliance" {
		t.Errorf("plugin = %q", out["plugin"])
	}
}

func TestNewRouter_UnknownRoute404(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestNewRouter_Ready(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectPing()
	r := newRouter(db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_StatsRouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	for i := 0; i < 11; i++ {
		mock.ExpectQuery("SELECT COUNT").WithArgs(pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	}
	r := newRouter(db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_SOC2ControlsRouteDispatch(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/soc2/controls", nil)
	req.Header.Set("X-Nself-License-Tier", "business")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_SOC2ControlsRouteDispatch_Unlicensed(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/soc2/controls", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_DSARsRouteDispatch(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "request_type", "request_number", "user_id", "requester_email",
			"requester_name", "description", "data_categories", "status", "assigned_to", "started_at",
			"completed_at", "deadline", "resolution_notes", "rejection_reason", "regulation",
			"jurisdiction", "legal_basis", "ip_address", "user_agent", "created_at", "updated_at",
		}))
	r := newRouter(db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dsars", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_ChangeLogRouteDispatch_Unlicensed(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance/change-log", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
