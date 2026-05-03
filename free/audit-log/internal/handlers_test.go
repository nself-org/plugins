package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestAuthorizeAdminRequest_PluginSecret verifies that a valid X-Plugin-Secret
// header grants access.
func TestAuthorizeAdminRequest_PluginSecret(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Plugin-Secret", "my-secret")
	if !authorizeAdminRequest(r, "my-secret", "") {
		t.Error("expected authorizeAdminRequest to return true for valid plugin secret")
	}
}

// TestAuthorizeAdminRequest_AdminSecret verifies that a valid
// X-Hasura-Admin-Secret header grants access when adminSecret is non-empty.
func TestAuthorizeAdminRequest_AdminSecret(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Hasura-Admin-Secret", "admin-secret")
	if !authorizeAdminRequest(r, "plugin-secret", "admin-secret") {
		t.Error("expected authorizeAdminRequest to return true for valid admin secret")
	}
}

// TestAuthorizeAdminRequest_WrongSecret verifies that a wrong secret is
// rejected.
func TestAuthorizeAdminRequest_WrongSecret(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Plugin-Secret", "wrong")
	if authorizeAdminRequest(r, "correct", "") {
		t.Error("expected authorizeAdminRequest to return false for wrong secret")
	}
}

// TestAuthorizeAdminRequest_NoHeader verifies that missing headers are
// rejected.
func TestAuthorizeAdminRequest_NoHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if authorizeAdminRequest(r, "secret", "admin") {
		t.Error("expected authorizeAdminRequest to return false when no auth header present")
	}
}

// TestAuthorizeAdminRequest_AdminSecretDisabled verifies that the admin secret
// path is not triggered when adminSecret is empty.
func TestAuthorizeAdminRequest_AdminSecretDisabled(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Hasura-Admin-Secret", "some-admin-secret")
	// adminSecret param is empty — the admin-secret path must be skipped.
	if authorizeAdminRequest(r, "plugin-secret", "") {
		t.Error("expected authorizeAdminRequest to return false when adminSecret is disabled")
	}
}

// TestParseQueryFilter_Defaults verifies that an empty query string produces
// the default QueryFilter (limit=50).
func TestParseQueryFilter_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/events", nil)
	f, err := parseQueryFilter(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Limit != 50 {
		t.Errorf("default limit = %d, want 50", f.Limit)
	}
}

// TestParseQueryFilter_ValidParams verifies that known valid params are parsed.
func TestParseQueryFilter_ValidParams(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/events?event_type=auth.login&severity=info&limit=100&offset=10", nil)
	f, err := parseQueryFilter(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.EventType != "auth.login" {
		t.Errorf("EventType = %q, want %q", f.EventType, "auth.login")
	}
	if f.Severity != "info" {
		t.Errorf("Severity = %q, want %q", f.Severity, "info")
	}
	if f.Limit != 100 {
		t.Errorf("Limit = %d, want 100", f.Limit)
	}
	if f.Offset != 10 {
		t.Errorf("Offset = %d, want 10", f.Offset)
	}
}

// TestParseQueryFilter_LimitCap verifies that limit is capped at 1000.
func TestParseQueryFilter_LimitCap(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/events?limit=9999", nil)
	f, err := parseQueryFilter(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Limit != 1000 {
		t.Errorf("capped limit = %d, want 1000", f.Limit)
	}
}

// TestParseQueryFilter_InvalidLimit verifies that a non-numeric limit returns
// an error.
func TestParseQueryFilter_InvalidLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/events?limit=abc", nil)
	if _, err := parseQueryFilter(r); err == nil {
		t.Error("expected error for invalid limit, got nil")
	}
}

// TestParseQueryFilter_InvalidEventType verifies that an unknown event_type
// value returns an error.
func TestParseQueryFilter_InvalidEventType(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/events?event_type=unknown.event", nil)
	if _, err := parseQueryFilter(r); err == nil {
		t.Error("expected error for unknown event_type, got nil")
	}
}

// TestParseQueryFilter_InvalidSeverity verifies that an unknown severity
// returns an error.
func TestParseQueryFilter_InvalidSeverity(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/events?severity=extreme", nil)
	if _, err := parseQueryFilter(r); err == nil {
		t.Error("expected error for unknown severity, got nil")
	}
}

// TestParseQueryFilter_ValidTimeRange verifies that RFC3339 from/to
// parameters parse without error.
func TestParseQueryFilter_ValidTimeRange(t *testing.T) {
	from := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	r := httptest.NewRequest(http.MethodGet, "/events?from="+from+"&to="+to, nil)
	f, err := parseQueryFilter(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.From == nil || f.To == nil {
		t.Error("expected From and To to be non-nil after parsing valid timestamps")
	}
}

// TestParseQueryFilter_InvalidFrom verifies that a malformed from timestamp
// returns an error.
func TestParseQueryFilter_InvalidFrom(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/events?from=not-a-date", nil)
	if _, err := parseQueryFilter(r); err == nil {
		t.Error("expected error for invalid from timestamp, got nil")
	}
}

// TestValidEventTypes verifies that the validEventTypes map contains all
// expected event types and does not contain unknown ones.
func TestValidEventTypes(t *testing.T) {
	known := []string{
		"auth.login", "auth.logout", "auth.login_failed", "auth.mfa_enabled",
		"privilege.change", "secret.accessed", "plugin.installed", "plugin.uninstalled",
	}
	for _, et := range known {
		if !validEventTypes[et] {
			t.Errorf("validEventTypes missing %q", et)
		}
	}
	if validEventTypes["unknown.event"] {
		t.Error("validEventTypes should not contain 'unknown.event'")
	}
}

// TestValidSeverities verifies the severity enum map.
func TestValidSeverities(t *testing.T) {
	for _, s := range []string{"info", "warning", "critical"} {
		if !validSeverities[s] {
			t.Errorf("validSeverities missing %q", s)
		}
	}
	if validSeverities["debug"] {
		t.Error("validSeverities should not contain 'debug'")
	}
}

// TestValidActorTypes verifies the actor_type enum map.
func TestValidActorTypes(t *testing.T) {
	for _, at := range []string{"user", "system", "plugin"} {
		if !validActorTypes[at] {
			t.Errorf("validActorTypes missing %q", at)
		}
	}
	if validActorTypes["bot"] {
		t.Error("validActorTypes should not contain 'bot'")
	}
}

// TestRealIP_XRealIP verifies that the X-Real-IP header is used when present.
func TestRealIP_XRealIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Real-IP", "203.0.113.1")
	if got := realIP(r); got != "203.0.113.1" {
		t.Errorf("realIP = %q, want %q", got, "203.0.113.1")
	}
}

// TestRealIP_XForwardedFor verifies that the first element of X-Forwarded-For
// is returned when X-Real-IP is absent.
func TestRealIP_XForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.2, 10.0.0.1")
	if got := realIP(r); got != "203.0.113.2" {
		t.Errorf("realIP = %q, want %q", got, "203.0.113.2")
	}
}
