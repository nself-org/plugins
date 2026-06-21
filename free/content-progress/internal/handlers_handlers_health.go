package internal

import (
	"fmt"
	"net/http"
	"runtime"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// Health / Readiness / Liveness
// =========================================================================

func handleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			sdk.Error(w, http.StatusServiceUnavailable, fmt.Errorf("database unavailable"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"ready":     true,
			"plugin":    "content-progress",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleLive(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetPluginStats()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get stats: %w", err))
			return
		}

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"alive":   true,
			"plugin":  "content-progress",
			"version": "1.0.0",
			"uptime":  time.Since(startTime).Seconds(),
			"memory": map[string]interface{}{
				"alloc_mb":       float64(memStats.Alloc) / 1024 / 1024,
				"sys_mb":         float64(memStats.Sys) / 1024 / 1024,
				"num_gc":         memStats.NumGC,
				"goroutines":     runtime.NumGoroutine(),
			},
			"stats": map[string]interface{}{
				"totalUsers":      stats.TotalUsers,
				"totalPositions":  stats.TotalPositions,
				"totalCompleted":  stats.TotalCompleted,
				"lastActivity":    stats.LastActivity,
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleStatus(db *DB, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetPluginStats()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get stats: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"plugin":  "content-progress",
			"version": "1.0.0",
			"status":  "running",
			"config": map[string]interface{}{
				"completeThreshold":    cfg.CompleteThreshold,
				"historySampleSeconds": cfg.HistorySampleSeconds,
			},
			"stats":     stats,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

