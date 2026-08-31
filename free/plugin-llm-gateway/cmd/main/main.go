// Command plugin-llm-gateway starts the ClawDE LLM gateway HTTP server.
//
// Purpose: Proxy LLM requests to nself-ai-gateway with quota, caching, and context injection.
// Constraints:
//   - Requires DATABASE_URL and NSELF_AI_GATEWAY_URL (defaults to http://127.0.0.1:3761).
//   - License validated via ping_api (NSELF_PLUGIN_LICENSE_KEY).
//   - Port defaults to 8090 (overridable via PORT env).
package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"github.com/nself-org/plugin-llm-gateway/internal"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	var db *sql.DB
	if dbURL != "" {
		var err error
		db, err = sql.Open("postgres", dbURL)
		if err != nil {
			log.Fatalf("sql.Open: %v", err)
		}
		defer db.Close()
		if err := db.Ping(); err != nil {
			log.Printf("WARN: db ping failed: %v — running without DB (cache/quota disabled)", err)
			db = nil
		}
	}

	gw := internal.NewGateway(db)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", gw.Health)
	r.Post("/v1/completions", gw.Completions)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	addr := fmt.Sprintf(":%s", port)
	log.Printf("plugin-llm-gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
