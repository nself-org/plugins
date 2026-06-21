package internal

import (
	"context"
)

// Webhook event storage
// ============================================================================

// InsertWebhookEvent stores a raw webhook event in np_stripe_webhook_events.
func (db *DB) InsertWebhookEvent(ctx context.Context, event *StripeWebhookEvent) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_stripe_webhook_events (
			id, type, api_version, data, object_type, object_id,
			request_id, request_idempotency_key, livemode, pending_webhooks,
			processed, processed_at, error, retry_count,
			source_account_id, created_at, received_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (id) DO NOTHING`,
		event.ID, event.Type, nullStr(event.APIVersion), event.Data,
		nullStr(event.ObjectType), nullStr(event.ObjectID),
		nullStr(event.RequestID), nullStr(event.RequestIdempotencyKey),
		event.Livemode, event.PendingWebhooks,
		event.Processed, nullTime(event.ProcessedAt), nullStr(event.Error),
		event.RetryCount, db.SourceAccountID,
		nullTime(event.CreatedAt), nullTime(event.ReceivedAt),
	)
	return err
}

// MarkEventProcessed marks a webhook event as processed, optionally with an error.
func (db *DB) MarkEventProcessed(ctx context.Context, eventID string, errMsg string) error {
	if errMsg != "" {
		_, err := db.Pool.Exec(ctx,
			"UPDATE np_stripe_webhook_events SET processed = true, processed_at = NOW(), error = $2 WHERE id = $1",
			eventID, errMsg,
		)
		return err
	}
	_, err := db.Pool.Exec(ctx,
		"UPDATE np_stripe_webhook_events SET processed = true, processed_at = NOW() WHERE id = $1",
		eventID,
	)
	return err
}

