// Purpose: Route-wiring tests for the web3 plugin's cmd/main.
// Inputs: httptest requests against the router built by newRouter, with a mocked pgx pool.
// Outputs: asserts route dispatch (200/404/204) and JSON health body.
// Constraints: No real Postgres, no real port binding; exercises newRouter only, never main().
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pashagolub/pgxmock/v3"

	"github.com/nself-org/nself-web3/internal"
)

func newTestHandlers(t *testing.T) (*internal.Handlers, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return internal.NewHandlers(mock), mock
}

func TestNewRouter_Health(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
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
	if out["plugin"] != "web3" {
		t.Errorf("plugin = %q", out["plugin"])
	}
}

func TestNewRouter_UnknownRoute404(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestNewRouter_Ready_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectPing()
	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestNewRouter_WalletsRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "user_id", "workspace_id", "address", "chain_id", "chain_name",
			"wallet_type", "ens_name", "label", "is_primary", "is_active", "created_at", "updated_at",
		}))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wallets/", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_RPCRouteDispatch(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc/999999", nil)
	r.ServeHTTP(rec, req)
	// chain 999999 is not configured via env -> 503, proving route dispatch works
	// without touching the DB.
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_GateStatsRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"total_checks", "passed_checks", "failed_checks", "last_checked_at"}).
			AddRow(0, 0, 0, nil))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate-stats", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
