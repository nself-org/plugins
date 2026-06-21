package internal

import (
	"context"
)

// -------------------------------------------------------------------------
// Inventory
// -------------------------------------------------------------------------

// UpsertInventoryLevel inserts or updates an inventory level record.
func (db *DB) UpsertInventoryLevel(ctx context.Context, inventoryItemID, locationID int64, available int) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_shopify_inventory (inventory_item_id, location_id, available, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (inventory_item_id, location_id, source_account_id) DO UPDATE SET
			available  = EXCLUDED.available,
			updated_at = NOW(),
			synced_at  = NOW()
	`, inventoryItemID, locationID, available, db.SourceAccountID)
	return err
}

// ListInventory returns inventory levels with pagination.
func (db *DB) ListInventory(ctx context.Context, limit, offset int) ([]InventoryLevel, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, inventory_item_id, location_id, available, updated_at, source_account_id, synced_at
		FROM np_shopify_inventory WHERE source_account_id = $1
		ORDER BY updated_at DESC LIMIT $2 OFFSET $3
	`, db.SourceAccountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []InventoryLevel
	for rows.Next() {
		var inv InventoryLevel
		if err := rows.Scan(&inv.ID, &inv.InventoryItemID, &inv.LocationID, &inv.Available, &inv.UpdatedAt, &inv.SourceAccountID, &inv.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, inv)
	}
	return results, rows.Err()
}

// CountInventory returns the total number of inventory levels for this account.
func (db *DB) CountInventory(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_shopify_inventory WHERE source_account_id = $1`, db.SourceAccountID).Scan(&count)
	return count, err
}

// DeleteInventoryLevel removes an inventory level record.
func (db *DB) DeleteInventoryLevel(ctx context.Context, inventoryItemID, locationID int64) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM np_shopify_inventory WHERE inventory_item_id = $1 AND location_id = $2 AND source_account_id = $3`, inventoryItemID, locationID, db.SourceAccountID)
	return err
}

