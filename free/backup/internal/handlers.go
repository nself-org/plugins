package internal

import (
	"encoding/json"
	"fmt"
	"net/http"


	sdk "github.com/nself-org/plugin-sdk"
)

// Handler holds the HTTP handlers for the backup plugin.
type Handler struct {
	store         *Store
	databaseURL   string
	storagePath   string
	pgDumpPath    string
	pgRestorePath string
}

// NewHandler creates a Handler.
func NewHandler(store *Store, databaseURL, storagePath, pgDumpPath, pgRestorePath string) *Handler {
	return &Handler{
		store:         store,
		databaseURL:   databaseURL,
		storagePath:   storagePath,
		pgDumpPath:    pgDumpPath,
		pgRestorePath: pgRestorePath,
	}
}

// --------------------------------------------------------------------------
// POST /v1/backups — trigger a backup
// --------------------------------------------------------------------------

type createBackupRequest struct {
	Type string `json:"type"` // full | incremental | schema_only | data_only
}

func (h *Handler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	var req createBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	backupType := req.Type
	if backupType == "" {
		backupType = "full"
	}

	switch backupType {
	case "full", "incremental", "schema_only", "data_only":
	default:
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid backup type: %s", backupType))
		return
	}

	ctx := r.Context()

	job, err := h.store.InsertJob(ctx, backupType)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create job: %w", err))
		return
	}

	// Run pg_dump asynchronously so the HTTP response returns immediately.
	go h.runBackup(job.ID, backupType)

	sdk.Respond(w, http.StatusAccepted, job)
}

// runBackup executes pg_dump, streams output to a file, records the result.
