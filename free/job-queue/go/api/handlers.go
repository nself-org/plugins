// Package api implements the HTTP API and worker infrastructure for the
// job-queue plugin. It uses Redis BRPOPLPUSH for at-least-once delivery
// and writes visibility snapshots to Postgres np_jobqueue_jobs.
package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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
	cfg Config
	rdb *redis.Client
	db  database
	log *slog.Logger
	mu  sync.RWMutex
	mux *http.ServeMux
}

// database is the query surface shared by the migrated pool and test doubles.
type database interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// NewServer initialises connections and registers HTTP routes.
func NewServer(cfg Config, log *slog.Logger, pool *pgxpool.Pool) (*Server, error) {
	if pool == nil {
		return nil, fmt.Errorf("Postgres pool is required")
	}
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
	}
	rdb := redis.NewClient(opt)

	mux := http.NewServeMux()
	s := &Server{cfg: cfg, rdb: rdb, db: pool, log: log, mux: mux}
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

// HTTP handlers (handleHealth lives in health.go)

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
	if len(req.Payload) == 0 {
		req.Payload = json.RawMessage(`{}`)
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO public.np_jobqueue_jobs (id, queue, job_type, payload) VALUES ($1, $2, $3, $4)`, id, req.Queue, req.JobType, string(req.Payload)); err != nil {
		http.Error(w, "snapshot insert failed", http.StatusServiceUnavailable)
		return
	}
	job := jobPayload{ID: id, JobType: req.JobType, Payload: req.Payload}
	raw, _ := json.Marshal(job)

	if err := s.rdb.LPush(r.Context(), "queue:"+req.Queue, raw).Err(); err != nil {
		s.updateJobStatus(r.Context(), id, "failed", "redis enqueue failed")
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
	rows, err := s.db.Query(r.Context(),
		`SELECT j.id, j.queue, j.job_type, d.final_error, d.dlq_at
		 FROM public.np_job_dlq d JOIN public.np_jobqueue_jobs j ON d.job_id = j.id
		 WHERE j.source_account_id='primary' AND d.source_account_id='primary'
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
	items := []dlqRow{}
	for rows.Next() {
		var row dlqRow
		if err := rows.Scan(&row.ID, &row.Queue, &row.JobType, &row.FinalError, &row.DLQAt); err != nil {
			http.Error(w, "query failed", http.StatusInternalServerError)
			return
		}
		items = append(items, row)
	}
	if rows.Err() != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (s *Server) handleRequeue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if s.db == nil {
		http.Error(w, "postgres not configured", http.StatusServiceUnavailable)
		return
	}

	// Restore the stored execution payload before removing the DLQ entry.
	var queue, jobType, dlqID string
	var payload json.RawMessage
	if err := s.db.QueryRow(r.Context(), `SELECT d.id, j.queue, j.job_type, j.payload FROM public.np_job_dlq d JOIN public.np_jobqueue_jobs j ON d.job_id=j.id WHERE j.id=$1 AND j.source_account_id='primary' AND d.source_account_id='primary'`, req.JobID).Scan(&dlqID, &queue, &jobType, &payload); err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "job not found in DLQ", http.StatusNotFound)
		} else {
			http.Error(w, "query failed", http.StatusInternalServerError)
		}
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE public.np_jobqueue_jobs SET status='pending', error=NULL, attempt_count=0 WHERE id=$1 AND source_account_id='primary'`, req.JobID); err != nil {
		http.Error(w, "snapshot update failed", http.StatusServiceUnavailable)
		return
	}
	raw, _ := json.Marshal(jobPayload{ID: req.JobID, JobType: jobType, Payload: payload})
	if err := s.rdb.LPush(r.Context(), "queue:"+queue, raw).Err(); err != nil {
		http.Error(w, "redis enqueue failed", http.StatusServiceUnavailable)
		return
	}
	// Delete only the specific DLQ entry we requeued, not all entries for this
	// job_id. A broad DELETE can race with a fast worker that re-fails and
	// inserts a fresh DLQ row before this DELETE executes (MaxAttempts=1).
	if _, err := s.db.Exec(r.Context(), `DELETE FROM public.np_job_dlq WHERE id=$1 AND source_account_id='primary'`, dlqID); err != nil {
		http.Error(w, "DLQ delete failed", http.StatusServiceUnavailable)
		return
	}

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
	if _, err := s.db.Exec(r.Context(), `UPDATE public.np_jobqueue_jobs SET status='dead' WHERE id=$1 AND source_account_id='primary'`, req.JobID); err != nil {
		http.Error(w, "snapshot update failed", http.StatusServiceUnavailable)
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM public.np_job_dlq WHERE job_id=$1 AND source_account_id='primary'`, req.JobID); err != nil {
		http.Error(w, "DLQ delete failed", http.StatusServiceUnavailable)
		return
	}

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
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}
