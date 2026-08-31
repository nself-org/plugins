// Package handlers — health.go
//
// Health and readiness probes for the auth-enterprise plugin.
package handlers

import (
	"encoding/json"
	"net/http"
)

// HandleHealth responds to GET /health and GET /ready with a simple liveness response.
// Used by the nself healthcheck system and container orchestrators.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"plugin": "nself-auth-enterprise",
		"status": "healthy",
	})
}
