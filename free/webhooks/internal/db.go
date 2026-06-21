package internal

import (
	"time"

)

// --- Types -------------------------------------------------------------------

// Endpoint represents a row in np_webhooks_endpoints.
type Endpoint struct {
	ID             string     `json:"id"`
	URL            string     `json:"url"`
	Description    *string    `json:"description"`
	Secret         string     `json:"secret"`
	Events         []string   `json:"events"`
	Headers        string     `json:"headers"`
	Enabled        bool       `json:"enabled"`
	FailureCount   int        `json:"failure_count"`
	LastSuccessAt  *time.Time `json:"last_success_at"`
	LastFailureAt  *time.Time `json:"last_failure_at"`
	DisabledAt     *time.Time `json:"disabled_at"`
	DisabledReason *string    `json:"disabled_reason"`
	Metadata       string     `json:"metadata"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Delivery represents a row in np_webhooks_deliveries.
type Delivery struct {
	ID             string     `json:"id"`
	EndpointID     string     `json:"endpoint_id"`
	EventType      string     `json:"event_type"`
	Payload        string     `json:"payload"`
	Status         string     `json:"status"`
	ResponseStatus *int       `json:"response_status"`
	ResponseBody   *string    `json:"response_body"`
	ResponseTimeMs *int       `json:"response_time_ms"`
	AttemptCount   int        `json:"attempt_count"`
	MaxAttempts    int        `json:"max_attempts"`
	NextRetryAt    *time.Time `json:"next_retry_at"`
	ErrorMessage   *string    `json:"error_message"`
	Signature      string     `json:"signature"`
	DeliveredAt    *time.Time `json:"delivered_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// --- Migration ---------------------------------------------------------------

// Migrate creates the required tables if they do not exist.
