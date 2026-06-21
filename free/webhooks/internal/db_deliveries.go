package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
