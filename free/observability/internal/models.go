package internal

import "time"

// Service represents a monitored service in np_observability_services.
type Service struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Name            string     `json:"name"`
	URL             *string    `json:"url"`
	CheckType       string     `json:"check_type"`
	CheckIntervalSec int       `json:"check_interval_sec"`
	TimeoutSec      int        `json:"timeout_sec"`
	Status          string     `json:"status"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// HealthHistory represents a row in np_observability_health_history.
type HealthHistory struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	ServiceID       string     `json:"service_id"`
	Status          string     `json:"status"`
	ResponseTimeMs  *int       `json:"response_time_ms"`
	StatusCode      *int       `json:"status_code"`
	ErrorMessage    *string    `json:"error_message"`
	CheckedAt       time.Time  `json:"checked_at"`
}

// WatchdogEvent represents a row in np_observability_watchdog_events.
type WatchdogEvent struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	ServiceID       *string    `json:"service_id"`
	EventType       string     `json:"event_type"`
	Details         *string    `json:"details"`
	OccurredAt      time.Time  `json:"occurred_at"`
}

// HealthSummary is the response for GET /api/v1/health.
type HealthSummary struct {
	Total     int       `json:"total"`
	Up        int       `json:"up"`
	Down      int       `json:"down"`
	Degraded  int       `json:"degraded"`
	Unknown   int       `json:"unknown"`
	Timestamp time.Time `json:"timestamp"`
}

// Stats is the response for GET /api/v1/stats.
type Stats struct {
	Total          int     `json:"total_services"`
	Up             int     `json:"up"`
	Down           int     `json:"down"`
	Degraded       int     `json:"degraded"`
	AvgResponseMs  *int    `json:"avg_response_time_ms"`
}
