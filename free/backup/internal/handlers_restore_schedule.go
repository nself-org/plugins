package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

func (h *Handler) runRestore(jobID, filePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	args := []string{
		"--no-owner",
		"--no-acl",
		"--verbose",
		"--dbname", h.databaseURL,
		filePath,
	}

	cmd := exec.CommandContext(ctx, h.pgRestorePath, args...)
	cmd.Env = h.pgEnv()

	// Capture stderr for diagnostics. Stdout from pg_restore is typically
	// empty (it writes directly to the database), so we capture stderr via
	// a pipe to avoid CombinedOutput (TRAP 3).
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("restore: stderr pipe: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("restore: start pg_restore: %v", err)
		return
	}

	stderrBytes, _ := io.ReadAll(stderr)

	if err := cmd.Wait(); err != nil {
		log.Printf("restore: pg_restore failed for job %s: %v\nstderr: %s", jobID, err, string(stderrBytes))
		return
	}

	log.Printf("restore: job %s completed successfully", jobID)
}

// --------------------------------------------------------------------------
// POST /v1/schedules — create a schedule
// --------------------------------------------------------------------------

type createScheduleRequest struct {
	CronExpr string `json:"cron_expr"`
	Enabled  *bool  `json:"enabled"`
}

func (h *Handler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req createScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.CronExpr == "" {
		sdk.Error(w, http.StatusBadRequest, fmt.Errorf("cron_expr is required"))
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	sc, err := h.store.InsertSchedule(r.Context(), req.CronExpr, enabled)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create schedule: %w", err))
		return
	}

	sdk.Respond(w, http.StatusCreated, sc)
}

// --------------------------------------------------------------------------
// GET /v1/schedules — list schedules
// --------------------------------------------------------------------------

func (h *Handler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	schedules, err := h.store.ListSchedules(r.Context(), limit, offset)
	if err != nil {
		sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list schedules: %w", err))
		return
	}

	if schedules == nil {
		schedules = []BackupSchedule{}
	}

	sdk.Respond(w, http.StatusOK, map[string]interface{}{
		"data":   schedules,
		"limit":  limit,
		"offset": offset,
	})
}
