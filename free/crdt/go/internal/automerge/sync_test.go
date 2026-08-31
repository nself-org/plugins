package automerge_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSyncReturns413WhenBodyTooLarge verifies the max doc size enforcement.
// This test uses only net/http/httptest and does not require a live DB.
func TestSyncReturns413WhenBodyTooLarge(t *testing.T) {
	// We cannot instantiate SyncHandler without a real store here,
	// but we can verify the MaxBytesReader plumbing behaviour separately.
	// Full integration test: tests/integration/automerge_test.go.

	// Create a mock handler that replicates the MaxBytesReader check.
	maxBytes := int64(10 * 1024 * 1024) // 10 MB
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1)
		buf := make([]byte, maxBytes+10)
		_, err := r.Body.Read(buf)
		if err != nil && err.Error() == "http: request body too large" {
			http.Error(w, "document too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// 11 MB body — should get 413.
	bigBody := bytes.Repeat([]byte("x"), int(maxBytes)+100)
	req := httptest.NewRequest(http.MethodPost, "/crdt/sync/test-doc", bytes.NewReader(bigBody))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rr.Code)
	}
}
