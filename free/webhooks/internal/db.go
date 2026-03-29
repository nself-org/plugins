package internal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS np_webhooks_endpoints (
			id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			url             TEXT NOT NULL,
			description     TEXT,
			secret          VARCHAR(255) NOT NULL,
			events          TEXT[] NOT NULL,
			headers         JSONB DEFAULT '{}',
			enabled         BOOLEAN DEFAULT TRUE,
			failure_count   INTEGER DEFAULT 0,
			last_success_at TIMESTAMPTZ,
			last_failure_at TIMESTAMPTZ,
			disabled_at     TIMESTAMPTZ,
			disabled_reason TEXT,
			metadata        JSONB DEFAULT '{}',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_enabled
			ON np_webhooks_endpoints (enabled);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_events
			ON np_webhooks_endpoints USING GIN(events);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_created
			ON np_webhooks_endpoints (created_at DESC);

		CREATE TABLE IF NOT EXISTS np_webhooks_deliveries (
			id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			endpoint_id      UUID NOT NULL REFERENCES np_webhooks_endpoints(id) ON DELETE CASCADE,
			event_type       VARCHAR(128) NOT NULL,
			payload          JSONB NOT NULL,
			status           VARCHAR(32) DEFAULT 'pending',
			response_status  INTEGER,
			response_body    TEXT,
			response_time_ms INTEGER,
			attempt_count    INTEGER DEFAULT 0,
			max_attempts     INTEGER DEFAULT 5,
			next_retry_at    TIMESTAMPTZ,
			error_message    TEXT,
			signature        VARCHAR(255),
			delivered_at     TIMESTAMPTZ,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_endpoint
			ON np_webhooks_deliveries (endpoint_id);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_status
			ON np_webhooks_deliveries (status);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_event_type
			ON np_webhooks_deliveries (event_type);
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_next_retry
			ON np_webhooks_deliveries (next_retry_at) WHERE status = 'pending';
		CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_created
			ON np_webhooks_deliveries (created_at DESC);
	`)
	return err
}

// --- Helpers -----------------------------------------------------------------

// GenerateSecret returns a random webhook secret prefixed with "whsec_".
func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

// --- Endpoint CRUD -----------------------------------------------------------

// CreateEndpoint inserts a new webhook endpoint.
func CreateEndpoint(ctx context.Context, pool *pgxpool.Pool, url string, events []string, description *string, secret string, headersJSON string, metadataJSON string) (*Endpoint, error) {
	var e Endpoint
	err := pool.QueryRow(ctx, `
		INSERT INTO np_webhooks_endpoints (url, description, secret, events, headers, metadata)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb)
		RETURNING id, url, description, secret, events, headers::text, enabled,
		          failure_count, last_success_at, last_failure_at, disabled_at,
		          disabled_reason, metadata::text, created_at, updated_at
	`, url, description, secret, events, headersJSON, metadataJSON).Scan(
		&e.ID, &e.URL, &e.Description, &e.Secret, &e.Events, &e.Headers,
		&e.Enabled, &e.FailureCount, &e.LastSuccessAt, &e.LastFailureAt,
		&e.DisabledAt, &e.DisabledReason, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetEndpoint returns a single endpoint by ID.
func GetEndpoint(ctx context.Context, pool *pgxpool.Pool, id string) (*Endpoint, error) {
	var e Endpoint
	err := pool.QueryRow(ctx, `
		SELECT id, url, description, secret, events, headers::text, enabled,
		       failure_count, last_success_at, last_failure_at, disabled_at,
		       disabled_reason, metadata::text, created_at, updated_at
		FROM np_webhooks_endpoints WHERE id = $1
	`, id).Scan(
		&e.ID, &e.URL, &e.Description, &e.Secret, &e.Events, &e.Headers,
		&e.Enabled, &e.FailureCount, &e.LastSuccessAt, &e.LastFailureAt,
		&e.DisabledAt, &e.DisabledReason, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListEndpoints returns all endpoints, optionally filtered by enabled status.
func ListEndpoints(ctx context.Context, pool *pgxpool.Pool, enabledFilter *bool) ([]Endpoint, error) {
	query := `SELECT id, url, description, secret, events, headers::text, enabled,
	                 failure_count, last_success_at, last_failure_at, disabled_at,
	                 disabled_reason, metadata::text, created_at, updated_at
	          FROM np_webhooks_endpoints WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if enabledFilter != nil {
		query += fmt.Sprintf(" AND enabled = $%d", argIdx)
		args = append(args, *enabledFilter)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Endpoint
	for rows.Next() {
		var e Endpoint
		if err := rows.Scan(
			&e.ID, &e.URL, &e.Description, &e.Secret, &e.Events, &e.Headers,
			&e.Enabled, &e.FailureCount, &e.LastSuccessAt, &e.LastFailureAt,
			&e.DisabledAt, &e.DisabledReason, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// UpdateEndpoint updates fields on an existing endpoint. Only non-nil fields
// are changed. Returns the updated endpoint or nil if not found.
func UpdateEndpoint(ctx context.Context, pool *pgxpool.Pool, id string, url *string, description *string, events []string, headersJSON *string, enabled *bool, metadataJSON *string) (*Endpoint, error) {
	updates := []string{"updated_at = NOW()"}
	args := []interface{}{}
	argIdx := 1

	if url != nil {
		updates = append(updates, fmt.Sprintf("url = $%d", argIdx))
		args = append(args, *url)
		argIdx++
	}
	if description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *description)
		argIdx++
	}
	if events != nil {
		updates = append(updates, fmt.Sprintf("events = $%d", argIdx))
		args = append(args, events)
		argIdx++
	}
	if headersJSON != nil {
		updates = append(updates, fmt.Sprintf("headers = $%d::jsonb", argIdx))
		args = append(args, *headersJSON)
		argIdx++
	}
	if enabled != nil {
		updates = append(updates, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *enabled)
		argIdx++
	}
	if metadataJSON != nil {
		updates = append(updates, fmt.Sprintf("metadata = $%d::jsonb", argIdx))
		args = append(args, *metadataJSON)
		argIdx++
	}

	if len(updates) == 1 {
		return GetEndpoint(ctx, pool, id)
	}

	args = append(args, id)

	query := fmt.Sprintf(`UPDATE np_webhooks_endpoints SET %s WHERE id = $%d
		RETURNING id, url, description, secret, events, headers::text, enabled,
		          failure_count, last_success_at, last_failure_at, disabled_at,
		          disabled_reason, metadata::text, created_at, updated_at`,
		joinStrings(updates, ", "), argIdx)

	var e Endpoint
	err := pool.QueryRow(ctx, query, args...).Scan(
		&e.ID, &e.URL, &e.Description, &e.Secret, &e.Events, &e.Headers,
		&e.Enabled, &e.FailureCount, &e.LastSuccessAt, &e.LastFailureAt,
		&e.DisabledAt, &e.DisabledReason, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// DeleteEndpoint removes an endpoint by ID. Returns true if a row was deleted.
func DeleteEndpoint(ctx context.Context, pool *pgxpool.Pool, id string) (bool, error) {
	tag, err := pool.Exec(ctx,
		"DELETE FROM np_webhooks_endpoints WHERE id = $1", id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// --- Endpoint status tracking ------------------------------------------------

// RecordEndpointSuccess resets failure count and updates last_success_at.
func RecordEndpointSuccess(ctx context.Context, pool *pgxpool.Pool, id string) error {
	_, err := pool.Exec(ctx, `
		UPDATE np_webhooks_endpoints
		SET failure_count = 0, last_success_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

// RecordEndpointFailure increments failure count, auto-disables when threshold
// is reached.
func RecordEndpointFailure(ctx context.Context, pool *pgxpool.Pool, id string, autoDisableThreshold int) error {
	_, err := pool.Exec(ctx, `
		UPDATE np_webhooks_endpoints
		SET failure_count = failure_count + 1,
		    last_failure_at = NOW(),
		    enabled = CASE
		      WHEN failure_count + 1 >= $2 THEN FALSE
		      ELSE enabled
		    END,
		    disabled_at = CASE
		      WHEN failure_count + 1 >= $2 THEN NOW()
		      ELSE disabled_at
		    END,
		    disabled_reason = CASE
		      WHEN failure_count + 1 >= $2 THEN 'Auto-disabled after ' || $2 || ' consecutive failures'
		      ELSE disabled_reason
		    END,
		    updated_at = NOW()
		WHERE id = $1
	`, id, autoDisableThreshold)
	return err
}

// --- Delivery CRUD -----------------------------------------------------------

// CreateDelivery inserts a new delivery record.
func CreateDelivery(ctx context.Context, pool *pgxpool.Pool, endpointID, eventType, payloadJSON, signature string, maxAttempts int) (*Delivery, error) {
	var d Delivery
	err := pool.QueryRow(ctx, `
		INSERT INTO np_webhooks_deliveries
			(endpoint_id, event_type, payload, signature, max_attempts)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		RETURNING id, endpoint_id, event_type, payload::text, status,
		          response_status, response_body, response_time_ms,
		          attempt_count, max_attempts, next_retry_at, error_message,
		          signature, delivered_at, created_at
	`, endpointID, eventType, payloadJSON, signature, maxAttempts).Scan(
		&d.ID, &d.EndpointID, &d.EventType, &d.Payload, &d.Status,
		&d.ResponseStatus, &d.ResponseBody, &d.ResponseTimeMs,
		&d.AttemptCount, &d.MaxAttempts, &d.NextRetryAt, &d.ErrorMessage,
		&d.Signature, &d.DeliveredAt, &d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListDeliveries returns deliveries with optional filters.
func ListDeliveries(ctx context.Context, pool *pgxpool.Pool, endpointID, eventType, status string, limit, offset int) ([]Delivery, error) {
	query := `SELECT id, endpoint_id, event_type, payload::text, status,
	                 response_status, response_body, response_time_ms,
	                 attempt_count, max_attempts, next_retry_at, error_message,
	                 signature, delivered_at, created_at
	          FROM np_webhooks_deliveries WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if endpointID != "" {
		query += fmt.Sprintf(" AND endpoint_id = $%d", argIdx)
		args = append(args, endpointID)
		argIdx++
	}
	if eventType != "" {
		query += fmt.Sprintf(" AND event_type = $%d", argIdx)
		args = append(args, eventType)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(
			&d.ID, &d.EndpointID, &d.EventType, &d.Payload, &d.Status,
			&d.ResponseStatus, &d.ResponseBody, &d.ResponseTimeMs,
			&d.AttemptCount, &d.MaxAttempts, &d.NextRetryAt, &d.ErrorMessage,
			&d.Signature, &d.DeliveredAt, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// GetPendingDeliveries returns deliveries ready for processing.
func GetPendingDeliveries(ctx context.Context, pool *pgxpool.Pool, limit int) ([]Delivery, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, endpoint_id, event_type, payload::text, status,
		       response_status, response_body, response_time_ms,
		       attempt_count, max_attempts, next_retry_at, error_message,
		       signature, delivered_at, created_at
		FROM np_webhooks_deliveries
		WHERE status = 'pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(
			&d.ID, &d.EndpointID, &d.EventType, &d.Payload, &d.Status,
			&d.ResponseStatus, &d.ResponseBody, &d.ResponseTimeMs,
			&d.AttemptCount, &d.MaxAttempts, &d.NextRetryAt, &d.ErrorMessage,
			&d.Signature, &d.DeliveredAt, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// UpdateDeliveryStatus updates the status and related fields of a delivery.
func UpdateDeliveryStatus(ctx context.Context, pool *pgxpool.Pool, id, status string, responseStatus *int, responseBody *string, responseTimeMs *int, errorMessage *string, nextRetryAt *time.Time) error {
	updates := []string{"status = $2", "attempt_count = attempt_count + 1"}
	args := []interface{}{id, status}
	argIdx := 3

	if status == "delivered" {
		updates = append(updates, "delivered_at = NOW()")
	}

	if responseStatus != nil {
		updates = append(updates, fmt.Sprintf("response_status = $%d", argIdx))
		args = append(args, *responseStatus)
		argIdx++
	}
	if responseBody != nil {
		updates = append(updates, fmt.Sprintf("response_body = $%d", argIdx))
		args = append(args, *responseBody)
		argIdx++
	}
	if responseTimeMs != nil {
		updates = append(updates, fmt.Sprintf("response_time_ms = $%d", argIdx))
		args = append(args, *responseTimeMs)
		argIdx++
	}
	if errorMessage != nil {
		updates = append(updates, fmt.Sprintf("error_message = $%d", argIdx))
		args = append(args, *errorMessage)
		argIdx++
	}
	if nextRetryAt != nil {
		updates = append(updates, fmt.Sprintf("next_retry_at = $%d", argIdx))
		args = append(args, *nextRetryAt)
		argIdx++
	}

	query := fmt.Sprintf("UPDATE np_webhooks_deliveries SET %s WHERE id = $1",
		joinStrings(updates, ", "))
	_, err := pool.Exec(ctx, query, args...)
	return err
}

// MarkDeliveryDeadLetter moves a delivery to dead_letter status.
func MarkDeliveryDeadLetter(ctx context.Context, pool *pgxpool.Pool, id string, responseTimeMs *int, errorMessage *string) error {
	return UpdateDeliveryStatus(ctx, pool, id, "dead_letter", nil, nil, responseTimeMs, errorMessage, nil)
}

// --- Helpers -----------------------------------------------------------------

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// MarshalJSONOrDefault marshals a value to JSON, returning defaultVal on error.
func MarshalJSONOrDefault(v interface{}, defaultVal string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return defaultVal
	}
	return string(b)
}
