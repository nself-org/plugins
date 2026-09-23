package sdk

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Without env vars, defaults should apply
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	cfg := LoadConfig()
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
}

func TestLoadConfig_CustomPort(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	cfg := LoadConfig()
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Errorf("DatabaseURL = %q, want postgres://localhost/test", cfg.DatabaseURL)
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	t.Setenv("PORT", "not-a-number")
	cfg := LoadConfig()
	if cfg.Port != 3000 {
		t.Errorf("invalid PORT should default to 3000, got %d", cfg.Port)
	}
}

func TestNewServer(t *testing.T) {
	s := NewServer(9999)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.Router() == nil {
		t.Error("Router() returned nil")
	}
}

// TestNewServer_RouterUseAfterConstruction is the direct regression guard
// for PR #104: sdk.NewServer(port) must return a router with NO routes
// registered yet, so that the standard plugin pattern —
//
//	srv := sdk.NewServer(port)
//	r := srv.Router()
//	r.Use(sdk.Recovery)   // <- panicked here before this fix
//
// — never hits chi's "all middlewares must be defined before routes on a
// mux" panic. #104 shipped NewServer pre-registering /healthz, /health,
// /readyz at construction time, which broke this exact call sequence in
// every plugin built on it (notify, cron: nself golden-path E2E,
// 2026-09-21). The #104 unit test never called Use() after NewServer, so
// it never caught this.
func TestNewServer_RouterUseAfterConstruction(t *testing.T) {
	s := NewServer(9999)
	r := s.Router()
	// Must not panic.
	r.Use(Recovery)
	r.Use(Logger)
	r.Get("/v1/ping", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestListenAndServe_DefaultHealthRoutes mirrors a real plugin main: build a
// Server via NewServer, call Router().Use(...) for middleware, register a
// plugin route, and only then exercise the health routes through the same
// registration path ListenAndServe uses (ensureHealthRoutes), rather than
// hitting NewServer's router directly — the #104 test's gap. Regression
// guard: plugins built on sdk.NewServer (notify, cron, and 19 others)
// previously shipped with no health route at all, so the container
// healthcheck 404'd and reported them permanently unhealthy; the #104 fix
// for that then broke Router().Use() with a chi panic (nself CLI
// golden-path E2E, 2026-09-21).
func TestListenAndServe_DefaultHealthRoutes(t *testing.T) {
	s := NewServer(9999)
	r := s.Router()
	r.Use(Recovery)
	r.Use(Logger)
	r.Get("/v1/ping", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Exercise the exact registration path ListenAndServe uses, without
	// actually binding a listener.
	ensureHealthRoutes(r)

	for _, path := range []string{"/healthz", "/health", "/readyz"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, w.Code)
		}
	}
}

// TestListenAndServe_HeadHealthRoutes is the direct regression guard for the
// nself golden-path E2E failure on free/cron (2026-09-22): the CLI's
// docker-compose healthcheck template runs `wget --spider`, which sends
// HEAD, not GET. Before this fix, ensureHealthRoutes registered GET only,
// so HEAD /health fell through to chi's default 405 handler and the
// container never reported healthy even though GET /health returned 200.
// Mirrors TestListenAndServe_DefaultHealthRoutes: build a Server via
// NewServer, register middleware and a plugin route, then exercise HEAD
// through the same registration path ListenAndServe uses
// (ensureHealthRoutes).
func TestListenAndServe_HeadHealthRoutes(t *testing.T) {
	s := NewServer(9999)
	r := s.Router()
	r.Use(Recovery)
	r.Use(Logger)
	r.Get("/v1/ping", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ensureHealthRoutes(r)

	for _, path := range []string{"/healthz", "/health", "/readyz"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodHead, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("HEAD %s: status = %d, want 200", path, w.Code)
		}
		if w.Body.Len() != 0 {
			t.Errorf("HEAD %s: body = %q, want empty", path, w.Body.String())
		}
	}
}

// TestEnsureHealthRoutes_PluginRouteWins verifies a plugin-registered
// /health handler is NOT overridden by ensureHealthRoutes — an explicit
// plugin route must always win over the default liveness responder.
func TestEnsureHealthRoutes_PluginRouteWins(t *testing.T) {
	s := NewServer(9999)
	r := s.Router()
	r.Use(Recovery)

	const sentinelBody = `{"custom":"health"}`
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sentinelBody))
	})

	ensureHealthRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/health: status = %d, want 200", w.Code)
	}
	if w.Body.String() != sentinelBody {
		t.Errorf("/health: body = %q, want plugin's own %q (default must not override an explicit plugin route)", w.Body.String(), sentinelBody)
	}

	// The two routes ensureHealthRoutes still owns must still be present.
	for _, path := range []string{"/healthz", "/readyz"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, w.Code)
		}
	}

	// The plugin registered its own GET /health but no HEAD /health, so
	// ensureHealthRoutes must still supply the default HEAD handler — GET
	// and HEAD are independent entries in chi's routing tree.
	hw := httptest.NewRecorder()
	hreq := httptest.NewRequest(http.MethodHead, "/health", nil)
	r.ServeHTTP(hw, hreq)
	if hw.Code != http.StatusOK {
		t.Fatalf("HEAD /health: status = %d, want 200", hw.Code)
	}
	if hw.Body.Len() != 0 {
		t.Errorf("HEAD /health: body = %q, want empty (plugin's GET body must not leak into the default HEAD handler)", hw.Body.String())
	}
}

func TestRecovery_Passthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := Recovery(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("Recovery passthrough status = %d, want 200", w.Code)
	}
}

func TestLogger_Passthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := Logger(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("Logger passthrough status = %d, want 200", w.Code)
	}
}

func TestCORS_Passthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := CORS(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("CORS passthrough status = %d, want 200", w.Code)
	}
}

func TestSourceAccountID_AllSpellings(t *testing.T) {
	cases := []struct {
		header string
		value  string
		want   string
	}{
		{"X-Source-Account-ID", "acct-1", "acct-1"},
		{"X-Source-Account-Id", "acct-2", "acct-2"},
		{"X-Hasura-Source-Account-Id", "acct-3", "acct-3"},
		{"X-Source-Account", "acct-4", "acct-4"},
		{"", "", "primary"}, // no header → default
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			r.Header.Set(tc.header, tc.value)
		}
		got := SourceAccountID(r)
		if got != tc.want {
			t.Errorf("SourceAccountID with header %q = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestRequestID_Passthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequestID(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("RequestID passthrough status = %d, want 200", w.Code)
	}
}
