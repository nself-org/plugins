package api

import (
	"encoding/json"
	"net/http"
	"os"
)

// handleHealth answers GET /health: Redis liveness plus the boot-migrations counts (contract
// plugin.boot-migrations v1). Redis down answers 503 as before. JOBQUEUE_HEALTH_IGNORE_REDIS=true skips the Redis
// probe (the boot test kit has no Redis); it is off by default, so a normal install behaves exactly as before.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("JOBQUEUE_HEALTH_IGNORE_REDIS") != "true" {
		if err := s.rdb.Ping(r.Context()).Err(); err != nil {
			http.Error(w, `{"status":"unhealthy","redis":"down"}`, http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	// bootMigrationsHealth may answer 503 itself, so no explicit WriteHeader(200) before it (the default applies).
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "migrations": bootMigrationsHealth(r.Context(), w)})
}
