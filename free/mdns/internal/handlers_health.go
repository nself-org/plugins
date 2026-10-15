package internal

import (
	"encoding/json"
	"fmt"
	sdk "github.com/nself-org/plugin-sdk"
	"net/http"
	"runtime"
	"time"
)

// handleHealth answers GET /health: database reachability plus the boot-migrations counts.
func handleHealth(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// bootMigrationsHealth may answer 503 itself, so no explicit WriteHeader(200) before it (the default applies).
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "migrations": bootMigrationsHealth(r.Context(), w)})
	}
}

func handleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			sdk.Error(w, http.StatusServiceUnavailable, fmt.Errorf("database not ready: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func handleLive(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		stats, _ := db.GetStats()

		resp := map[string]interface{}{
			"status": "live",
			"uptime": time.Since(startTime).String(),
			"memory": map[string]interface{}{
				"alloc_mb":       fmt.Sprintf("%.2f", float64(mem.Alloc)/1024/1024),
				"total_alloc_mb": fmt.Sprintf("%.2f", float64(mem.TotalAlloc)/1024/1024),
				"sys_mb":         fmt.Sprintf("%.2f", float64(mem.Sys)/1024/1024),
				"num_gc":         mem.NumGC,
			},
			"goroutines": runtime.NumGoroutine(),
		}

		if stats != nil {
			resp["stats"] = stats
		}

		sdk.Respond(w, http.StatusOK, resp)
	}
}

// --- Service CRUD handlers ---
