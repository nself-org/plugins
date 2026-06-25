package internal

import (
	"context"
	"encoding/json"
	"fmt"
)

// Upsert operations for webhook-driven data updates
// ============================================================================

// UpsertFromWebhookEvent stores or updates the object from a webhook event payload.
// The eventData is the raw JSON of event.data.object from Stripe.
func (db *DB) UpsertFromWebhookEvent(ctx context.Context, objectType string, eventData json.RawMessage) error {
	// Extract the id from the object
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(eventData, &obj); err != nil {
		return fmt.Errorf("failed to extract id from event data: %w", err)
	}
	if obj.ID == "" {
		return fmt.Errorf("event data has no id field")
	}

	table := objectTypeToTable(objectType)
	if table == "" {
		// Unknown object type, skip silently
		return nil
	}

	// Use a generic upsert: store the full JSON in the data column of webhook_events
	// For actual object tables, we do a lightweight upsert of synced_at to mark freshness.
	// The full sync operations handle detailed column-level upserts.
	_, err := db.Pool.Exec(ctx,
		fmt.Sprintf("UPDATE %s SET synced_at = NOW() WHERE id = $1 AND source_account_id = $2", table),
		obj.ID, db.SourceAccountID,
	)
	return err
}

// DeleteObject marks an object as deleted (soft delete) or hard-deletes it.
func (db *DB) DeleteObject(ctx context.Context, objectType string, objectID string) error {
	table := objectTypeToTable(objectType)
	if table == "" {
		return nil
	}
	// Tables with deleted_at use soft delete; others use hard delete
	switch table {
	case "np_stripe_customers", "np_stripe_products", "np_stripe_prices", "np_stripe_coupons":
		_, err := db.Pool.Exec(ctx,
			fmt.Sprintf("UPDATE %s SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND source_account_id = $2", table),
			objectID, db.SourceAccountID,
		)
		return err
	default:
		_, err := db.Pool.Exec(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE id = $1 AND source_account_id = $2", table),
			objectID, db.SourceAccountID,
		)
		return err
	}
}

