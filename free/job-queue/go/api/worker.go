package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// StartWorkers launches one goroutine per queue × concurrency worker.
func (s *Server) StartWorkers(ctx context.Context) error {
	for _, q := range s.cfg.Queues {
		for i := 0; i < s.cfg.Concurrency; i++ {
			go s.worker(ctx, q)
		}
		s.log.Info("workers started", "queue", q, "concurrency", s.cfg.Concurrency)
	}
	return nil
}

// worker blocks on BRPOPLPUSH, processes each job, and updates np_jobqueue_jobs status.
func (s *Server) worker(ctx context.Context, queue string) {
	qKey := "queue:" + queue
	processingKey := "queue:" + queue + ":processing"
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// BRPOPLPUSH: atomically move job from tail of qKey to head of processingKey.
		result, err := s.rdb.BRPopLPush(ctx, qKey, processingKey, 5*time.Second).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("BRPOPLPUSH error", "queue", queue, "err", err)
			time.Sleep(time.Second)
			continue
		}

		s.processJob(ctx, queue, processingKey, result)
	}
}

type jobPayload struct {
	ID           string          `json:"id"`
	JobType      string          `json:"job_type"`
	Payload      json.RawMessage `json:"payload"`
	AttemptCount int             `json:"attempt_count"`
}

func (s *Server) processJob(ctx context.Context, queue, processingKey, raw string) {
	var job jobPayload
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		s.log.Error("invalid job payload", "raw", raw, "err", err)
		s.rdb.LRem(ctx, processingKey, 1, raw)
		return
	}

	s.log.Info("processing job", "id", job.ID, "type", job.JobType, "queue", queue, "attempt", job.AttemptCount+1)
	s.updateJobStatus(ctx, job.ID, "active", "")

	// Dispatch to registered handlers (stub: log only — real handlers registered via SDK).
	err := s.dispatch(ctx, job.JobType, job.Payload)

	if err != nil {
		job.AttemptCount++
		if job.AttemptCount >= s.cfg.MaxAttempts {
			s.log.Warn("job exhausted retries, moving to DLQ", "id", job.ID, "attempts", job.AttemptCount)
			s.updateJobStatus(ctx, job.ID, "dead", err.Error())
			s.moveToDLQ(ctx, job.ID, err.Error())
		} else {
			delay := retryDelay(job.AttemptCount)
			s.log.Info("job failed, scheduling retry", "id", job.ID, "delay", delay, "attempt", job.AttemptCount)
			s.updateJobStatus(ctx, job.ID, "pending", err.Error())
			// Preserve the incremented attempt count in the Redis execution payload.
			retryRaw, _ := json.Marshal(job)
			// Re-enqueue after delay
			go func(raw string, delay time.Duration) {
				time.Sleep(delay)
				s.rdb.LPush(ctx, "queue:"+queue, raw)
			}(string(retryRaw), delay)
		}
	} else {
		s.updateJobStatus(ctx, job.ID, "completed", "")
	}

	// Remove from processing set regardless of outcome.
	s.rdb.LRem(ctx, processingKey, 1, raw)
}

// dispatch calls the registered handler for jobType.
// Returns an error for unknown job types so jobs fail fast and route through
// the retry/DLQ path instead of being silently marked completed.
// Wire real handlers here or use the SDK RegisterHandler bridge once available.
func (s *Server) dispatch(_ context.Context, jobType string, _ json.RawMessage) error {
	s.log.Warn("no handler registered for job type; failing job so it routes to retry/DLQ",
		"job_type", jobType)
	return fmt.Errorf("no handler registered for job_type %q", jobType)
}

// retryDelay returns exponential backoff: 2^attempt seconds, capped at 1h.
func retryDelay(attempt int) time.Duration {
	secs := math.Pow(2, float64(attempt))
	if secs > 3600 {
		secs = 3600
	}
	return time.Duration(secs) * time.Second
}

func (s *Server) updateJobStatus(ctx context.Context, id, status, errMsg string) {
	if s.db == nil {
		return
	}
	now := time.Now().UTC()
	var q string
	var args []interface{}
	switch status {
	case "active":
		q = `UPDATE public.np_jobqueue_jobs SET status=$1, started_at=$2, attempt_count=attempt_count+1 WHERE id=$3 AND source_account_id='primary'`
		args = []interface{}{status, now, id}
	case "completed":
		q = `UPDATE public.np_jobqueue_jobs SET status=$1, completed_at=$2 WHERE id=$3 AND source_account_id='primary'`
		args = []interface{}{status, now, id}
	default:
		q = `UPDATE public.np_jobqueue_jobs SET status=$1, error=$2 WHERE id=$3 AND source_account_id='primary'`
		args = []interface{}{status, nullStr(errMsg), id}
	}
	if _, err := s.db.Exec(ctx, q, args...); err != nil {
		s.log.Warn("failed to update job status in Postgres", "id", id, "err", err)
	}
}

func (s *Server) moveToDLQ(ctx context.Context, jobID, finalError string) {
	if s.db == nil {
		return
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO public.np_job_dlq (job_id, final_error) SELECT id, $2 FROM public.np_jobqueue_jobs WHERE id=$1 AND source_account_id='primary'`, jobID, finalError)
	if err != nil {
		s.log.Warn("failed to insert into np_job_dlq", "job_id", jobID, "err", err)
	}
}
