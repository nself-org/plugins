package internal

import (
	"context"
)

// -------------------------------------------------------------------------
// Stats
// -------------------------------------------------------------------------

// GetStats returns aggregate counts across all shopify tables.
func (db *DB) GetStats(ctx context.Context) (*SyncStats, error) {
	var stats SyncStats
	err := db.Pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM np_shopify_shops),
			(SELECT COUNT(*) FROM np_shopify_products),
			(SELECT COUNT(*) FROM np_shopify_variants),
			(SELECT COUNT(*) FROM np_shopify_collections),
			(SELECT COUNT(*) FROM np_shopify_customers),
			(SELECT COUNT(*) FROM np_shopify_orders),
			(SELECT COUNT(*) FROM np_shopify_order_items),
			(SELECT COUNT(*) FROM np_shopify_inventory),
			(SELECT COUNT(*) FROM np_shopify_webhook_events),
			(SELECT MAX(synced_at) FROM np_shopify_products)
	`).Scan(&stats.Shops, &stats.Products, &stats.Variants, &stats.Collections, &stats.Customers, &stats.Orders, &stats.OrderItems, &stats.Inventory, &stats.Events, &stats.LastSynced)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

