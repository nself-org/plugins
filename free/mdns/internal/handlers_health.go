package internal

import (
	"fmt"
	"net/http"
	"runtime"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

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
