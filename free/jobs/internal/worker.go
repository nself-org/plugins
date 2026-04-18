package internal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// Worker polls the database for pending jobs and dispatches them.
//
// S18 upgrade: real HTTP callback dispatch replaces the prior no-op. When a
// job has callback_url set, the worker POSTs the payload (optionally
// HMAC-SHA256 signed, matching the webhooks + cron plugin standard). On
// non-2xx or transport error the worker applies capped exponential backoff
// via FailJob; when attempts >= max_attempts the job moves to the DLQ
// (dlq=TRUE). Jobs with no callback_url are ack-processed (legacy behavior).
type Worker struct {
	db              *DB
	interval        time.Duration
	maxAttempts     int
	client          *http.Client
	signingSecret   string
	requestTimeout  time.Duration
}

// NewWorker creates a Worker with S18 dispatcher config.
func NewWorker(db *DB, interval time.Duration, maxAttempts int) *Worker {
	timeout := 30 * time.Second
	if v := os.Getenv("JOBS_REQUEST_TIMEOUT_SECS"); v != "" {
		if n, err := time.ParseDuration(v + "s"); err == nil && n > 0 {
			timeout = n
		}
	}
	signingSecret := os.Getenv("JOBS_SIGNING_SECRET")
	if signingSecret == "" {
		signingSecret = os.Getenv("PLUGIN_INTERNAL_SECRET")
	}
	return &Worker{
		db:             db,
		interval:       interval,
		maxAttempts:    maxAttempts,
		client:         &http.Client{Timeout: timeout},
		signingSecret:  signingSecret,
		requestTimeout: timeout,
	}
}

// Run polls for jobs until the context is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	log.Printf("[nself-jobs] worker started (interval=%s, max_attempts=%d, signing=%t)",
		w.interval, w.maxAttempts, w.signingSecret != "")

	for {
		select {
		case <-ctx.Done():
			log.Println("[nself-jobs] worker: shutting down")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	for {
		job, err := w.db.ClaimNextJob(ctx)
		if err != nil {
			log.Printf("[nself-jobs] worker claim error: %v", err)
			return
		}
		if job == nil {
			return // no more pending jobs
		}

		w.process(ctx, job)
	}
}

func (w *Worker) process(ctx context.Context, job *Job) {
	log.Printf("[nself-jobs] worker: processing job %s (queue=%s, attempt=%d/%d)",
		job.ID, job.Queue, job.Attempts, job.MaxAttempts)

	start := time.Now()
	success, statusCode, errMsg := w.execute(ctx, job)
	duration := time.Since(start).Milliseconds()

	// Record the attempt history regardless of outcome.
	_ = w.db.RecordJobRun(ctx, job.ID, job.Attempts, success, statusCode, errMsg, duration)

	if success {
		if dbErr := w.db.CompleteJob(ctx, job.ID); dbErr != nil {
			log.Printf("[nself-jobs] worker: failed to complete job %s: %v", job.ID, dbErr)
			return
		}
		log.Printf("[nself-jobs] worker: job %s completed in %dms", job.ID, duration)
		return
	}

	finalMsg := ""
	if errMsg != nil {
		finalMsg = *errMsg
	}

	// Attempts already incremented by ClaimNextJob. If we've exhausted retries,
	// promote to the DLQ rather than leaving the job in a silent failed state.
	if job.Attempts >= job.MaxAttempts {
		if dbErr := w.db.MarkDLQ(ctx, job.ID, finalMsg); dbErr != nil {
			log.Printf("[nself-jobs] worker: failed to mark DLQ for job %s: %v", job.ID, dbErr)
		} else {
			log.Printf("[nself-jobs] worker: job %s moved to DLQ after %d attempts: %s",
				job.ID, job.Attempts, finalMsg)
		}
		return
	}

	if dbErr := w.db.FailJob(ctx, job.ID, finalMsg, w.maxAttempts); dbErr != nil {
		log.Printf("[nself-jobs] worker: failed to record failure for job %s: %v", job.ID, dbErr)
	}
	log.Printf("[nself-jobs] worker: job %s failed (attempt %d/%d), will retry: %s",
		job.ID, job.Attempts, job.MaxAttempts, finalMsg)
}

// execute runs the job. Returns (success, statusCode, errMsg).
//
// If the job has no callback_url, it is treated as ack-only (always succeeds)
// so legacy callers keep working. With callback_url set, a POST is issued with
// optional HMAC-SHA256 signature headers matching the webhooks plugin format:
//   X-Webhook-Signature: t=<unix_ts>,v1=<hex>
//   X-Webhook-Signature-Version: v1
func (w *Worker) execute(ctx context.Context, job *Job) (bool, *int, *string) {
	callbackURL, sign, err := w.db.GetCallbackURL(ctx, job.ID)
	if err != nil {
		e := fmt.Sprintf("load callback url: %v", err)
		return false, nil, &e
	}
	if callbackURL == nil || *callbackURL == "" {
		// Ack-only path — preserved for legacy callers.
		return true, nil, nil
	}

	body := []byte(job.Payload)
	if len(body) == 0 {
		body = []byte("{}")
	}

	reqCtx, cancel := context.WithTimeout(ctx, w.requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, *callbackURL, bytes.NewReader(body))
	if err != nil {
		e := fmt.Sprintf("build request: %v", err)
		return false, nil, &e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nself-jobs/1.0")
	req.Header.Set("X-Job-Id", job.ID)
	req.Header.Set("X-Job-Queue", job.Queue)
	req.Header.Set("X-Job-Attempt", fmt.Sprintf("%d", job.Attempts))
	if token := os.Getenv("PLUGIN_INTERNAL_SECRET"); token != "" {
		req.Header.Set("X-Internal-Token", token)
	}
	if sign && w.signingSecret != "" {
		ts := time.Now().Unix()
		mac := hmac.New(sha256.New, []byte(w.signingSecret))
		fmt.Fprintf(mac, "%d.%s", ts, body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", fmt.Sprintf("t=%d,v1=%s", ts, sig))
		req.Header.Set("X-Webhook-Signature-Version", "v1")
	}

	resp, err := w.client.Do(req)
	if err != nil {
		e := err.Error()
		return false, nil, &e
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	code := resp.StatusCode
	if code >= 200 && code < 300 {
		return true, &code, nil
	}
	e := fmt.Sprintf("HTTP %d", code)
	return false, &code, &e
}
