// Command alpha is a fixture plugin: a Go service with a Postgres pool and a
// /health handler, used by the conformance, codemod and boot tests.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"example.com/localsdk"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("alpha: pool: %v", err)
	}
	mustBootMigrations(ctx, pool)
	defer pool.Close()
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "plugin": localsdk.Name("alpha"), "migrations": bootMigrationsHealth()})
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "3901"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
