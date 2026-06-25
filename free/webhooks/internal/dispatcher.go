package internal

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	sdkhttpx "github.com/nself-org/plugin-sdk/httpx"
)

// ValidateWebhookURL verifies that a webhook destination URL is safe to
// deliver to. It delegates to the canonical shared SSRF guard in
// github.com/nself-org/plugin-sdk/httpx.ValidateOutboundURL.
//
// Returns nil when the URL is safe, or a descriptive error otherwise.
func ValidateWebhookURL(rawURL string) error {
	return sdkhttpx.ValidateOutboundURL(rawURL)
}

// Dispatcher handles webhook delivery with retry logic, HMAC signing, and DLQ.
//
// Concurrency control: a semaphore channel (sem) caps the number of goroutines
// that can hold a DB connection simultaneously. This prevents connection-pool
// exhaustion under sustained traffic or post-downtime backlogs.
// Configured via WEBHOOK_DISPATCHER_CONCURRENCY (default 50, max 200).
type Dispatcher struct {
	pool              *pgxpool.Pool
	client            *http.Client
	maxAttempts       int
	requestTimeoutMs  int
	retryDelays       []time.Duration
	autoDisableThresh int
	sem               chan struct{} // semaphore: bounded concurrent deliveries
	maxConcurrency    int           // cap used for warning threshold
	stopCh            chan struct{}
}

// TestResult holds the outcome of a test webhook delivery.
type TestResult struct {
	Success      bool   `json:"success"`
	Status       *int   `json:"status,omitempty"`
	ResponseTime *int   `json:"response_time,omitempty"`
	Error        string `json:"error,omitempty"`
}

// DispatchResult holds the outcome of dispatching an event.
type DispatchResult struct {
	Dispatched int      `json:"dispatched"`
	Endpoints  []string `json:"endpoints"`
}

// NewDispatcher creates a new Dispatcher with configuration from environment.
//
// The semaphore (sem) is initialised here so it is guaranteed to be non-nil
// before any Deliver() / processPending() call — avoids a nil-channel panic if
// the dispatcher is shared across HTTP handlers.
