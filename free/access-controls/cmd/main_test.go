// Purpose: Cover the pure-logic helpers wired into cmd/main.go (CORS origin
// allowlist parsing, CORS middleware, API key middleware) without booting a
// real HTTP server or database connection.
// Inputs: httptest requests + env vars.
// Outputs: asserted response headers/status for allow/deny branches.
// Constraints: main() itself (DB connect + ListenAndServe + signal handling)
// is not unit-testable; this file covers everything reachable without it.
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nself-org/nself-access-controls/internal"
)

// TestNewRouter_HealthNoAuth verifies the health endpoint is reachable
// without an API key, and that unknown routes 404, covering the router
// wiring logic extracted from main() into newRouter().
func TestNewRouter_HealthNoAuth(t *testing.T) {
	h := internal.NewHandlers(&internal.DB{}, internal.Config{})
	r := newRouter(h, internal.Config{})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /health = %d; want 200", w.Code)
	}
}

func TestNewRouter_APIKeyRequiredWhenConfigured(t *testing.T) {
	h := internal.NewHandlers(&internal.DB{}, internal.Config{})
	r := newRouter(h, internal.Config{APIKey: "topsecret"})

	req := httptest.NewRequest("POST", "/api/v1/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401 without API key", w.Code)
	}
}

func TestNewRouter_NotFound(t *testing.T) {
	h := internal.NewHandlers(&internal.DB{}, internal.Config{})
	r := newRouter(h, internal.Config{})

	req := httptest.NewRequest("GET", "/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestAllowedOriginsSet_Empty(t *testing.T) {
	os.Unsetenv("TEST_ORIGINS_EMPTY")
	got := allowedOriginsSet("TEST_ORIGINS_EMPTY")
	if len(got) != 0 {
		t.Errorf("expected empty set, got %v", got)
	}
}

func TestAllowedOriginsSet_WildcardIgnored(t *testing.T) {
	os.Setenv("TEST_ORIGINS_WILDCARD", "*")
	defer os.Unsetenv("TEST_ORIGINS_WILDCARD")
	got := allowedOriginsSet("TEST_ORIGINS_WILDCARD")
	if len(got) != 0 {
		t.Errorf("wildcard should never be accepted, got %v", got)
	}
}

func TestAllowedOriginsSet_ParsesCSVAndNormalizes(t *testing.T) {
	os.Setenv("TEST_ORIGINS_CSV", " HTTPS://Example.com , https://foo.test ,, *")
	defer os.Unsetenv("TEST_ORIGINS_CSV")
	got := allowedOriginsSet("TEST_ORIGINS_CSV")
	if !got["https://example.com"] {
		t.Errorf("expected lowercased+trimmed origin in set: %v", got)
	}
	if !got["https://foo.test"] {
		t.Errorf("expected second origin in set: %v", got)
	}
	if got["*"] {
		t.Error("wildcard must never be in the allowed set")
	}
	if len(got) != 2 {
		t.Errorf("expected exactly 2 entries, got %v", got)
	}
}

func TestCorsMiddleware_AllowedOrigin(t *testing.T) {
	os.Setenv("ACCESS_CONTROLS_ALLOWED_ORIGINS", "https://example.com")
	defer os.Unsetenv("ACCESS_CONTROLS_ALLOWED_ORIGINS")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := corsMiddleware(next)

	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !called {
		t.Error("expected next handler to be called for allowed origin")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("ACAO header = %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCorsMiddleware_DisallowedOrigin_StillCallsNext(t *testing.T) {
	os.Setenv("ACCESS_CONTROLS_ALLOWED_ORIGINS", "https://example.com")
	defer os.Unsetenv("ACCESS_CONTROLS_ALLOWED_ORIGINS")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := corsMiddleware(next)

	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !called {
		t.Error("non-preflight requests always reach next, even for disallowed origin")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("ACAO header should not be set for disallowed origin")
	}
}

func TestCorsMiddleware_PreflightAllowed(t *testing.T) {
	os.Setenv("ACCESS_CONTROLS_ALLOWED_ORIGINS", "https://example.com")
	defer os.Unsetenv("ACCESS_CONTROLS_ALLOWED_ORIGINS")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called for OPTIONS preflight")
	})
	h := corsMiddleware(next)

	r := httptest.NewRequest(http.MethodOptions, "/x", nil)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d; want 204", w.Code)
	}
}

func TestCorsMiddleware_PreflightDisallowed(t *testing.T) {
	os.Setenv("ACCESS_CONTROLS_ALLOWED_ORIGINS", "https://example.com")
	defer os.Unsetenv("ACCESS_CONTROLS_ALLOWED_ORIGINS")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called for disallowed OPTIONS preflight")
	})
	h := corsMiddleware(next)

	r := httptest.NewRequest(http.MethodOptions, "/x", nil)
	r.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", w.Code)
	}
}

func TestApiKeyMiddleware_ValidHeaderKey(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := apiKeyMiddleware("secret123")(next)

	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-API-Key", "secret123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !called {
		t.Error("expected next to be called with valid API key")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestApiKeyMiddleware_ValidBearerToken(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := apiKeyMiddleware("secret123")(next)

	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !called {
		t.Error("expected next to be called with valid bearer token")
	}
}

func TestApiKeyMiddleware_MissingKey(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called without API key")
	})
	h := apiKeyMiddleware("secret123")(next)

	r := httptest.NewRequest("GET", "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

func TestApiKeyMiddleware_WrongKey(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called with wrong API key")
	})
	h := apiKeyMiddleware("secret123")(next)

	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-API-Key", "wrong")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

func TestApiKeyMiddleware_NonBearerAuthorizationTreatedAsRawKey(t *testing.T) {
	// "secret123" has no "Bearer " prefix, so the strip is skipped and the
	// raw Authorization header value is compared directly to the key.
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := apiKeyMiddleware("secret123")(next)

	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "secret123") // no "Bearer " prefix
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !called {
		t.Error("expected next to be called: raw Authorization value equals key")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
}

func TestApiKeyMiddleware_BearerPrefixButShortValue(t *testing.T) {
	// len("Bear") == 4, not > 7, so the Bearer-strip branch is skipped and
	// the raw value is compared — covers the len(got) > 7 guard.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	})
	h := apiKeyMiddleware("secret123")(next)

	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Authorization", "Bear")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}
