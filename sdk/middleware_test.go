package sdk

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler is a trivial 200 OK handler used as the "next" in middleware tests.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// TestAllowedCallers_MissingHeader verifies that a request with no
// X-Source-Plugin header is rejected with 403 when StrictPluginAuth=true.
// S43-T02 acceptance criterion: "cross-plugin call without X-Source-Plugin → 403".
func TestAllowedCallers_MissingHeader(t *testing.T) {
	cfg := &Config{
		AllowedCallers:   map[string]bool{"claw": true},
		StrictPluginAuth: true,
	}
	handler := AllowedCallers(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/complete", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("missing X-Source-Plugin: got %d, want 403", rr.Code)
	}
}

// TestAllowedCallers_UnauthorisedCaller verifies that a request from a plugin
// not in AllowedCallers is rejected with 403. S43-T02 acceptance criterion:
// "call from non-allowlist source → 403".
func TestAllowedCallers_UnauthorisedCaller(t *testing.T) {
	cfg := &Config{
		AllowedCallers:   map[string]bool{"claw": true},
		StrictPluginAuth: true,
	}
	handler := AllowedCallers(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/complete", nil)
	req.Header.Set("X-Source-Plugin", "voice") // not in allowlist
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("unauthorised caller: got %d, want 403", rr.Code)
	}
}

// TestAllowedCallers_AuthorisedCaller verifies that a request from an allowlisted
// plugin is passed through with 200. S43-T02 acceptance criterion:
// "call from allowlist source → success".
func TestAllowedCallers_AuthorisedCaller(t *testing.T) {
	cfg := &Config{
		AllowedCallers:   map[string]bool{"claw": true, "mux": true},
		StrictPluginAuth: true,
	}
	handler := AllowedCallers(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/complete", nil)
	req.Header.Set("X-Source-Plugin", "claw")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("authorised caller: got %d, want 200", rr.Code)
	}
}

// TestAllowedCallers_DevBypass verifies that STRICT_PLUGIN_AUTH=false bypasses
// the check entirely, allowing any caller through. S43-T02 acceptance criterion:
// "Permissive in dev (STRICT_PLUGIN_AUTH=false) by default".
func TestAllowedCallers_DevBypass(t *testing.T) {
	cfg := &Config{
		AllowedCallers:   map[string]bool{}, // empty — would block all in strict mode
		StrictPluginAuth: false,             // dev bypass
	}
	handler := AllowedCallers(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/complete", nil)
	// No X-Source-Plugin header at all.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("dev bypass: got %d, want 200", rr.Code)
	}
}

// TestAllowedCallers_CaseInsensitive verifies that header values are compared
// case-insensitively (X-Source-Plugin: CLAW matches allowlist entry "claw").
func TestAllowedCallers_CaseInsensitive(t *testing.T) {
	cfg := &Config{
		AllowedCallers:   map[string]bool{"claw": true},
		StrictPluginAuth: true,
	}
	handler := AllowedCallers(cfg)(okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/complete", nil)
	req.Header.Set("X-Source-Plugin", "CLAW") // upper-case
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("case-insensitive header: got %d, want 200", rr.Code)
	}
}

// TestParseAllowedCallers verifies CSV parsing strips whitespace and lowercases.
func TestParseAllowedCallers(t *testing.T) {
	got := parseAllowedCallers("claw, MUX ,  voice,cron")
	expected := map[string]bool{"claw": true, "mux": true, "voice": true, "cron": true}
	for k := range expected {
		if !got[k] {
			t.Errorf("parseAllowedCallers: missing %q", k)
		}
	}
	if len(got) != len(expected) {
		t.Errorf("parseAllowedCallers: got %d entries, want %d", len(got), len(expected))
	}
}
