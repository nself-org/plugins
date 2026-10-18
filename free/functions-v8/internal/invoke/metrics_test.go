package invoke_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/nself-functions-v8/internal/invoke"
)

func TestRecordInvocation_Smoke(t *testing.T) {
	// RecordInvocation must not panic for success or error paths.
	invoke.RecordInvocation("hello", 200, 42*time.Millisecond, false)
	invoke.RecordInvocation("hello", 504, 30*time.Second, true)
	invoke.RecordInvocation("hello", 500, 100*time.Millisecond, false)
}

func TestRecordPoolState_Smoke(t *testing.T) {
	invoke.RecordPoolState(3, 5)
	invoke.RecordPoolState(0, 5)
}

func TestMetricsHandler_Exposes(t *testing.T) {
	// Emit some metrics then verify /metrics text contains expected metric names.
	invoke.RecordInvocation("my-func", 200, 10*time.Millisecond, false)

	h := invoke.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "nself_edge_functions_invocations_total") {
		t.Errorf("/metrics missing nself_edge_functions_invocations_total; got:\n%s", body[:min(500, len(body))])
	}
	if !strings.Contains(body, "nself_edge_functions_invocation_duration_seconds") {
		t.Errorf("/metrics missing nself_edge_functions_invocation_duration_seconds")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
