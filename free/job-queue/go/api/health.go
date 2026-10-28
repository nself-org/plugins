package api

import (
	"encoding/json"
	"net/http"
)

// handleHealth requires Redis liveness and successful boot migrations.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.rdb.Ping(r.Context()).Err(); err != nil {
		http.Error(w, `{"status":"unhealthy","redis":"down"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// bootMigrationsHealth may answer 503 itself, so no explicit WriteHeader(200) before it (the default applies).
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "migrations": bootMigrationsHealth(w)})
}
