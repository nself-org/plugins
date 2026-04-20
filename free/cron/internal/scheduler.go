package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	sdk "github.com/nself-org/plugin-sdk"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

// App holds shared state for the cron scheduler and HTTP handlers.
type App struct {
	Pool    *pgxpool.Pool
	Config  *sdk.Config
	Client  *http.Client
	// LockMgr handles advisory-lock-based cron job overlap detection (T05).
	// Initialized in NewApp. Never nil after construction.
	LockMgr *LockManager
	// OverlapCount tracks per-job skips for Prometheus metrics (T05).
	OverlapCount *OverlapCounter
}

// NewApp creates a new App instance.
func NewApp(pool *pgxpool.Pool, cfg *sdk.Config) *App {
	timeout := 30 * time.Second
	if v := os.Getenv("CRON_TIMEOUT_SECS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
		}
	}

	counter := NewOverlapCounter()
	return &App{
		Pool:         pool,
		Config:       cfg,
		Client:       &http.Client{Timeout: timeout},
		OverlapCount: counter,
		LockMgr:      NewLockManager(pool, counter),
	}
}

// NextRunTime calculates the next run time from a cron expression.
// Returns nil if the expression is invalid.
func NextRunTime(expr string) *time.Time {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return nil
	}
	next := schedule.Next(time.Now().UTC())
	return &next
}

// ValidateCronExpr returns an error if the expression is invalid.
func ValidateCronExpr(expr string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	_, err := parser.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid cron expression '%s': %w", expr, err)
	}
	return nil
}

// RecoverMissed finds and executes jobs whose next_run_at is in the past.
func (a *App) RecoverMissed() {
	ids, err := GetDueJobIDs(a.Pool)
	if err != nil {
		log.Printf("recover missed: %v", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	log.Printf("recovering %d missed job(s)", len(ids))
	for _, id := range ids {
		go a.ExecuteJob(id)
	}
}

// StartScheduler launches a background goroutine that polls for due jobs
// every 30 seconds.
func (a *App) StartScheduler() {
	go func() {
		log.Println("cron scheduler started (30s poll interval)")
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			a.runDueJobs()
		}
	}()
}

// runDueJobs finds all enabled jobs whose next_run_at is past and executes them.
func (a *App) runDueJobs() {
	ids, err := GetDueJobIDs(a.Pool)
	if err != nil {
		log.Printf("scheduler error: %v", err)
		return
	}
	for _, id := range ids {
		go a.ExecuteJob(id)
	}
}

// ExecuteJob runs a single job by ID with retry logic (exponential backoff).
// Uses advisory lock (pg_try_advisory_lock) to prevent concurrent execution
// of the same job. If the previous instance is still running, emits
// cron_job_overlap_skipped metric and skips this tick. (S42-T05)
func (a *App) ExecuteJob(jobID string) {
	job, err := GetJob(a.Pool, jobID)
	if err != nil || job == nil {
		return
	}

	// Overlap detection: acquire advisory lock.
	// Non-blocking — returns immediately if lock not available.
	ctx := context.Background()
	jobRunCtx := JobRunContext{
		Lock:    a.LockMgr,
		JobName: job.Name,
	}
	shouldRun, releaseLock := SkipIfOverlapping(ctx, jobRunCtx)
	if !shouldRun {
		// Previous instance still running — skip this tick.
		return
	}
	defer releaseLock()

	maxAttempts := job.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if maxAttempts > 5 {
		maxAttempts = 5
	}

	start := time.Now()
	var lastSuccess bool
	var lastHTTPStatus *int
	var lastError *string
	var finalAttempt int

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		success, httpStatus, errMsg := a.dispatchWebhook(job)
		lastSuccess = success
		lastHTTPStatus = httpStatus
		lastError = errMsg
		finalAttempt = attempt

		if success {
			break
		}

		// Exponential backoff before retry: 1s, 2s, 4s, ...
		if attempt < maxAttempts {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("job %s attempt %d/%d failed: %v, retrying in %s",
				job.Name, attempt, maxAttempts, errMsg, backoff)
			time.Sleep(backoff)
		}
	}

	durationMs := time.Since(start).Milliseconds()

	if lastSuccess {
		log.Printf("job %s completed successfully (attempt %d, %dms)", job.Name, finalAttempt, durationMs)
	} else {
		errStr := "(unknown)"
		if lastError != nil {
			errStr = *lastError
		}
		log.Printf("job %s failed after %d attempts: %s", job.Name, finalAttempt, errStr)
	}

	if err := RecordRun(a.Pool, jobID, lastSuccess, lastHTTPStatus, lastError, durationMs, finalAttempt); err != nil {
		log.Printf("failed to record run for job %s: %v", jobID, err)
	}
}

// dispatchWebhook POSTs the callback URL and returns (success, httpStatus, error).
func (a *App) dispatchWebhook(job *CronJob) (bool, *int, *string) {
	var body []byte
	if job.Payload != nil {
		body = []byte(*job.Payload)
	} else {
		body = []byte("{}")
	}

	req, err := http.NewRequest("POST", job.CallbackURL, bytes.NewReader(body))
	if err != nil {
		msg := fmt.Sprintf("request creation failed: %v", err)
		return false, nil, &msg
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nself-cron/1.0")

	resp, err := a.Client.Do(req)
	if err != nil {
		msg := err.Error()
		return false, nil, &msg
	}
	defer resp.Body.Close()

	code := resp.StatusCode
	if code >= 200 && code < 300 {
		return true, &code, nil
	}

	msg := fmt.Sprintf("HTTP %d", code)
	return false, &code, &msg
}

// StartRetentionCleanup launches a background goroutine that prunes old run
// history daily at 02:00 UTC.
func (a *App) StartRetentionCleanup() {
	retentionDays := 90
	if v := os.Getenv("CRON_RETENTION_DAYS"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			retentionDays = d
		}
	}

	go func() {
		for {
			now := time.Now().UTC()
			// Calculate next 02:00 UTC.
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 2, 0, 0, 0, time.UTC)
			sleepDur := next.Sub(now)
			time.Sleep(sleepDur)

			deleted, err := PruneOldRuns(a.Pool, retentionDays)
			if err != nil {
				log.Printf("retention cleanup error: %v", err)
			} else if deleted > 0 {
				log.Printf("retention cleanup: pruned %d old run(s) (retention=%d days)", deleted, retentionDays)
			}
		}
	}()
}

// marshalJSON is a helper to produce JSON bytes from a value, ignoring errors.
func marshalJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
