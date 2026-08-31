package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	h := &Handlers{cfg: LoadConfig()}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["plugin"] != "home" {
		t.Fatalf("expected plugin=home, got %s", body["plugin"])
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %s", body["status"])
	}
}

func TestSourceAccountHeader(t *testing.T) {
	h := &Handlers{cfg: LoadConfig()}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Default
	if sa := h.sa(req); sa != "primary" {
		t.Fatalf("expected primary, got %s", sa)
	}

	// With header
	req.Header.Set("X-Source-Account-ID", "tenant-1")
	if sa := h.sa(req); sa != "tenant-1" {
		t.Fatalf("expected tenant-1, got %s", sa)
	}
}

func TestCommandValidation(t *testing.T) {
	h := &Handlers{cfg: LoadConfig()}
	req := httptest.NewRequest(http.MethodPost, "/home/command", nil)
	w := httptest.NewRecorder()
	h.HandleCommand(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400 for nil body, got %d", w.Code)
	}
}
