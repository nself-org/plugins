package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestParsePagination_Defaults verifies that missing params return defaults (limit=100, offset=0).
func TestParsePagination_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/repos", nil)
	limit, offset := parsePagination(r)
	if limit != 100 {
		t.Errorf("default limit = %d, want 100", limit)
	}
	if offset != 0 {
		t.Errorf("default offset = %d, want 0", offset)
	}
}

// TestParsePagination_Valid verifies that valid params are parsed correctly.
func TestParsePagination_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/repos?limit=25&offset=50", nil)
	limit, offset := parsePagination(r)
	if limit != 25 {
		t.Errorf("limit = %d, want 25", limit)
	}
	if offset != 50 {
		t.Errorf("offset = %d, want 50", offset)
	}
}

// TestParsePagination_LimitCap verifies that limit > 1000 falls back to 100 (not capped to 1000).
func TestParsePagination_LimitExceedsMax(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/repos?limit=9999", nil)
	limit, _ := parsePagination(r)
	if limit != 100 {
		t.Errorf("limit for 9999 = %d, want 100 (default)", limit)
	}
}

// TestParsePagination_InvalidLimit verifies that a non-numeric limit falls back to default.
func TestParsePagination_InvalidLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/repos?limit=abc", nil)
	limit, _ := parsePagination(r)
	if limit != 100 {
		t.Errorf("invalid limit = %d, want 100", limit)
	}
}

// TestParseOptionalInt64_Missing verifies that a missing param returns nil.
func TestParseOptionalInt64_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/repos", nil)
	got := parseOptionalInt64(r, "repo_id")
	if got != nil {
		t.Errorf("parseOptionalInt64(missing) = %v, want nil", got)
	}
}

// TestParseOptionalInt64_Valid verifies that a valid int64 param is parsed.
func TestParseOptionalInt64_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/repos?repo_id=12345", nil)
	got := parseOptionalInt64(r, "repo_id")
	if got == nil {
		t.Fatal("parseOptionalInt64 = nil, want non-nil")
	}
	if *got != 12345 {
		t.Errorf("*parseOptionalInt64 = %d, want 12345", *got)
	}
}

// TestParseOptionalInt64_Invalid verifies that a non-integer param returns nil.
func TestParseOptionalInt64_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/repos?repo_id=notanumber", nil)
	got := parseOptionalInt64(r, "repo_id")
	if got != nil {
		t.Errorf("parseOptionalInt64(invalid) = %v, want nil", got)
	}
}

// TestToRawMessage_Valid verifies that a struct is correctly marshalled to *json.RawMessage.
func TestToRawMessage_Valid(t *testing.T) {
	input := map[string]string{"key": "value"}
	got := toRawMessage(input)
	if got == nil {
		t.Fatal("toRawMessage = nil, want non-nil")
	}
	var out map[string]string
	if err := json.Unmarshal(*got, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if out["key"] != "value" {
		t.Errorf("out[key] = %q, want %q", out["key"], "value")
	}
}

// TestDefaultJSONB_Nil verifies that nil returns "{}".
func TestDefaultJSONB_Nil(t *testing.T) {
	got := defaultJSONB(nil)
	if string(got) != "{}" {
		t.Errorf("defaultJSONB(nil) = %s, want {}", string(got))
	}
}

// TestDefaultJSONB_NonNil verifies that a non-nil value is returned unchanged.
func TestDefaultJSONB_NonNil(t *testing.T) {
	raw := json.RawMessage(`{"a":1}`)
	got := defaultJSONB(&raw)
	if string(got) != `{"a":1}` {
		t.Errorf("defaultJSONB = %s, want %s", string(got), `{"a":1}`)
	}
}
