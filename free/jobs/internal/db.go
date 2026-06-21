package internal

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Job status constants.
const (
	StatusPending   = "pending"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// Job represents a row in np_jobs_jobs.
type Job struct {
	ID           string          `json:"id"`
	Queue        string          `json:"queue"`
	Payload      json.RawMessage `json:"payload"`
	Priority     int             `json:"priority"`
	Status       string          `json:"status"`
	Attempts     int             `json:"attempts"`
	MaxAttempts  int             `json:"max_attempts"`
	ScheduledAt  time.Time       `json:"scheduled_at"`
	StartedAt    *time.Time      `json:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at"`
	Error        *string         `json:"error"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Queue represents a row in np_jobs_queues.
type Queue struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// QueueStats holds aggregate counts for a queue.
type QueueStats struct {
	Name      string `json:"name"`
	Pending   int64  `json:"pending"`
	Active    int64  `json:"active"`
	Completed int64  `json:"completed"`
	Failed    int64  `json:"failed"`
}

// DB wraps pgxpool and provides parameterized queries.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new DB handle.
