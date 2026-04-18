package internal

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handlers groups HTTP handler methods.
type Handlers struct {
	db *DB
}

// NewHandlers creates a Handlers instance.
func NewHandlers(db *DB) *Handlers {
	return &Handlers{db: db}
}

// createJobRequest is the JSON body for POST /v1/jobs.
//
// S18: callback_url + sign_payload enable real HTTP dispatch. Omit
// callback_url to keep the legacy ack-only behavior.
type createJobRequest struct {
	Queue       string          `json:"queue"`
	Payload     json.RawMessage `json:"payload"`
	Priority    int             `json:"priority"`
	DelayMS     int64           `json:"delay_ms"`
	MaxAttempts int             `json:"max_attempts"`
	CallbackURL string          `json:"callback_url"`
	SignPayload *bool           `json:"sign_payload"`
	AccountID   string          `json:"account_id"`
}

// CreateJob handles POST /v1/jobs.
func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	if req.Queue == "" {
		req.Queue = "default"
	}
	if req.Payload == nil {
		req.Payload = json.RawMessage(`{}`)
	}
	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 3
	}

	delay := time.Duration(req.DelayMS) * time.Millisecond

	opts := CreateJobOpts{
		CallbackURL: req.CallbackURL,
		SignPayload: req.SignPayload,
		AccountID:   req.AccountID,
	}

	job, err := h.db.CreateJob(r.Context(), req.Queue, req.Payload, req.Priority, delay, req.MaxAttempts, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

// ListDLQ handles GET /v1/dlq.
func (h *Handlers) ListDLQ(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	if limit > 500 {
		limit = 500
	}
	jobs, err := h.db.ListDLQ(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if jobs == nil {
		jobs = []Job{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":   jobs,
		"count":  len(jobs),
		"limit":  limit,
		"offset": offset,
	})
}

// ReviveDLQ handles POST /v1/dlq/{id}/revive.
func (h *Handlers) ReviveDLQ(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.db.ReviveDLQ(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not in DLQ"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// ListJobs handles GET /v1/jobs.
func (h *Handlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	queue := r.URL.Query().Get("queue")
	status := r.URL.Query().Get("status")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	if limit > 1000 {
		limit = 1000
	}

	jobs, err := h.db.ListJobs(r.Context(), queue, status, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if jobs == nil {
		jobs = []Job{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":   jobs,
		"count":  len(jobs),
		"limit":  limit,
		"offset": offset,
	})
}

// GetJob handles GET /v1/jobs/{id}.
func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	job, err := h.db.GetJob(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// DeleteJob handles DELETE /v1/jobs/{id}.
func (h *Handlers) DeleteJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	deleted, err := h.db.DeleteJob(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// RetryJob handles POST /v1/jobs/{id}/retry.
func (h *Handlers) RetryJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	job, err := h.db.RetryJob(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if job == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found or not in failed state"})
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// ListQueues handles GET /v1/queues.
func (h *Handlers) ListQueues(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.ListQueuesWithStats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if stats == nil {
		stats = []QueueStats{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"queues": stats,
		"count":  len(stats),
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func queryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
