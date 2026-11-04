package internal

import "time"

// CronJob represents a stored cron job.
type CronJob struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CronExpr    string     `json:"cron_expr"`
	CallbackURL string     `json:"callback_url"`
	Payload     *string    `json:"payload,omitempty"`
	Enabled     bool       `json:"enabled"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`
	RunCount    int64      `json:"run_count"`
	MaxAttempts int        `json:"max_attempts"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CronRun represents a single execution record.
type CronRun struct {
	ID          string     `json:"id"`
	JobID       string     `json:"job_id"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Status      string     `json:"status"`
	HTTPStatus  *int       `json:"http_status,omitempty"`
	Error       *string    `json:"error,omitempty"`
	DurationMs  *int64     `json:"duration_ms,omitempty"`
	Attempt     int        `json:"attempt"`
}
