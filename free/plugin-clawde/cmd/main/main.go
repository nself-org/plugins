// Purpose: plugin-clawde entry point. Starts HTTP server on port 3847.
//   Provides /health, /sessions, /sessions/{id}/heartbeat,
//   /sessions/{id}/events endpoints.
// Inputs: NSELF_DB_URL env var.
// Outputs: HTTP JSON responses and SSE event streams.
// Constraints:
//   - plugin-clawde is a free plugin (plugin.json: requires_license=false) —
//     the former NSELF_LICENSE_KEY-required gate was removed 2026-09-03
//     (P6-E3-W2-S1-T5 FIX-PLUGINS).
//   - All session operations require source_account_id (Multi-App Isolation).
package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/nself-org/plugin-clawde/internal"
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

	daemonAddr := os.Getenv("NSELF_CLAWDE_DAEMON_ADDR")
	if daemonAddr == "" {
		daemonAddr = "localhost:3848"
	}

	h := &internal.Handlers{DB: db, DaemonAddr: daemonAddr}
	port := os.Getenv("NSELF_CLAWDE_PORT")
	if port == "" {
		port = "3847"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/sessions", h.CreateSession)
	mux.HandleFunc("/sessions/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/heartbeat") && r.Method == http.MethodPost {
			h.Heartbeat(w, r)
		} else if strings.HasSuffix(path, "/events") && r.Method == http.MethodPost {
			h.AppendEvent(w, r)
		} else if r.Method == http.MethodDelete {
			h.CloseSession(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	addr := fmt.Sprintf(":%s", port)
	log.Printf("plugin-clawde listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
