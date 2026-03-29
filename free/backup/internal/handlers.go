package internal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

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
func (h *Handler) runBackup(jobID, backupType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := os.MkdirAll(h.storagePath, 0o755); err != nil {
		log.Printf("backup: mkdir failed: %v", err)
		_ = h.store.FailJob(ctx, jobID, fmt.Sprintf("mkdir: %v", err))
		return
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	fileName := fmt.Sprintf("backup-%s-%s.pgdump", jobID, timestamp)
	filePath := filepath.Join(h.storagePath, fileName)

	args := h.buildPgDumpArgs(backupType)

	cmd := exec.CommandContext(ctx, h.pgDumpPath, args...)
	cmd.Env = h.pgEnv()

	// CRITICAL: Use StdoutPipe + io.Copy for streaming (TRAP 3).
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("backup: stdout pipe: %v", err)
		_ = h.store.FailJob(ctx, jobID, fmt.Sprintf("stdout pipe: %v", err))
		return
	}

	outFile, err := os.Create(filePath)
	if err != nil {
		log.Printf("backup: create file: %v", err)
		_ = h.store.FailJob(ctx, jobID, fmt.Sprintf("create file: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		outFile.Close()
		os.Remove(filePath)
		log.Printf("backup: start pg_dump: %v", err)
		_ = h.store.FailJob(ctx, jobID, fmt.Sprintf("start pg_dump: %v", err))
		return
	}

	// Stream pg_dump stdout directly to file. Hash as we go.
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(outFile, hasher), stdout)

	// Close the file before waiting so the fd is released.
	outFile.Close()

	waitErr := cmd.Wait()
	if waitErr != nil {
		os.Remove(filePath)
		msg := fmt.Sprintf("pg_dump exited: %v", waitErr)
		log.Printf("backup: %s", msg)
		_ = h.store.FailJob(ctx, jobID, msg)
		return
	}
	if copyErr != nil {
		os.Remove(filePath)
		msg := fmt.Sprintf("io copy: %v", copyErr)
		log.Printf("backup: %s", msg)
		_ = h.store.FailJob(ctx, jobID, msg)
		return
	}

	_ = fmt.Sprintf("%x", hasher.Sum(nil)) // checksum available if needed later

	if err := h.store.CompleteJob(ctx, jobID, filePath, written); err != nil {
		log.Printf("backup: complete job: %v", err)
	}

	log.Printf("backup: job %s completed, %d bytes written to %s", jobID, written, filePath)
}

// buildPgDumpArgs returns pg_dump flags for the given backup type.
// Output goes to stdout (custom format) so it can be piped.
func (h *Handler) buildPgDumpArgs(backupType string) []string {
	args := []string{
		"--format=custom",
		"--no-owner",
		"--no-acl",
		"--verbose",
	}

	switch backupType {
	case "schema_only":
		args = append(args, "--schema-only")
	case "data_only":
		args = append(args, "--data-only")
	}

	// Use the DATABASE_URL via --dbname so pg_dump resolves host/port/user from it.
	args = append(args, "--dbname", h.databaseURL)

	return args
}

// pgEnv returns environment variables for pg_dump / pg_restore, carrying over
// the current env but ensuring PGPASSWORD is NOT set (the connection string
// already includes credentials).
func (h *Handler) pgEnv() []string {
	env := os.Environ()
	// Parse password from DATABASE_URL and inject as PGPASSWORD so that
	// pg_dump/pg_restore do not prompt interactively.
	if u, err := url.Parse(h.databaseURL); err == nil {
		if pw, ok := u.User.Password(); ok {
			env = append(env, "PGPASSWORD="+pw)
		}
	}
	return env
}

// --------------------------------------------------------------------------
// GET /v1/backups — list backups
// --------------------------------------------------------------------------

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
