package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Handlers holds the database pool and handles all HTTP requests.
type Handlers struct {
	pool PgxIface
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(pool PgxIface) *Handlers {
	return &Handlers{pool: pool}
}

// sa returns the source account ID from the request header, defaulting to "primary".
func (h *Handlers) sa(r *http.Request) string {
	if sa := r.Header.Get("X-Source-Account-ID"); sa != "" {
		return sa
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ============================================================
// Health / Status
// ============================================================

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "plugin": "entitlements", "migrations": bootMigrationsHealth(r.Context(), w)})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"ready": "false", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ready": "true", "plugin": "entitlements"})
}

func (h *Handlers) PluginStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plugin": "entitlements", "version": "1.0.0", "status": "running"})
}

// ============================================================
// Plans
// ============================================================
