package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestHealthHandler_LedgerUnreadable: with no reachable database the handler answers 503 (the cli retries any
// non-200) and reports -1 counts; GET and HEAD agree on the status.
func TestHealthHandler_LedgerUnreadable(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/x?connect_timeout=1")
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	prev := bootPool
	bootPool = pool
	t.Cleanup(func() { bootPool = prev })

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec := httptest.NewRecorder()
		HealthHandler(rec, httptest.NewRequest(method, "/health", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s /health: status %d, want 503", method, rec.Code)
		}
		if method != http.MethodGet {
			continue
		}
		var body struct {
			Plugin     string         `json:"plugin"`
			Migrations map[string]int `json:"migrations"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body: %v", err)
		}
		if body.Plugin != "backup" || body.Migrations["applied"] != -1 || body.Migrations["expected"] != -1 {
			t.Errorf("body = %+v, want plugin backup and -1 counts", body)
		}
	}
}
