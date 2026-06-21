package internal

import (
	"context"
)

// -------------------------------------------------------------------------
// Variants
// -------------------------------------------------------------------------

// UpsertVariant inserts or updates a variant record.
func (db *DB) UpsertVariant(ctx context.Context, v *Variant) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_shopify_variants (shopify_id, product_id, title, price, compare_at_price, sku, barcode, position, inventory_quantity, inventory_item_id, weight, weight_unit, option1, option2, option3, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW())
		ON CONFLICT (shopify_id, source_account_id) DO UPDATE SET
			product_id         = EXCLUDED.product_id,
			title              = EXCLUDED.title,
			price              = EXCLUDED.price,
			compare_at_price   = EXCLUDED.compare_at_price,
			sku                = EXCLUDED.sku,
			barcode            = EXCLUDED.barcode,
			position           = EXCLUDED.position,
			inventory_quantity = EXCLUDED.inventory_quantity,
			inventory_item_id  = EXCLUDED.inventory_item_id,
			weight             = EXCLUDED.weight,
			weight_unit        = EXCLUDED.weight_unit,
			option1            = EXCLUDED.option1,
			option2            = EXCLUDED.option2,
			option3            = EXCLUDED.option3,
			updated_at         = NOW(),
			synced_at          = NOW()
	`, v.ShopifyID, v.ProductID, v.Title, v.Price, v.CompareAtPrice, v.SKU, v.Barcode, v.Position, v.InventoryQuantity, v.InventoryItemID, v.Weight, v.WeightUnit, v.Option1, v.Option2, v.Option3, db.SourceAccountID)
	return err
}

// ListVariants returns variants with pagination.
func (db *DB) ListVariants(ctx context.Context, limit, offset int) ([]Variant, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, shopify_id, product_id, title, price, compare_at_price, sku, barcode, position, inventory_quantity, inventory_item_id, weight, weight_unit, option1, option2, option3, created_at, updated_at, source_account_id, synced_at
		FROM np_shopify_variants WHERE source_account_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, db.SourceAccountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Variant
	for rows.Next() {
		var v Variant
		if err := rows.Scan(&v.ID, &v.ShopifyID, &v.ProductID, &v.Title, &v.Price, &v.CompareAtPrice, &v.SKU, &v.Barcode, &v.Position, &v.InventoryQuantity, &v.InventoryItemID, &v.Weight, &v.WeightUnit, &v.Option1, &v.Option2, &v.Option3, &v.CreatedAt, &v.UpdatedAt, &v.SourceAccountID, &v.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, v)
	}
	return results, rows.Err()
}

// CountVariants returns the total number of variants for this account.
func (db *DB) CountVariants(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_shopify_variants WHERE source_account_id = $1`, db.SourceAccountID).Scan(&count)
	return count, err
}

