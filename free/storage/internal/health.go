package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// handleHealth answers GET /health: database reachability plus the boot-migrations counts.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	s3 := "not_configured"
	if s.store != nil {
		s3 = "configured" // no network probe: /health must stay cheap and never depend on the S3 endpoint
	}
	// bootMigrationsHealth may answer 503 itself, so no explicit WriteHeader(200) before it (the default applies).
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "migrations": bootMigrationsHealth(w), "s3": s3})
}
