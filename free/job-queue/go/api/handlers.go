// Package api implements the HTTP API and worker infrastructure for the
// job-queue plugin. It uses Redis BRPOPLPUSH for at-least-once delivery
// and writes visibility snapshots to Postgres np_jobs.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds runtime configuration for the job-queue service.
type Config struct {
	RedisURL    string
	DatabaseURL string
	Port        string
	Concurrency int
	MaxAttempts int
	Queues      []string
}

// Server is the job-queue HTTP + worker server.
type Server struct {
	cfg    Config
	rdb    *redis.Client
	db     *sql.DB
	log    *slog.Logger
	mu     sync.RWMutex
	mux    *http.ServeMux
}

// NewServer initialises connections and registers HTTP routes.
func NewServer(cfg Config, log *slog.Logger) (*Server, error) {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
	}
	rdb := redis.NewClient(opt)

	mux := http.NewServeMux()
	s := &Server{cfg: cfg, rdb: rdb, log: log, mux: mux}
	s.routes()
	return s, nil
}

// Router returns the HTTP handler.
func (s *Server) Router() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/jobs", s.handleListJobs)
	s.mux.HandleFunc("/jobs/enqueue", s.handleEnqueue)
	s.mux.HandleFunc("/dlq", s.handleListDLQ)
	s.mux.HandleFunc("/dlq/requeue", s.handleRequeue)
	s.mux.HandleFunc("/dlq/discard", s.handleDiscard)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
}

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

// worker blocks on BRPOPLPUSH, processes each job, and updates np_jobs status.
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
	ID          string          `json:"id"`
	JobType     string          `json:"job_type"`
	Payload     json.RawMessage `json:"payload"`
	AttemptCount int            `json:"attempt_count"`
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
			// Re-enqueue after delay
			go func(raw string, delay time.Duration) {
				time.Sleep(delay)
				s.rdb.LPush(ctx, "queue:"+queue, raw)
			}(raw, delay)
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
		q = `UPDATE np_jobs SET status=$1, started_at=$2, attempt_count=attempt_count+1 WHERE id=$3`
		args = []interface{}{status, now, id}
	case "completed":
		q = `UPDATE np_jobs SET status=$1, completed_at=$2 WHERE id=$3`
		args = []interface{}{status, now, id}
	default:
		q = `UPDATE np_jobs SET status=$1, error=$2 WHERE id=$3`
		args = []interface{}{status, nullStr(errMsg), id}
	}
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		s.log.Warn("failed to update job status in Postgres", "id", id, "err", err)
	}
}

func (s *Server) moveToDLQ(ctx context.Context, jobID, finalError string) {
	if s.db == nil {
		return
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO np_job_dlq (job_id, final_error) VALUES ($1, $2)`, jobID, finalError)
	if err != nil {
		s.log.Warn("failed to insert into np_job_dlq", "job_id", jobID, "err", err)
	}
}

// HTTP handlers

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.rdb.Ping(r.Context()).Err(); err != nil {
		http.Error(w, `{"status":"unhealthy","redis":"down"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleEnqueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Queue   string          `json:"queue"`
		JobType string          `json:"job_type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Queue == "" || req.JobType == "" {
		http.Error(w, "queue and job_type are required", http.StatusBadRequest)
		return
	}

	id := generateID()
	job := jobPayload{ID: id, JobType: req.JobType, Payload: req.Payload}
	raw, _ := json.Marshal(job)

	if err := s.rdb.LPush(r.Context(), "queue:"+req.Queue, raw).Err(); err != nil {
		http.Error(w, "redis enqueue failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "enqueued"})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Return queue lengths from Redis
	out := map[string]int64{}
	for _, q := range s.cfg.Queues {
		n, _ := s.rdb.LLen(r.Context(), "queue:"+q).Result()
		out[q] = n
	}
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handleListDLQ(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
		return
	}
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT j.id, j.queue, j.job_type, d.final_error, d.dlq_at
		 FROM np_job_dlq d JOIN np_jobs j ON d.job_id = j.id
		 ORDER BY d.dlq_at DESC LIMIT 100`)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type dlqRow struct {
		ID         string    `json:"id"`
		Queue      string    `json:"queue"`
		JobType    string    `json:"job_type"`
		FinalError string    `json:"final_error"`
		DLQAt      time.Time `json:"dlq_at"`
	}
	var items []dlqRow
	for rows.Next() {
		var row dlqRow
		rows.Scan(&row.ID, &row.Queue, &row.JobType, &row.FinalError, &row.DLQAt)
		items = append(items, row)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (s *Server) handleRequeue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct{ JobID string `json:"job_id"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if s.db == nil {
		http.Error(w, "postgres not configured", http.StatusServiceUnavailable)
		return
	}

	// Reset job status to pending and delete from DLQ
	s.db.ExecContext(r.Context(), `UPDATE np_jobs SET status='pending', error=NULL, attempt_count=0 WHERE id=$1`, req.JobID)
	s.db.ExecContext(r.Context(), `DELETE FROM np_job_dlq WHERE job_id=$1`, req.JobID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"requeued"}`))
}

// handleDiscard permanently removes a job from the DLQ.
// Unlike requeue, discard deletes the DLQ entry and marks the job dead.
// Idempotent: discarding an already-discarded job returns 200.
func (s *Server) handleDiscard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.JobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	if s.db == nil {
		http.Error(w, "postgres not configured", http.StatusServiceUnavailable)
		return
	}
	s.db.ExecContext(r.Context(), `DELETE FROM np_job_dlq WHERE job_id=$1`, req.JobID)
	s.db.ExecContext(r.Context(), `UPDATE np_jobs SET status='dead' WHERE id=$1`, req.JobID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"discarded"}`))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Prometheus exposition format — queue depths
	var sb strings.Builder
	sb.WriteString("# HELP nself_jobqueue_queue_depth Number of pending jobs per queue.\n")
	sb.WriteString("# TYPE nself_jobqueue_queue_depth gauge\n")
	for _, q := range s.cfg.Queues {
		n, _ := s.rdb.LLen(r.Context(), "queue:"+q).Result()
		fmt.Fprintf(&sb, "nself_jobqueue_queue_depth{queue=%q} %d\n", q, n)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write([]byte(sb.String()))
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func generateID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
