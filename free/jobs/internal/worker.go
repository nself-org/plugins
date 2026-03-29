package internal

import (
	"context"
	"log"
	"time"
)

// Worker polls the database for pending jobs and marks them completed.
// This is a simplified implementation: the "processing" step is a no-op
// (the job payload is acknowledged). Real processing would dispatch based
// on queue name or payload content to external systems.
type Worker struct {
	db          *DB
	interval    time.Duration
	maxAttempts int
}

// NewWorker creates a Worker.
func NewWorker(db *DB, interval time.Duration, maxAttempts int) *Worker {
	return &Worker{
		db:          db,
		interval:    interval,
		maxAttempts: maxAttempts,
	}
}

// Run polls for jobs until the context is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("worker: shutting down")
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
			log.Printf("worker: claim error: %v", err)
			return
		}
		if job == nil {
			return // no more pending jobs
		}

		w.process(ctx, job)
	}
}

func (w *Worker) process(ctx context.Context, job *Job) {
	log.Printf("worker: processing job %s (queue=%s, attempt=%d/%d)",
		job.ID, job.Queue, job.Attempts, job.MaxAttempts)

	// Simplified processing: treat all jobs as successful.
	// In a production system this would dispatch to registered handlers
	// based on queue name or job payload type, make HTTP callbacks, etc.
	err := w.execute(ctx, job)

	if err != nil {
		log.Printf("worker: job %s failed: %v", job.ID, err)
		if dbErr := w.db.FailJob(ctx, job.ID, err.Error(), w.maxAttempts); dbErr != nil {
			log.Printf("worker: failed to record failure for job %s: %v", job.ID, dbErr)
		}
		return
	}

	if dbErr := w.db.CompleteJob(ctx, job.ID); dbErr != nil {
		log.Printf("worker: failed to complete job %s: %v", job.ID, dbErr)
		return
	}

	log.Printf("worker: job %s completed", job.ID)
}

// execute runs the job. Currently a no-op that always succeeds.
// Extend this to add real processing logic (HTTP callbacks, etc.).
func (w *Worker) execute(_ context.Context, _ *Job) error {
	return nil
}
