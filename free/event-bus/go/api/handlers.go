// Package api provides the HTTP control plane for the nSelf event-bus plugin.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nself-org/nself-event-bus/broker"
	"github.com/nself-org/nself-event-bus/stats"
)

// Handler holds the dependencies for all HTTP handlers.
type Handler struct {
	brk   broker.Broker
	store stats.Store
}

// NewMux wires all event-bus routes onto a new http.ServeMux.
func NewMux(brk broker.Broker, store stats.Store) http.Handler {
	mux := http.NewServeMux()
	h := &Handler{brk: brk, store: store}

	mux.HandleFunc("GET /event-bus/status", h.Status)
	mux.HandleFunc("GET /event-bus/subjects", h.Subjects)
	mux.HandleFunc("POST /event-bus/publish", h.Publish)
	mux.HandleFunc("GET /event-bus/consumers", h.Consumers)
	mux.HandleFunc("POST /event-bus/purge/{subject}", h.Purge)
	mux.HandleFunc("GET /health", h.Health)

	return mux
}

// StatusResponse is the JSON shape returned by GET /event-bus/status.
type StatusResponse struct {
	BrokerType   string `json:"broker_type"`
	SubjectCount int    `json:"subject_count"`
	Running      bool   `json:"running"`
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	subjects := h.store.Subjects()
	writeJSON(w, http.StatusOK, StatusResponse{
		BrokerType:   h.brk.Type(),
		SubjectCount: len(subjects),
		Running:      true,
	})
}

// SubjectEntry is one row in the GET /event-bus/subjects response.
type SubjectEntry struct {
	Subject       string `json:"subject"`
	MsgCount      int64  `json:"msg_count"`
	ConsumerCount int    `json:"consumer_count"`
}

func (h *Handler) Subjects(w http.ResponseWriter, r *http.Request) {
	rows := h.store.Subjects()
	out := make([]SubjectEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, SubjectEntry{
			Subject:       row.Subject,
			MsgCount:      row.MsgCount,
			ConsumerCount: row.ConsumerCount,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// PublishRequest is the body accepted by POST /event-bus/publish (admin only).
type PublishRequest struct {
	Subject string          `json:"subject"`
	Payload json.RawMessage `json:"payload"`
}

func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Subject == "" {
		http.Error(w, "subject is required", http.StatusBadRequest)
		return
	}
	if err := h.brk.Publish(r.Context(), req.Subject, req.Payload); err != nil {
		slog.Error("admin publish failed", "subject", req.Subject, "err", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	h.store.RecordPublish(req.Subject)
	writeJSON(w, http.StatusOK, map[string]string{"status": "published"})
}

// ConsumerEntry is one row in the GET /event-bus/consumers response.
type ConsumerEntry struct {
	Subject  string `json:"subject"`
	Durable  string `json:"durable"`
	Lag      int64  `json:"pending_msgs"`
}

func (h *Handler) Consumers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.Consumers())
}

func (h *Handler) Purge(w http.ResponseWriter, r *http.Request) {
	subject := r.PathValue("subject")
	if subject == "" {
		http.Error(w, "subject is required", http.StatusBadRequest)
		return
	}
	// Decode URL path segment (%2F → /).
	subject = strings.ReplaceAll(subject, "%2F", "/")
	if err := h.store.Purge(context.Background(), subject); err != nil {
		slog.Error("purge failed", "subject", subject, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "purged", "subject": subject})
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "broker": h.brk.Type()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
