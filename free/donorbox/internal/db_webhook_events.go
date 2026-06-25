package internal

import (
	"context"
	"encoding/json"
)

// --- Webhook event storage ---------------------------------------------------

// InsertWebhookEvent stores a raw webhook event.
func (db *DB) InsertWebhookEvent(ctx context.Context, id, eventType string, payload json.RawMessage) error {
	if payload == nil {
		payload = json.RawMessage("{}")
	}
	_, err := db.pool.Exec(ctx, `
		INSERT INTO np_donorbox_webhook_events (id, event_type, payload, source_account_id, created_at, synced_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, id, eventType, payload, db.sourceAccountID)
	return err
}

// MarkEventProcessed marks a webhook event as processed, optionally with an error.
func (db *DB) MarkEventProcessed(ctx context.Context, eventID string, errMsg *string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE np_donorbox_webhook_events SET processed = true, processed_at = NOW(), error = $2
		WHERE id = $1 AND source_account_id = $3
	`, eventID, errMsg, db.sourceAccountID)
	return err
}

// --- Statistics --------------------------------------------------------------
