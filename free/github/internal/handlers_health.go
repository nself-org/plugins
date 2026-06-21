package internal

import (
	"log"
	"net/http"
	"runtime"
	"time"
)

// --- Health endpoints --------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"plugin":    "github",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.db.Ping(ctx); err != nil {
		log.Printf("[github:server] Readiness check failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ready":     false,
			"plugin":    "github",
			"error":     "Database unavailable",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ready":     true,
		"plugin":    "github",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := s.db.GetStats(ctx)
	if err != nil {
		log.Printf("[github:server] Live check stats error: %v", err)
		stats = &SyncStats{}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"alive":   true,
		"plugin":  "github",
		"version": "1.0.0",
		"uptime":  time.Since(s.startTime).Seconds(),
		"memory": map[string]interface{}{
			"alloc":      mem.Alloc,
			"totalAlloc": mem.TotalAlloc,
			"sys":        mem.Sys,
			"heapInuse":  mem.HeapInuse,
		},
		"stats": map[string]interface{}{
			"repositories": stats.Repositories,
			"issues":       stats.Issues,
			"pullRequests": stats.PullRequests,
			"lastSync":     stats.LastSyncedAt,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := s.db.GetStats(ctx)
	if err != nil {
		log.Printf("[github:server] Status stats error: %v", err)
		stats = &SyncStats{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"plugin":    "github",
		"version":   "1.0.0",
		"status":    "running",
		"stats":     stats,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

