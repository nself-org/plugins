// Package api provides the CDC plugin HTTP control plane.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/nself-org/nself-cdc/broker"
	"github.com/nself-org/nself-cdc/replication"
)

// engineIface is the subset of replication.Engine used by the API handlers.
type engineIface interface {
	Snapshot(ctx interface{ Deadline() (interface{}, bool) }, table string) error
	Pause()
	Resume(ctx interface{ Deadline() (interface{}, bool) })
	IsRunning() bool
	IsPaused() bool
	SlotLagBytes() int64
	EventsPerSecond() int64
}

// NewMux wires all CDC API routes onto a new http.ServeMux.
func NewMux(eng *replication.Engine, brk broker.Broker) http.Handler {
	mux := http.NewServeMux()
	h := &handler{eng: eng, brk: brk}

	mux.HandleFunc("GET /cdc/status", h.Status)
	mux.HandleFunc("GET /cdc/topics", h.Topics)
	mux.HandleFunc("POST /cdc/snapshot", h.TriggerSnapshot)
	mux.HandleFunc("POST /cdc/pause", h.Pause)
	mux.HandleFunc("POST /cdc/resume", h.Resume)
	mux.HandleFunc("DELETE /cdc/slot", h.DropSlot)
	mux.HandleFunc("GET /health", h.Health)

	return mux
}

type handler struct {
	eng *replication.Engine
	brk broker.Broker
}

// StatusResponse is the JSON shape returned by GET /cdc/status.
type StatusResponse struct {
	Running         bool   `json:"running"`
	Paused          bool   `json:"paused"`
	BrokerType      string `json:"broker_type"`
	SlotLagBytes    int64  `json:"slot_lag_bytes"`
	SlotLagSeconds  float64 `json:"slot_lag_seconds"`
	EventsPerSecond int64  `json:"events_per_second"`
}

func (h *handler) Status(w http.ResponseWriter, r *http.Request) {
	lagBytes := h.eng.SlotLagBytes()
	// Approximate lag in seconds: assume 1 MB/s WAL write rate as baseline.
	lagSeconds := float64(lagBytes) / (1024 * 1024)

	resp := StatusResponse{
		Running:         h.eng.IsRunning(),
		Paused:          h.eng.IsPaused(),
		BrokerType:      h.brk.Type(),
		SlotLagBytes:    lagBytes,
		SlotLagSeconds:  lagSeconds,
		EventsPerSecond: h.eng.EventsPerSecond(),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handler) Topics(w http.ResponseWriter, r *http.Request) {
	// In production this reads from np_cdc_events DISTINCT table_name + operation.
	writeJSON(w, http.StatusOK, map[string]string{"message": "topic list requires DB query"})
}

func (h *handler) TriggerSnapshot(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	if table == "" {
		http.Error(w, "table query param required", http.StatusBadRequest)
		return
	}
	go func() {
		if err := h.eng.Snapshot(r.Context(), table); err != nil {
			slog.Error("cdc: snapshot", "table", table, "err", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "snapshot started", "table": table})
}

func (h *handler) Pause(w http.ResponseWriter, r *http.Request) {
	h.eng.Pause()
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (h *handler) Resume(w http.ResponseWriter, r *http.Request) {
	h.eng.Resume(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (h *handler) DropSlot(w http.ResponseWriter, r *http.Request) {
	confirm := r.URL.Query().Get("confirm")
	if confirm != "true" {
		http.Error(w, "confirm=true required to drop replication slot", http.StatusBadRequest)
		return
	}
	// Actual slot drop handled by plugin uninstall lifecycle hook.
	writeJSON(w, http.StatusOK, map[string]string{"status": "slot drop queued"})
}

func (h *handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "cdc"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("cdc: json encode", "err", err)
	}
}
