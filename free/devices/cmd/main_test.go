// Purpose: unit tests for cmd/main.go route wiring (newRouter, newServer).
// Inputs: a pgxmock-backed internal.Handlers, httptest requests.
// Outputs: asserts routes are wired to the expected handlers and the server
// struct carries the expected address/timeouts.
// Constraints: does not start a real listener or touch a live database.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/nself-devices/internal"
	"github.com/pashagolub/pgxmock/v4"
)

func newTestHandlers(t *testing.T) (*internal.Handlers, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return &internal.Handlers{DB: &internal.DB{Pool: mock}, Cfg: internal.Config{}}, mock
}

func TestNewRouter_Health(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /health status=%d want 200", w.Code)
	}
}

func TestNewRouter_Ready(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectPing().WillReturnError(nil)
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /ready status=%d want 200", w.Code)
	}
}

func TestNewRouter_DevicesRouteWired(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{
		"id", "source_account_id", "app_id", "device_id", "name", "device_type", "model",
		"firmware_version", "status", "trust_level", "last_seen_at", "capabilities",
		"config", "labels", "metadata", "created_at", "updated_at",
	})
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_devices").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(rows)
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/devices status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNewRouter_TelemetryRouteWired(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{
		"id", "source_account_id", "app_id", "device_id", "telemetry_type", "data",
		"recorded_at", "received_at",
	})
	mock.ExpectQuery("SELECT (.|\n)*FROM np_dev_telemetry").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(rows)
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/telemetry status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNewRouter_UnknownRoute404(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestServerAddr(t *testing.T) {
	cfg := internal.Config{Host: "0.0.0.0", Port: 3603}
	if got := serverAddr(cfg); got != "0.0.0.0:3603" {
		t.Errorf("serverAddr = %q", got)
	}
}

func TestNewServer_Wiring(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	srv := newServer("127.0.0.1:9999", r)

	if srv.Addr != "127.0.0.1:9999" {
		t.Errorf("Addr = %q", srv.Addr)
	}
	if srv.Handler == nil {
		t.Error("Handler should not be nil")
	}
	if srv.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v", srv.IdleTimeout)
	}
}

func TestRequireDatabaseURL_Missing(t *testing.T) {
	if err := requireDatabaseURL(internal.Config{}); err == nil {
		t.Error("expected error when DatabaseURL is empty")
	}
}

func TestRequireDatabaseURL_Present(t *testing.T) {
	if err := requireDatabaseURL(internal.Config{DatabaseURL: "postgres://x"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShutdownServer_Graceful(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	srv := newServer("127.0.0.1:0", r)

	if err := shutdownServer(srv, time.Second); err != nil {
		t.Errorf("shutdownServer on a never-started server should not error: %v", err)
	}
}
