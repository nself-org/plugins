// Purpose: plugin-retrieval entry point. Starts HTTP server on port 3825.
//   Provides /health, /search, /index endpoints for hybrid pgvector + tsvector retrieval.
// Inputs: NSELF_DB_URL env var.
// Outputs: HTTP JSON responses.
// Constraints:
//   - plugin-retrieval is a free plugin (plugin.json: requires_license=false)
//     — the former NSELF_LICENSE_KEY-required gate was removed 2026-09-03
//     (P6-E3-W2-S1-T5 FIX-PLUGINS).
//   - SSRF N/A: all DB queries are local Postgres only.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"

	"github.com/nself-org/plugin-retrieval/internal"
)

func main() {
	dbURL := os.Getenv("NSELF_DB_URL")
	if dbURL == "" {
		log.Fatal("NSELF_DB_URL is required")
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	h := &internal.Handlers{DB: db}
	port := os.Getenv("NSELF_RETRIEVAL_PORT")
	if port == "" {
		port = "3825"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/search", h.Search)
	mux.HandleFunc("/index", h.Index)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("plugin-retrieval listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
