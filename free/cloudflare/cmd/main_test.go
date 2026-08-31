// Purpose: unit tests for cmd/main.go route wiring (newRouter, newServer).
// Inputs: a pgxmock-backed internal.DB, httptest requests.
// Outputs: asserts routes are wired to the expected handlers and the server
// struct carries the expected address/timeouts.
// Constraints: does not start a real listener or touch a live database.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/nself-cloudflare/internal"
	"github.com/pashagolub/pgxmock/v4"
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

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /health status=%d want 200", w.Code)
	}
}

func TestNewRouter_Ready(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectPing().WillReturnError(nil)
	r := newRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /ready status=%d want 200", w.Code)
	}
}

func TestNewRouter_ZonesRouteWired(t *testing.T) {
	db, mock := newTestDB(t)
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "name", "status", "created_at", "synced_at"})
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_zones").WithArgs(pgxmock.AnyArg()).WillReturnRows(rows)
	r := newRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/zones status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNewRouter_UnknownRoute404(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestNewRouter_StatsAndAnalyticsWired(t *testing.T) {
	db, mock := newTestDB(t)
	for _, table := range []string{"cf_zones", "cf_dns_records", "cf_r2_buckets", "cf_cache_purge_log", "cf_analytics"} {
		rows := pgxmock.NewRows([]string{"count"}).AddRow(int64(0))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM " + table).WithArgs(pgxmock.AnyArg()).WillReturnRows(rows)
	}
	r := newRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/stats status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestServerAddr(t *testing.T) {
	cfg := internal.Config{Host: "0.0.0.0", Port: 3024}
	if got := serverAddr(cfg); got != "0.0.0.0:3024" {
		t.Errorf("serverAddr = %q", got)
	}
}

func TestNewServer_Wiring(t *testing.T) {
	db, _ := newTestDB(t)
	r := newRouter(db)
	srv := newServer("127.0.0.1:9999", r)

	if srv.Addr != "127.0.0.1:9999" {
		t.Errorf("Addr = %q", srv.Addr)
	}
	if srv.Handler == nil {
		t.Error("Handler should not be nil")
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 60*time.Second {
		t.Errorf("WriteTimeout = %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v", srv.IdleTimeout)
	}
}
