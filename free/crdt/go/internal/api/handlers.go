// Package api provides the HTTP REST handlers for the CRDT plugin admin API.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/nself-crdt/internal/automerge"
	"github.com/nself-org/nself-crdt/internal/store"
	"github.com/nself-org/nself-crdt/internal/yjs"
)

// Handlers bundles all HTTP handlers for the CRDT plugin.
type Handlers struct {
	store      *store.Store
	yjsHandler *yjs.Handler
	amHandler  *automerge.SyncHandler
	compactor  *automerge.Compactor
	log        *slog.Logger
}

// New creates Handlers.
func New(
	st *store.Store,
	yjsH *yjs.Handler,
	amH *automerge.SyncHandler,
	comp *automerge.Compactor,
	log *slog.Logger,
) *Handlers {
	return &Handlers{
		store:      st,
		yjsHandler: yjsH,
		amHandler:  amH,
		compactor:  comp,
		log:        log,
	}
}

// Mount registers all CRDT routes onto r.
func (h *Handlers) Mount(r chi.Router) {
	r.Get("/health", h.Health)

	// Yjs WebSocket endpoint.
	r.Get("/crdt/yjs/{docID}", h.YjsWS)

	// automerge HTTP sync endpoint.
	r.Post("/crdt/sync/{docID}", h.AutomergeSync)

	// Document management endpoints.
	r.Get("/crdt/doc/{docID}", h.GetDoc)
	r.Delete("/crdt/doc/{docID}", h.DeleteDoc)
	r.Get("/crdt/docs", h.ListDocs)

	// Admin-only endpoints (callers should verify admin token upstream).
	r.Post("/crdt/compact/{docID}", h.CompactDoc)
}

// Health returns 200 OK with {"status":"ok"}.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	automerge.WriteHealthOK(w)
}

// YjsWS upgrades the connection and runs the y-websocket server protocol.
func (h *Handlers) YjsWS(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docID")
	if docID == "" {
		http.Error(w, "docID required", http.StatusBadRequest)
		return
	}
	h.yjsHandler.ServeWS(w, r, docID)
}

// AutomergeSync handles POST /crdt/sync/:doc_id.
func (h *Handlers) AutomergeSync(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docID")
	if docID == "" {
		http.Error(w, "docID required", http.StatusBadRequest)
		return
	}
	h.amHandler.ServeHTTP(w, r, docID)
}

// docResponse is the JSON representation of a document for API responses.
type docResponse struct {
	DocID        string `json:"doc_id"`
	Engine       string `json:"engine"`
	StateSizeB   int    `json:"state_size_bytes"`
	UpdatesCount int    `json:"updates_count"`
	LastModified string `json:"last_modified"`
}

// GetDoc retrieves the current document state.
// Returns binary (Content-Type: application/octet-stream) when Accept header
// is application/octet-stream; otherwise returns JSON metadata.
func (h *Handlers) GetDoc(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docID")
	doc, err := h.store.LoadDocument(r.Context(), docID)
	if err == store.ErrNotFound {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.log.Error("get doc", "id", docID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("Accept") == "application/octet-stream" {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(doc.State)
		return
	}

	writeJSON(w, docResponse{
		DocID:        doc.DocID,
		Engine:       doc.Engine,
		StateSizeB:   len(doc.State),
		UpdatesCount: doc.UpdatesCount,
		LastModified: doc.LastModified.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// DeleteDoc removes a document and all its update history.
func (h *Handlers) DeleteDoc(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docID")
	if err := h.store.DeleteDocument(r.Context(), docID); err == store.ErrNotFound {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		h.log.Error("delete doc", "id", docID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listResponse is the JSON envelope for paginated doc lists.
type listResponse struct {
	Documents []docResponse `json:"documents"`
	Total     int           `json:"total"`
}

// ListDocs returns a paginated list of documents (admin only).
func (h *Handlers) ListDocs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	docs, err := h.store.ListDocuments(r.Context(), limit, offset)
	if err != nil {
		h.log.Error("list docs", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := listResponse{Documents: make([]docResponse, 0, len(docs))}
	for _, d := range docs {
		resp.Documents = append(resp.Documents, docResponse{
			DocID:        d.DocID,
			Engine:       d.Engine,
			StateSizeB:   len(d.State),
			UpdatesCount: d.UpdatesCount,
			LastModified: d.LastModified.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	resp.Total = len(resp.Documents)
	writeJSON(w, resp)
}

// compactResponse is the JSON envelope for compact results.
type compactResponse struct {
	DocID       string `json:"doc_id"`
	RowsDeleted int64  `json:"rows_deleted"`
}

// CompactDoc forces history compaction for a single document (admin only).
func (h *Handlers) CompactDoc(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "docID")
	deleted, err := h.compactor.CompactOne(r.Context(), docID)
	if err == store.ErrNotFound {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.log.Error("compact doc", "id", docID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, compactResponse{DocID: docID, RowsDeleted: deleted})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
	}
}
