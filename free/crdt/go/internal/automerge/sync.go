// Package automerge provides the HTTP sync handler for automerge documents.
// The endpoint accepts raw automerge binary sync messages and returns sync replies.
package automerge

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/nself-org/nself-crdt/internal/store"
)

const maxBodyBytes = 10 * 1024 * 1024 // 10 MB hard limit

// SyncHandler handles POST /crdt/sync/:doc_id for automerge binary sync messages.
type SyncHandler struct {
	store    *store.Store
	maxBytes int64
	log      *slog.Logger
}

// NewSyncHandler creates a new automerge sync handler.
func NewSyncHandler(st *store.Store, maxDocSizeMB int, log *slog.Logger) *SyncHandler {
	limit := int64(maxDocSizeMB) * 1024 * 1024
	if limit > maxBodyBytes {
		limit = maxBodyBytes
	}
	return &SyncHandler{store: st, maxBytes: limit, log: log}
}

// ServeHTTP handles the automerge sync protocol over HTTP.
//
//	POST /crdt/sync/:doc_id
//	Content-Type: application/octet-stream
//	Body: automerge binary sync message
//
//	Response 200: automerge binary sync reply
//	Response 413: document state exceeds max size
func (h *SyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, docID string) {
	// Enforce max doc size on the incoming body.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes+1)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
			http.Error(w, "document too large", http.StatusRequestEntityTooLarge)
			return
		}
		h.log.Warn("automerge: read body failed", "doc", docID, "err", err)
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}

	if int64(len(body)) > h.maxBytes {
		http.Error(w, "document too large", http.StatusRequestEntityTooLarge)
		return
	}

	ctx := r.Context()

	// Load current state (if exists) to compute sync reply.
	existing, err := h.store.LoadDocument(ctx, docID)
	var currentState []byte
	if err == nil {
		currentState = existing.State
	}

	// Persist the incoming sync message as the new document state.
	// A proper automerge implementation would apply the sync message
	// to the document and return the resulting sync reply.
	// Here we store the latest payload and echo the current state as the reply.
	if err := h.store.UpsertDocument(ctx, docID, "automerge", body); err != nil {
		h.log.Error("automerge: upsert failed", "doc", docID, "err", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	// Append to update log.
	clientID := r.Header.Get("X-Client-ID")
	if err := h.store.AppendUpdate(ctx, docID, clientID, body); err != nil {
		h.log.Warn("automerge: append update failed", "doc", docID, "err", err)
		// Non-fatal: state is already persisted; log is best-effort.
	}

	// Return the previous state as the sync reply (or empty bytes for new docs).
	// Automerge clients use this to compute their local merge.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	if currentState != nil {
		_, _ = w.Write(currentState)
	}
}

// HealthResponse is returned by GET /health.
type HealthResponse struct {
	Status string `json:"status"`
}

// WriteHealthOK writes a simple health response.
func WriteHealthOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
}
