// Package main — family-ancestry plugin HTTP server.
//
// Exposes /health and /ready probes. Ancestry.com import API routes will be
// registered here as the plugin moves from "planned" to "stable".
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PLUGIN_FAMILY_ANCESTRY_PORT")
	if port == "" {
		port = "3513"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ready", handleHealth)

	addr := ":" + port
	log.Printf("nself-family-ancestry listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"plugin": "nself-family-ancestry",
	})
}
