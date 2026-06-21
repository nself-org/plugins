package internal

import (
	"context"
	"encoding/json"
	"fmt"
)

// -------------------------------------------------------------------------
// Webhook Events
// -------------------------------------------------------------------------

// InsertWebhookEvent inserts a webhook event record.
func (db *DB) InsertWebhookEvent(ctx context.Context, shopifyEventID, topic, shopDomain string, body json.RawMessage) error {
	if body == nil {
		body = json.RawMessage("{}")
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_shopify_webhook_events (shopify_event_id, topic, shop_domain, body, processed, source_account_id)
		VALUES ($1, $2, $3, $4, false, $5)
	`, shopifyEventID, topic, shopDomain, body, db.SourceAccountID)
	return err
}

// ListWebhookEvents returns webhook events with optional topic filter and a limit.
func (db *DB) ListWebhookEvents(ctx context.Context, topic string, limit int) ([]WebhookEvent, error) {
	query := `SELECT id, shopify_event_id, topic, shop_domain, body, processed, created_at, source_account_id
		FROM np_shopify_webhook_events WHERE source_account_id = $1`
	args := []interface{}{db.SourceAccountID}
	argIdx := 2

	if topic != "" {
		query += fmt.Sprintf(" AND topic = $%d", argIdx)
		args = append(args, topic)
		argIdx++
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []WebhookEvent
	for rows.Next() {
		var evt WebhookEvent
		if err := rows.Scan(&evt.ID, &evt.ShopifyEventID, &evt.Topic, &evt.ShopDomain, &evt.Body, &evt.Processed, &evt.CreatedAt, &evt.SourceAccountID); err != nil {
			return nil, err
		}
		results = append(results, evt)
	}
	return results, rows.Err()
}

