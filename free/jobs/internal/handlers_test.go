package internal

import (
	"net/http/httptest"
	"net/http"
	"testing"
)

// TestQueryInt_Default verifies that a missing key returns the fallback.
func TestQueryInt_Default(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	got := queryInt(r, "limit", 25)
	if got != 25 {
		t.Errorf("queryInt(missing) = %d, want 25", got)
	}
}

// TestQueryInt_Valid verifies that a valid integer query param is parsed.
func TestQueryInt_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/jobs?limit=50", nil)
	got := queryInt(r, "limit", 25)
	if got != 50 {
		t.Errorf("queryInt(limit=50) = %d, want 50", got)
	}
}

// TestQueryInt_Invalid verifies that a non-integer query param falls back to default.
func TestQueryInt_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/jobs?limit=abc", nil)
	got := queryInt(r, "limit", 25)
	if got != 25 {
		t.Errorf("queryInt(limit=abc) = %d, want 25", got)
	}
}

// TestQueryInt_Zero verifies that zero is a valid parsed value.
func TestQueryInt_Zero(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/jobs?offset=0", nil)
	got := queryInt(r, "offset", 10)
	if got != 0 {
		t.Errorf("queryInt(offset=0) = %d, want 0", got)
	}
}
