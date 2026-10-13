package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// With no boot pool the /health handler must answer 503 and still carry the migrations member (never a false 200).
func TestBootHealthHandler_NoPoolIs503(t *testing.T) {
	old := bootPool
	defer func() { bootPool = old }()
	bootPool = nil
	rec := httptest.NewRecorder()
	BootHealthHandler()(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["plugin"] != "vpn" {
		t.Errorf("plugin = %v", out["plugin"])
	}
	if _, ok := out["migrations"].(map[string]any); !ok {
		t.Errorf("migrations member missing: %v", out)
	}
}
