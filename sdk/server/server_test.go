package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthIsAliasOfHealthz verifies GET /health returns the same status
// and body shape as GET /healthz. The nSelf CLI's docker-compose healthcheck
// convention probes /health (internal/compose/custom_services.go); every
// plugin's docker-compose.plugin.yml relies on this alias existing so the
// container is reported healthy instead of 404-ing the probe.
func TestHealthIsAliasOfHealthz(t *testing.T) {
	mux := New(Options{Plugin: "test-plugin", Version: "1.2.3"})

	for _, path := range []string{"/healthz", "/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: code=%d want=%d body=%s", path, rec.Code, http.StatusOK, rec.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode body: %v", path, err)
		}
		if body["status"] != "ok" {
			t.Errorf("%s: status=%v want=ok", path, body["status"])
		}
		if body["plugin"] != "test-plugin" {
			t.Errorf("%s: plugin=%v want=test-plugin", path, body["plugin"])
		}
		if body["version"] != "1.2.3" {
			t.Errorf("%s: version=%v want=1.2.3", path, body["version"])
		}
	}
}

// TestHealthAndHealthzBodiesMatch verifies the two endpoints are true
// aliases: identical response bytes, not just identical shape.
func TestHealthAndHealthzBodiesMatch(t *testing.T) {
	mux := New(Options{Plugin: "test-plugin", Version: "1.2.3"})

	reqZ := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recZ := httptest.NewRecorder()
	mux.ServeHTTP(recZ, reqZ)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if recZ.Body.String() != rec.Body.String() {
		t.Fatalf("body mismatch:\n/healthz: %s\n/health:  %s", recZ.Body.String(), rec.Body.String())
	}
}
