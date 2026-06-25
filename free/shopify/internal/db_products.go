package internal

import (
	"context"
	"encoding/json"
)

// -------------------------------------------------------------------------
// Products
// -------------------------------------------------------------------------

// UpsertProduct inserts or updates a product and returns its internal UUID.
func (db *DB) UpsertProduct(ctx context.Context, p *Product) (string, error) {
	images := p.Images
	if images == nil {
		images = json.RawMessage("[]")
	}
	options := p.Options
	if options == nil {
		options = json.RawMessage("[]")
	}
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO np_shopify_products (shopify_id, title, body_html, vendor, product_type, handle, status, tags, images, options, published_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		ON CONFLICT (shopify_id, source_account_id) DO UPDATE SET
			title        = EXCLUDED.title,
			body_html    = EXCLUDED.body_html,
			vendor       = EXCLUDED.vendor,
			product_type = EXCLUDED.product_type,
			handle       = EXCLUDED.handle,
			status       = EXCLUDED.status,
			tags         = EXCLUDED.tags,
			images       = EXCLUDED.images,
			options      = EXCLUDED.options,
			published_at = EXCLUDED.published_at,
			updated_at   = NOW(),
			synced_at    = NOW()
		RETURNING id
	`, p.ShopifyID, p.Title, p.BodyHTML, p.Vendor, p.ProductType, p.Handle, p.Status, p.Tags, images, options, p.PublishedAt, db.SourceAccountID).Scan(&id)
	return id, err
}

// GetProduct returns a single product by internal ID.
func (db *DB) GetProduct(ctx context.Context, id string) (*Product, error) {
	var p Product
	err := db.Pool.QueryRow(ctx, `
		SELECT id, shopify_id, title, body_html, vendor, product_type, handle, status, tags, images, options, created_at, updated_at, published_at, source_account_id, synced_at
		FROM np_shopify_products WHERE id = $1
	`, id).Scan(&p.ID, &p.ShopifyID, &p.Title, &p.BodyHTML, &p.Vendor, &p.ProductType, &p.Handle, &p.Status, &p.Tags, &p.Images, &p.Options, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &p.SourceAccountID, &p.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProducts returns products with pagination.
func (db *DB) ListProducts(ctx context.Context, limit, offset int) ([]Product, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, shopify_id, title, body_html, vendor, product_type, handle, status, tags, images, options, created_at, updated_at, published_at, source_account_id, synced_at
		FROM np_shopify_products WHERE source_account_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, db.SourceAccountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.ShopifyID, &p.Title, &p.BodyHTML, &p.Vendor, &p.ProductType, &p.Handle, &p.Status, &p.Tags, &p.Images, &p.Options, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &p.SourceAccountID, &p.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// CountProducts returns the total number of products for this account.
func (db *DB) CountProducts(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_shopify_products WHERE source_account_id = $1`, db.SourceAccountID).Scan(&count)
	return count, err
}

// GetProductVariants returns all variants for a product.
func (db *DB) GetProductVariants(ctx context.Context, productID string) ([]Variant, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, shopify_id, product_id, title, price, compare_at_price, sku, barcode, position, inventory_quantity, inventory_item_id, weight, weight_unit, option1, option2, option3, created_at, updated_at, source_account_id, synced_at
		FROM np_shopify_variants WHERE product_id = $1 AND source_account_id = $2
		ORDER BY position
	`, productID, db.SourceAccountID)
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

// DeleteProductByShopifyID removes a product and its variants by Shopify ID.
func (db *DB) DeleteProductByShopifyID(ctx context.Context, shopifyID int64) error {
	var productID string
	err := db.Pool.QueryRow(ctx, `SELECT id FROM np_shopify_products WHERE shopify_id = $1 AND source_account_id = $2`, shopifyID, db.SourceAccountID).Scan(&productID)
	if err != nil {
		return err
	}
	_, err = db.Pool.Exec(ctx, `DELETE FROM np_shopify_variants WHERE product_id = $1 AND source_account_id = $2`, productID, db.SourceAccountID)
	if err != nil {
		return err
	}
	_, err = db.Pool.Exec(ctx, `DELETE FROM np_shopify_products WHERE shopify_id = $1 AND source_account_id = $2`, shopifyID, db.SourceAccountID)
	return err
}

