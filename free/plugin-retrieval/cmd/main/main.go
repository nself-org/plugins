// Purpose: plugin-retrieval entry point. Starts HTTP server on port 3825.
//   Provides /health, /search, /index endpoints for hybrid pgvector + tsvector retrieval.
// Inputs: NSELF_DB_URL env var.
// Outputs: HTTP JSON responses.
// Constraints:
//   - NSELF_LICENSE_KEY must be set (license gate).
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
	licenseKey := os.Getenv("NSELF_LICENSE_KEY")
	if licenseKey == "" {
		log.Fatal("NSELF_LICENSE_KEY is required (pro license)")
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
