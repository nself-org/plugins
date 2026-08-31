// Purpose: Cover the router wiring extracted from main() into newRouter(),
// without booting a real HTTP server or database connection.
// Inputs: httptest requests against a router built with stub Handlers.
// Outputs: asserted response status for known + unknown routes.
// Constraints: main() itself (DB connect + ListenAndServe + signal handling)
// is not unit-testable; this file covers everything reachable without it.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/nself-rom-discovery/internal"
)

// fakeShutdowner is a stub shutdowner for exercising shutdown()'s success
// and error branches without binding a real listener.
type fakeShutdowner struct {
	err error
}

func (f fakeShutdowner) Shutdown(ctx context.Context) error { return f.err }

func TestShutdown_Success(t *testing.T) {
	if err := shutdown(fakeShutdowner{}, time.Second); err != nil {
		t.Errorf("shutdown() = %v; want nil", err)
	}
}

func TestShutdown_Error(t *testing.T) {
	want := errors.New("timed out")
	err := shutdown(fakeShutdowner{err: want}, time.Second)
	if err != want {
		t.Errorf("shutdown() = %v; want %v", err, want)
	}
}

func TestNewServer_CustomPort(t *testing.T) {
	srv := newServer(internal.Config{Port: "9999"}, http.NotFoundHandler())
	if srv.Addr != ":9999" {
		t.Errorf("Addr = %q; want :9999", srv.Addr)
	}
}

func TestNewServer_AddrAndTimeouts(t *testing.T) {
	h := internal.NewHandlers(&internal.DB{})
	r := newRouter(h)
	srv := newServer(internal.Config{Port: "3125"}, r)

	if srv.Addr != ":3125" {
		t.Errorf("Addr = %q; want :3125", srv.Addr)
	}
	if srv.Handler != r {
		t.Error("Handler not wired to router")
	}
	if srv.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 15*time.Second {
		t.Errorf("WriteTimeout = %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v", srv.IdleTimeout)
	}
}

func TestNewRouter_Health(t *testing.T) {
	h := internal.NewHandlers(&internal.DB{})
	r := newRouter(h)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /health = %d; want 200", w.Code)
	}
}

func TestNewRouter_NotFound(t *testing.T) {
	h := internal.NewHandlers(&internal.DB{})
	r := newRouter(h)

	req := httptest.NewRequest("GET", "/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// TestNewRouter_AllRoutesRegistered exercises every registered API route so
// the router-wiring branch inside newRouter() is fully covered. Handlers
// will fail against a nil Pool, but the assertion here is only that chi
// successfully dispatched to a handler (i.e. no 404/405), not that the
// handler itself succeeds.
func TestNewRouter_AllRoutesRegistered(t *testing.T) {
	h := internal.NewHandlers(&internal.DB{})
	r := newRouter(h)

	cases := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/metadata"},
		{"GET", "/api/v1/metadata/abc"},
		{"DELETE", "/api/v1/metadata/abc"},
		{"GET", "/api/v1/downloads"},
		{"PATCH", "/api/v1/downloads/abc"},
		{"GET", "/api/v1/scrapers"},
		{"PATCH", "/api/v1/scrapers/abc"},
		{"GET", "/api/v1/popularity"},
		{"GET", "/api/v1/audit"},
		{"GET", "/api/v1/legal"},
	}

	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		w := httptest.NewRecorder()

		func() {
			defer func() { recover() }() // handlers may panic on a nil Pool; router dispatch already ran.
			r.ServeHTTP(w, req)
		}()

		if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d; route should be registered", c.method, c.path, w.Code)
		}
	}
}
