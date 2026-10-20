package main

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationFailureHeaders(t *testing.T) {
	for _, contentType := range []string{"", "text/plain"} {
		t.Run(contentType, func(t *testing.T) {
			pool, err := pgxpool.New(context.Background(), "postgres://localhost/test")
			if err != nil {
				t.Fatal(err)
			}
			pool.Close() // Force Status to fail without requiring a database.
			old := bootPool
			bootPool = pool
			defer func() { bootPool = old }()
			w := httptest.NewRecorder()
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			counts := bootMigrationsHealth(w)
			response := w.Result()
			defer response.Body.Close()
			if response.StatusCode != 503 || response.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("status=%d headers=%v", response.StatusCode, response.Header)
			}
			if counts["applied"] != -1 || counts["expected"] != -1 {
				t.Fatalf("counts=%v", counts)
			}
		})
	}
}
