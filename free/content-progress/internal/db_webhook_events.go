package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
	"fmt"
	"time"
)

// =========================================================================
// Webhook Events
// =========================================================================

// ListWebhookEvents returns webhook events, optionally filtered by event type.
func (d *DB) ListWebhookEvents(eventType string, limit, offset int) ([]WebhookEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rows pgx.Rows
	var err error

	if eventType != "" {
		rows, err = d.pool.Query(ctx,
			`SELECT id, source_account_id, event_type, payload, processed, processed_at, error, created_at
			FROM np_progress_webhook_events
			WHERE source_account_id = $1 AND event_type = $2
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4`,
			d.sourceAccountID, eventType, limit, offset,
		)
	} else {
		rows, err = d.pool.Query(ctx,
			`SELECT id, source_account_id, event_type, payload, processed, processed_at, error, created_at
			FROM np_progress_webhook_events
			WHERE source_account_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`,
			d.sourceAccountID, limit, offset,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list webhook events: %w", err)
	}
	defer rows.Close()

	var items []WebhookEvent
	for rows.Next() {
		var evt WebhookEvent
		if err := rows.Scan(
			&evt.ID, &evt.SourceAccountID, &evt.EventType, &evt.Payload,
			&evt.Processed, &evt.ProcessedAt, &evt.Error, &evt.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan webhook event: %w", err)
		}
		items = append(items, evt)
	}
	if items == nil {
		items = []WebhookEvent{}
	}
	return items, rows.Err()
}

