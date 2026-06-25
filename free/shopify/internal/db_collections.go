package internal

import (
	"context"
)

// -------------------------------------------------------------------------
// Collections
// -------------------------------------------------------------------------

// UpsertCollection inserts or updates a collection record.
func (db *DB) UpsertCollection(ctx context.Context, c *Collection) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_shopify_collections (shopify_id, title, body_html, handle, sort_order, collection_type, image, published_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (shopify_id, source_account_id) DO UPDATE SET
			title           = EXCLUDED.title,
			body_html       = EXCLUDED.body_html,
			handle          = EXCLUDED.handle,
			sort_order      = EXCLUDED.sort_order,
			collection_type = EXCLUDED.collection_type,
			image           = EXCLUDED.image,
			published_at    = EXCLUDED.published_at,
			updated_at      = NOW(),
			synced_at       = NOW()
	`, c.ShopifyID, c.Title, c.BodyHTML, c.Handle, c.SortOrder, c.CollectionType, c.Image, c.PublishedAt, db.SourceAccountID)
	return err
}

// ListCollections returns collections with pagination.
func (db *DB) ListCollections(ctx context.Context, limit, offset int) ([]Collection, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, shopify_id, title, body_html, handle, sort_order, collection_type, image, published_at, created_at, updated_at, source_account_id, synced_at
		FROM np_shopify_collections WHERE source_account_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, db.SourceAccountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.ShopifyID, &c.Title, &c.BodyHTML, &c.Handle, &c.SortOrder, &c.CollectionType, &c.Image, &c.PublishedAt, &c.CreatedAt, &c.UpdatedAt, &c.SourceAccountID, &c.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

// CountCollections returns the total number of collections for this account.
func (db *DB) CountCollections(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_shopify_collections WHERE source_account_id = $1`, db.SourceAccountID).Scan(&count)
	return count, err
}

// DeleteCollectionByShopifyID removes a collection by Shopify ID.
func (db *DB) DeleteCollectionByShopifyID(ctx context.Context, shopifyID int64) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM np_shopify_collections WHERE shopify_id = $1 AND source_account_id = $2`, shopifyID, db.SourceAccountID)
	return err
}

