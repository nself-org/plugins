package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

func (h *Handler) ListBackups(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	jobs, err := h.store.ListJobs(r.Context(), limit, offset)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list backups: %w", err))
		return
	}

	if jobs == nil {
		jobs = []BackupJob{}
	}

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"data":   jobs,
		"limit":  limit,
		"offset": offset,
	})
}

// --------------------------------------------------------------------------
// GET /v1/backups/{id} — get backup status
// --------------------------------------------------------------------------

func (h *Handler) GetBackup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("missing backup id"))
		return
	}

	job, err := h.store.GetJob(r.Context(), id)
	if err != nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("backup not found"))
		return
	}

	sdk.Respond(w, http.StatusOK, job)
}

// --------------------------------------------------------------------------
// DELETE /v1/backups/{id} — delete a backup
// --------------------------------------------------------------------------

func (h *Handler) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("missing backup id"))
		return
	}

	ctx := r.Context()

	// Retrieve the job first to get the file path.
	job, err := h.store.GetJob(ctx, id)
	if err != nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("backup not found"))
		return
	}

	// Delete the backup file if it exists.
	if job.Path != nil && *job.Path != "" {
		_ = os.Remove(*job.Path)
	}

	deleted, err := h.store.DeleteJob(ctx, id)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete backup: %w", err))
		return
	}
	if !deleted {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("backup not found"))
		return
	}

	sdk.Respond(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --------------------------------------------------------------------------
// POST /v1/restore — trigger restore from backup
// --------------------------------------------------------------------------

type createRestoreRequest struct {
	BackupID string `json:"backup_id"`
}

func (h *Handler) CreateRestore(w http.ResponseWriter, r *http.Request) {
	var req createRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.BackupID == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("backup_id is required"))
		return
	}

	ctx := r.Context()

	job, err := h.store.GetJob(ctx, req.BackupID)
	if err != nil {
		sdk.Error(w, http.StatusNotFound, fmt.Errorf("backup not found"))
		return
	}
	if job.Status != "completed" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("backup is not completed (status: %s)", job.Status))
		return
	}
	if job.Path == nil || *job.Path == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("backup has no file path"))
		return
	}

	// Verify file exists.
	if _, err := os.Stat(*job.Path); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("backup file not found on disk"))
		return
	}

	// Run pg_restore asynchronously.
	go h.runRestore(job.ID, *job.Path)

	sdk.Respond(w, http.StatusAccepted, map[string]string{
		"status":    "restoring",
		"backup_id": job.ID,
	})
}

// runRestore executes pg_restore using StdoutPipe streaming.
