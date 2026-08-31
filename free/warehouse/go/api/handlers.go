// Package api provides the HTTP control plane for the warehouse plugin.
// All endpoints require x-hasura-role: admin (enforced by Nginx upstream).
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/nself-org/nself-warehouse/pipeline"
)

// pipelineIface is the subset of pipeline.Pipeline used by the handlers.
type pipelineIface interface {
	Store() *pipeline.Store
	Backfiller() *pipeline.Backfiller
	Pause()
	Resume()
	IsPaused() bool
}

// NewMux returns an http.ServeMux with all warehouse control endpoints.
func NewMux(p pipelineIface) *http.ServeMux {
	mux := http.NewServeMux()
	h := &handlers{p: p}
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/ready", h.health)
	mux.HandleFunc("/warehouse/status", h.status)
	mux.HandleFunc("/warehouse/backfill", h.backfill)
	mux.HandleFunc("/warehouse/schema", h.schema)
	mux.HandleFunc("/warehouse/pause", h.pause)
	mux.HandleFunc("/warehouse/resume", h.resume)
	return mux
}

type handlers struct {
	p pipelineIface
}

// GET /warehouse/status — sync lag, row counts, last export time per table.
func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	watermarks, err := h.p.Store().AllWatermarks(r.Context())
	if err != nil {
		jsonError(w, "query watermarks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	errors, err := h.p.Store().RecentErrors(r.Context())
	if err != nil {
		jsonError(w, "query errors: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Compute lag in seconds from last export
	type tableStatus struct {
		TableName  string    `json:"table_name"`
		LastLSN    string    `json:"last_lsn"`
		ExportedAt time.Time `json:"exported_at"`
		LagSeconds float64   `json:"lag_seconds"`
		RowCount   int64     `json:"row_count"`
	}
	statuses := make([]tableStatus, 0, len(watermarks))
	for _, wm := range watermarks {
		statuses = append(statuses, tableStatus{
			TableName:  wm.TableName,
			LastLSN:    wm.LastLSN,
			ExportedAt: wm.ExportedAt,
			LagSeconds: time.Since(wm.ExportedAt).Seconds(),
			RowCount:   wm.RowCount,
		})
	}

	jsonOK(w, map[string]interface{}{
		"paused":  h.p.IsPaused(),
		"tables":  statuses,
		"errors":  errors,
	})
}

// POST /warehouse/backfill — body: {"table":"np_foo","since":"2024-01-01T00:00:00Z"}
func (h *handlers) backfill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Table string  `json:"table"`
		Since *string `json:"since"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Table == "" {
		jsonError(w, "table is required", http.StatusBadRequest)
		return
	}

	bfReq := pipeline.BackfillRequest{Table: req.Table}
	if req.Since != nil && *req.Since != "" {
		t, err := time.Parse(time.RFC3339, *req.Since)
		if err != nil {
			jsonError(w, "since must be RFC3339: "+err.Error(), http.StatusBadRequest)
			return
		}
		bfReq.Since = &t
	}

	// Run backfill asynchronously so the HTTP call returns immediately.
	go func() {
		_ = h.p.Backfiller().Run(r.Context(), bfReq)
	}()

	jsonOK(w, map[string]string{"status": "backfill_started", "table": req.Table})
}

// GET /warehouse/schema — current mirror schema per target (placeholder — full
// schema introspection requires driver-side query, returned as column map).
func (h *handlers) schema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jsonOK(w, map[string]string{"note": "schema introspection available via driver EnsureTable"})
}

// POST /warehouse/pause — stop consuming new CDC events.
func (h *handlers) pause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.p.Pause()
	jsonOK(w, map[string]string{"status": "paused"})
}

// POST /warehouse/resume — resume CDC event consumption.
func (h *handlers) resume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.p.Resume()
	jsonOK(w, map[string]string{"status": "resumed"})
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// GET /health — liveness probe for warehouse plugin.
func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jsonOK(w, map[string]interface{}{
		"plugin": "nself-warehouse",
		"status": "healthy",
	})
}
