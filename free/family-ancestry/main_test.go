// Purpose: Baseline tests for the family-ancestry plugin HTTP server.
// Inputs: HTTP GET /health request via httptest.
// Outputs: 200 OK with JSON body containing status and plugin fields.
// Constraints: No external dependencies; uses net/http/httptest only.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBuild verifies the family-ancestry package compiles.
func TestBuild(t *testing.T) {
	t.Log("family-ancestry package compiled OK")
}

// TestHandleHealth verifies the /health endpoint returns 200 with correct JSON.
func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /health = %d, want %d", rr.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("body[\"status\"] = %q, want %q", body["status"], "ok")
	}
	if body["plugin"] != "nself-family-ancestry" {
		t.Errorf("body[\"plugin\"] = %q, want %q", body["plugin"], "nself-family-ancestry")
	}
}

// TestHandleHealthMethodNotAllowed verifies non-GET returns 405.
func TestHandleHealthMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rr := httptest.NewRecorder()
	handleHealth(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /health = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}
