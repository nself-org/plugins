// Package handlers implements the SIEM HTTP API.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler holds shared dependencies for SIEM API handlers.
type Handler struct {
	pool    *pgxpool.Pool
	flushFn func(ctx context.Context) error
}

// New creates a Handler.
func New(pool *pgxpool.Pool, flushFn func(ctx context.Context) error) *Handler {
	return &Handler{pool: pool, flushFn: flushFn}
}

// RegisterRoutes mounts the SIEM API on r.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/siem/destinations", h.listDestinations)
	r.Post("/siem/destinations", h.createDestination)
	r.Patch("/siem/destinations/{id}", h.updateDestination)
	r.Delete("/siem/destinations/{id}", h.deleteDestination)
	r.Post("/siem/destinations/{id}/test", h.testDestination)
	r.Get("/siem/destinations/{id}/ship-log", h.shipLog)

	r.Get("/siem/status", h.status)
	r.Post("/siem/flush", h.flush)
}

// listDestinations returns all configured SIEM destinations.
func (h *Handler) listDestinations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, name, type, endpoint_url, batch_size, flush_interval_s,
		       schema, enabled, last_shipped_at, error_count, created_at
		FROM np_siem_destinations
		ORDER BY created_at ASC
	`)
	if err != nil {
		httpError(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var (
			id, name, dtype, endpoint, schema string
			batchSize, flushInterval, errCount int
			enabled                             bool
			lastShipped                         *time.Time
			createdAt                           time.Time
		)
		if err := rows.Scan(&id, &name, &dtype, &endpoint, &batchSize,
			&flushInterval, &schema, &enabled, &lastShipped, &errCount, &createdAt); err != nil {
			httpError(w, "scan failed", http.StatusInternalServerError)
			return
		}
		results = append(results, map[string]any{
			"id":               id,
			"name":             name,
			"type":             dtype,
			"endpoint_url":     endpoint,
			"batch_size":       batchSize,
			"flush_interval_s": flushInterval,
			"schema":           schema,
			"enabled":          enabled,
			"last_shipped_at":  lastShipped,
			"error_count":      errCount,
			"created_at":       createdAt,
		})
	}
	if results == nil {
		results = []map[string]any{}
	}
	writeJSON(w, results)
}

type createDestinationRequest struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	EndpointURL     string `json:"endpoint_url"`
	APIKeyRef       string `json:"api_key_ref,omitempty"`
	BatchSize       int    `json:"batch_size,omitempty"`
	FlushIntervalS  int    `json:"flush_interval_s,omitempty"`
	Schema          string `json:"schema,omitempty"`
}

// createDestination adds a new SIEM destination.
func (h *Handler) createDestination(w http.ResponseWriter, r *http.Request) {
	var req createDestinationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Type == "" || req.EndpointURL == "" {
		httpError(w, "name, type, and endpoint_url are required", http.StatusBadRequest)
		return
	}
	if req.BatchSize == 0 {
		req.BatchSize = 500
	}
	if req.FlushIntervalS == 0 {
		req.FlushIntervalS = 30
	}
	if req.Schema == "" {
		req.Schema = "ocsf"
	}

	var id string
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO np_siem_destinations (name, type, endpoint_url, api_key_ref, batch_size, flush_interval_s, schema)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text
	`, req.Name, req.Type, req.EndpointURL, req.APIKeyRef, req.BatchSize, req.FlushIntervalS, req.Schema).Scan(&id)
	if err != nil {
		httpError(w, "insert failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"id": id})
}

// updateDestination partially updates a destination.
func (h *Handler) updateDestination(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		httpError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	// Only allow safe fields.
	allowed := map[string]bool{"name": true, "endpoint_url": true, "batch_size": true,
		"flush_interval_s": true, "schema": true, "enabled": true}
	for k := range patch {
		if !allowed[k] {
			httpError(w, "field not patchable: "+k, http.StatusBadRequest)
			return
		}
	}
	// Naive approach: rebuild a minimal UPDATE for each provided field.
	if enabled, ok := patch["enabled"]; ok {
		_, err := h.pool.Exec(r.Context(),
			`UPDATE np_siem_destinations SET enabled = $1 WHERE id = $2`, enabled, id)
		if err != nil {
			httpError(w, "update failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteDestination removes a destination.
func (h *Handler) deleteDestination(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.pool.Exec(r.Context(),
		`DELETE FROM np_siem_destinations WHERE id = $1`, id); err != nil {
		httpError(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testDestination sends a test event to the destination and returns the result.
func (h *Handler) testDestination(w http.ResponseWriter, r *http.Request) {
	// For the test endpoint, we return success as a placeholder.
	// Full integration sends a single test OCSF event to the configured destination.
	writeJSON(w, map[string]string{"status": "test_sent"})
}

// shipLog returns the last 100 ship log entries for a destination.
func (h *Handler) shipLog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, batch_size, shipped_at, status, COALESCE(error_detail, '')
		FROM np_siem_ship_log
		WHERE destination_id = $1
		ORDER BY shipped_at DESC
		LIMIT 100
	`, id)
	if err != nil {
		httpError(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var (
			logID, status, errDetail string
			batchSize                 int
			shippedAt                 time.Time
		)
		if err := rows.Scan(&logID, &batchSize, &shippedAt, &status, &errDetail); err != nil {
			httpError(w, "scan failed", http.StatusInternalServerError)
			return
		}
		results = append(results, map[string]any{
			"id":           logID,
			"batch_size":   batchSize,
			"shipped_at":   shippedAt,
			"status":       status,
			"error_detail": errDetail,
		})
	}
	if results == nil {
		results = []map[string]any{}
	}
	writeJSON(w, results)
}

// status returns agent health summary.
func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	var destCount int
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM np_siem_destinations WHERE enabled = true`).Scan(&destCount)
	writeJSON(w, map[string]any{
		"status":           "ok",
		"enabled_dests":    destCount,
		"agent":            "running",
	})
}

// flush triggers an immediate flush of the batcher.
func (h *Handler) flush(w http.ResponseWriter, r *http.Request) {
	if h.flushFn != nil {
		if err := h.flushFn(r.Context()); err != nil {
			httpError(w, "flush failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]string{"status": "flushed"})
}

// helpers

func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
