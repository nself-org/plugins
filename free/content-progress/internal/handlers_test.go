package internal

import (
	"net/http/httptest"
	"net/http"
	"testing"
)

// TestQueryInt_Default verifies that queryInt returns the default when the
// parameter is absent.
func TestQueryInt_Default(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/progress", nil)
	got := queryInt(r, "limit", 50)
	if got != 50 {
		t.Errorf("queryInt (missing) = %d, want 50", got)
	}
}

// TestQueryInt_Valid verifies that a valid integer query parameter is parsed.
func TestQueryInt_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/progress?limit=20", nil)
	got := queryInt(r, "limit", 50)
	if got != 20 {
		t.Errorf("queryInt (valid) = %d, want 20", got)
	}
}

// TestQueryInt_Invalid verifies that a non-numeric value falls back to the
// default.
func TestQueryInt_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/progress?limit=abc", nil)
	got := queryInt(r, "limit", 50)
	if got != 50 {
		t.Errorf("queryInt (invalid) = %d, want 50 (default)", got)
	}
}

// TestQueryInt_Zero verifies that a zero value is accepted (not treated as
// missing).
func TestQueryInt_Zero(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/progress?offset=0", nil)
	got := queryInt(r, "offset", 10)
	if got != 0 {
		t.Errorf("queryInt (zero) = %d, want 0", got)
	}
}
