package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with a source account identifier.
// All queries are scoped to the SourceAccountID where applicable.
type DB struct {
	Pool            *pgxpool.Pool
	SourceAccountID string
}

// NewDB creates a new DB instance.
func NewDB(pool *pgxpool.Pool, sourceAccountID string) *DB {
	return &DB{
		Pool:            pool,
		SourceAccountID: sourceAccountID,
	}
}

// Migrate creates all required tables and indexes if they do not exist.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_shopify_shops (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id        BIGINT NOT NULL,
			name              TEXT NOT NULL DEFAULT '',
			email             TEXT,
			domain            TEXT,
			myshopify_domain  TEXT NOT NULL DEFAULT '',
			country           TEXT,
			currency          TEXT NOT NULL DEFAULT 'USD',
			timezone          TEXT,
			plan_name         TEXT,
			plan_display_name TEXT,
			money_format      TEXT,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_shops_shopify_id_account
			ON np_shopify_shops (shopify_id, source_account_id);

		CREATE TABLE IF NOT EXISTS np_shopify_products (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id        BIGINT NOT NULL,
			title             TEXT NOT NULL DEFAULT '',
			body_html         TEXT,
			vendor            TEXT,
			product_type      TEXT,
			handle            TEXT,
			status            TEXT NOT NULL DEFAULT 'active',
			tags              TEXT,
			images            JSONB NOT NULL DEFAULT '[]',
			options           JSONB NOT NULL DEFAULT '[]',
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			published_at      TIMESTAMPTZ,
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_products_shopify_id_account
			ON np_shopify_products (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_products_status
			ON np_shopify_products (status);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_products_vendor
			ON np_shopify_products (vendor);

		CREATE TABLE IF NOT EXISTS np_shopify_variants (
			id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id         BIGINT NOT NULL,
			product_id         TEXT NOT NULL,
			title              TEXT,
			price              TEXT,
			compare_at_price   TEXT,
			sku                TEXT,
			barcode            TEXT,
			position           INT NOT NULL DEFAULT 1,
			inventory_quantity INT NOT NULL DEFAULT 0,
			inventory_item_id  BIGINT,
			weight             DOUBLE PRECISION,
			weight_unit        TEXT,
			option1            TEXT,
			option2            TEXT,
			option3            TEXT,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id  TEXT NOT NULL DEFAULT 'primary',
			synced_at          TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_variants_shopify_id_account
			ON np_shopify_variants (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_variants_product_id
			ON np_shopify_variants (product_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_variants_sku
			ON np_shopify_variants (sku);

		CREATE TABLE IF NOT EXISTS np_shopify_collections (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id        BIGINT NOT NULL,
			title             TEXT NOT NULL DEFAULT '',
			body_html         TEXT,
			handle            TEXT,
			sort_order        TEXT,
			collection_type   TEXT,
			image             JSONB,
			published_at      TIMESTAMPTZ,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_collections_shopify_id_account
			ON np_shopify_collections (shopify_id, source_account_id);

		CREATE TABLE IF NOT EXISTS np_shopify_customers (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id        BIGINT NOT NULL,
			email             TEXT,
			first_name        TEXT,
			last_name         TEXT,
			phone             TEXT,
			orders_count      INT NOT NULL DEFAULT 0,
			total_spent       TEXT,
			currency          TEXT,
			tags              TEXT,
			addresses         JSONB NOT NULL DEFAULT '[]',
			default_address   JSONB,
			accepts_marketing BOOLEAN NOT NULL DEFAULT false,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_customers_shopify_id_account
			ON np_shopify_customers (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_customers_email
			ON np_shopify_customers (email);

		CREATE TABLE IF NOT EXISTS np_shopify_orders (
			id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id         BIGINT NOT NULL,
			name               TEXT NOT NULL DEFAULT '',
			email              TEXT,
			total_price        TEXT,
			subtotal_price     TEXT,
			total_tax          TEXT,
			total_discounts    TEXT,
			currency           TEXT NOT NULL DEFAULT 'USD',
			financial_status   TEXT,
			fulfillment_status TEXT,
			customer_id        BIGINT,
			line_items         JSONB NOT NULL DEFAULT '[]',
			shipping_address   JSONB,
			billing_address    JSONB,
			note               TEXT,
			tags               TEXT,
			gateway            TEXT,
			confirmed          BOOLEAN NOT NULL DEFAULT false,
			cancelled_at       TIMESTAMPTZ,
			cancel_reason      TEXT,
			closed_at          TIMESTAMPTZ,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			processed_at       TIMESTAMPTZ,
			source_account_id  TEXT NOT NULL DEFAULT 'primary',
			synced_at          TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_orders_shopify_id_account
			ON np_shopify_orders (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_orders_financial_status
			ON np_shopify_orders (financial_status);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_orders_fulfillment_status
			ON np_shopify_orders (fulfillment_status);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_orders_customer_id
			ON np_shopify_orders (customer_id);

		CREATE TABLE IF NOT EXISTS np_shopify_order_items (
			id                 TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_id         BIGINT NOT NULL,
			order_id           TEXT NOT NULL,
			product_id         BIGINT,
			variant_id         BIGINT,
			title              TEXT NOT NULL DEFAULT '',
			quantity           INT NOT NULL DEFAULT 0,
			price              TEXT,
			sku                TEXT,
			vendor             TEXT,
			fulfillment_status TEXT,
			source_account_id  TEXT NOT NULL DEFAULT 'primary',
			synced_at          TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_order_items_shopify_id_account
			ON np_shopify_order_items (shopify_id, source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_order_items_order_id
			ON np_shopify_order_items (order_id);

		CREATE TABLE IF NOT EXISTS np_shopify_inventory (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			inventory_item_id BIGINT NOT NULL,
			location_id       BIGINT NOT NULL,
			available         INT NOT NULL DEFAULT 0,
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary',
			synced_at         TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_np_shopify_inventory_item_location_account
			ON np_shopify_inventory (inventory_item_id, location_id, source_account_id);

		CREATE TABLE IF NOT EXISTS np_shopify_webhook_events (
			id                TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			shopify_event_id  TEXT,
			topic             TEXT NOT NULL,
			shop_domain       TEXT,
			body              JSONB NOT NULL DEFAULT '{}',
			processed         BOOLEAN NOT NULL DEFAULT false,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_account_id TEXT NOT NULL DEFAULT 'primary'
		);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_webhook_events_topic
			ON np_shopify_webhook_events (topic);
		CREATE INDEX IF NOT EXISTS idx_np_shopify_webhook_events_processed
			ON np_shopify_webhook_events (processed);
	`)
	return err
}

// -------------------------------------------------------------------------
// Shop
// -------------------------------------------------------------------------

// UpsertShop inserts or updates a shop record.
func (db *DB) UpsertShop(ctx context.Context, s *shopifyShop) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_shopify_shops (shopify_id, name, email, domain, myshopify_domain, country, currency, timezone, plan_name, plan_display_name, money_format, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		ON CONFLICT (shopify_id, source_account_id) DO UPDATE SET
			name              = EXCLUDED.name,
			email             = EXCLUDED.email,
			domain            = EXCLUDED.domain,
			myshopify_domain  = EXCLUDED.myshopify_domain,
			country           = EXCLUDED.country,
			currency          = EXCLUDED.currency,
			timezone          = EXCLUDED.timezone,
			plan_name         = EXCLUDED.plan_name,
			plan_display_name = EXCLUDED.plan_display_name,
			money_format      = EXCLUDED.money_format,
			updated_at        = NOW(),
			synced_at         = NOW()
	`, s.ID, s.Name, s.Email, s.Domain, s.MyshopifyDomain, s.Country, s.Currency, s.Timezone, s.PlanName, s.PlanDisplayName, s.MoneyFormat, db.SourceAccountID)
	return err
}

// GetShop returns the first shop for this account.
func (db *DB) GetShop(ctx context.Context) (*Shop, error) {
	var s Shop
	err := db.Pool.QueryRow(ctx, `
		SELECT id, shopify_id, name, email, domain, myshopify_domain, country, currency, timezone, plan_name, plan_display_name, money_format, created_at, updated_at, source_account_id, synced_at
		FROM np_shopify_shops WHERE source_account_id = $1 ORDER BY created_at LIMIT 1
	`, db.SourceAccountID).Scan(&s.ID, &s.ShopifyID, &s.Name, &s.Email, &s.Domain, &s.MyshopifyDomain, &s.Country, &s.Currency, &s.Timezone, &s.PlanName, &s.PlanDisplayName, &s.MoneyFormat, &s.CreatedAt, &s.UpdatedAt, &s.SourceAccountID, &s.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

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

// -------------------------------------------------------------------------
// Customers
// -------------------------------------------------------------------------

// UpsertCustomer inserts or updates a customer record.
func (db *DB) UpsertCustomer(ctx context.Context, c *Customer) error {
	addresses := c.Addresses
	if addresses == nil {
		addresses = json.RawMessage("[]")
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_shopify_customers (shopify_id, email, first_name, last_name, phone, orders_count, total_spent, currency, tags, addresses, default_address, accepts_marketing, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
		ON CONFLICT (shopify_id, source_account_id) DO UPDATE SET
			email             = EXCLUDED.email,
			first_name        = EXCLUDED.first_name,
			last_name         = EXCLUDED.last_name,
			phone             = EXCLUDED.phone,
			orders_count      = EXCLUDED.orders_count,
			total_spent       = EXCLUDED.total_spent,
			currency          = EXCLUDED.currency,
			tags              = EXCLUDED.tags,
			addresses         = EXCLUDED.addresses,
			default_address   = EXCLUDED.default_address,
			accepts_marketing = EXCLUDED.accepts_marketing,
			updated_at        = NOW(),
			synced_at         = NOW()
	`, c.ShopifyID, c.Email, c.FirstName, c.LastName, c.Phone, c.OrdersCount, c.TotalSpent, c.Currency, c.Tags, addresses, c.DefaultAddress, c.AcceptsMarketing, db.SourceAccountID)
	return err
}

// GetCustomer returns a single customer by internal ID.
func (db *DB) GetCustomer(ctx context.Context, id string) (*Customer, error) {
	var c Customer
	err := db.Pool.QueryRow(ctx, `
		SELECT id, shopify_id, email, first_name, last_name, phone, orders_count, total_spent, currency, tags, addresses, default_address, accepts_marketing, created_at, updated_at, source_account_id, synced_at
		FROM np_shopify_customers WHERE id = $1
	`, id).Scan(&c.ID, &c.ShopifyID, &c.Email, &c.FirstName, &c.LastName, &c.Phone, &c.OrdersCount, &c.TotalSpent, &c.Currency, &c.Tags, &c.Addresses, &c.DefaultAddress, &c.AcceptsMarketing, &c.CreatedAt, &c.UpdatedAt, &c.SourceAccountID, &c.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCustomers returns customers with pagination.
func (db *DB) ListCustomers(ctx context.Context, limit, offset int) ([]Customer, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, shopify_id, email, first_name, last_name, phone, orders_count, total_spent, currency, tags, addresses, default_address, accepts_marketing, created_at, updated_at, source_account_id, synced_at
		FROM np_shopify_customers WHERE source_account_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, db.SourceAccountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Customer
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.ShopifyID, &c.Email, &c.FirstName, &c.LastName, &c.Phone, &c.OrdersCount, &c.TotalSpent, &c.Currency, &c.Tags, &c.Addresses, &c.DefaultAddress, &c.AcceptsMarketing, &c.CreatedAt, &c.UpdatedAt, &c.SourceAccountID, &c.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

// CountCustomers returns the total number of customers for this account.
func (db *DB) CountCustomers(ctx context.Context) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_shopify_customers WHERE source_account_id = $1`, db.SourceAccountID).Scan(&count)
	return count, err
}

// DeleteCustomerByShopifyID removes a customer by Shopify ID.
func (db *DB) DeleteCustomerByShopifyID(ctx context.Context, shopifyID int64) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM np_shopify_customers WHERE shopify_id = $1 AND source_account_id = $2`, shopifyID, db.SourceAccountID)
	return err
}

// -------------------------------------------------------------------------
// Orders
// -------------------------------------------------------------------------

// UpsertOrder inserts or updates an order and returns its internal UUID.
func (db *DB) UpsertOrder(ctx context.Context, o *Order) (string, error) {
	lineItems := o.LineItems
	if lineItems == nil {
		lineItems = json.RawMessage("[]")
	}
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO np_shopify_orders (shopify_id, name, email, total_price, subtotal_price, total_tax, total_discounts, currency, financial_status, fulfillment_status, customer_id, line_items, shipping_address, billing_address, note, tags, gateway, confirmed, cancelled_at, cancel_reason, closed_at, processed_at, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, NOW())
		ON CONFLICT (shopify_id, source_account_id) DO UPDATE SET
			name               = EXCLUDED.name,
			email              = EXCLUDED.email,
			total_price        = EXCLUDED.total_price,
			subtotal_price     = EXCLUDED.subtotal_price,
			total_tax          = EXCLUDED.total_tax,
			total_discounts    = EXCLUDED.total_discounts,
			currency           = EXCLUDED.currency,
			financial_status   = EXCLUDED.financial_status,
			fulfillment_status = EXCLUDED.fulfillment_status,
			customer_id        = EXCLUDED.customer_id,
			line_items         = EXCLUDED.line_items,
			shipping_address   = EXCLUDED.shipping_address,
			billing_address    = EXCLUDED.billing_address,
			note               = EXCLUDED.note,
			tags               = EXCLUDED.tags,
			gateway            = EXCLUDED.gateway,
			confirmed          = EXCLUDED.confirmed,
			cancelled_at       = EXCLUDED.cancelled_at,
			cancel_reason      = EXCLUDED.cancel_reason,
			closed_at          = EXCLUDED.closed_at,
			processed_at       = EXCLUDED.processed_at,
			updated_at         = NOW(),
			synced_at          = NOW()
		RETURNING id
	`, o.ShopifyID, o.Name, o.Email, o.TotalPrice, o.SubtotalPrice, o.TotalTax, o.TotalDiscounts, o.Currency, o.FinancialStatus, o.FulfillmentStatus, o.CustomerID, lineItems, o.ShippingAddress, o.BillingAddress, o.Note, o.Tags, o.Gateway, o.Confirmed, o.CancelledAt, o.CancelReason, o.ClosedAt, o.ProcessedAt, db.SourceAccountID).Scan(&id)
	return id, err
}

// GetOrder returns a single order by internal ID.
func (db *DB) GetOrder(ctx context.Context, id string) (*Order, error) {
	var o Order
	err := db.Pool.QueryRow(ctx, `
		SELECT id, shopify_id, name, email, total_price, subtotal_price, total_tax, total_discounts, currency, financial_status, fulfillment_status, customer_id, line_items, shipping_address, billing_address, note, tags, gateway, confirmed, cancelled_at, cancel_reason, closed_at, created_at, updated_at, processed_at, source_account_id, synced_at
		FROM np_shopify_orders WHERE id = $1
	`, id).Scan(&o.ID, &o.ShopifyID, &o.Name, &o.Email, &o.TotalPrice, &o.SubtotalPrice, &o.TotalTax, &o.TotalDiscounts, &o.Currency, &o.FinancialStatus, &o.FulfillmentStatus, &o.CustomerID, &o.LineItems, &o.ShippingAddress, &o.BillingAddress, &o.Note, &o.Tags, &o.Gateway, &o.Confirmed, &o.CancelledAt, &o.CancelReason, &o.ClosedAt, &o.CreatedAt, &o.UpdatedAt, &o.ProcessedAt, &o.SourceAccountID, &o.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrders returns orders with optional status filter and pagination.
func (db *DB) ListOrders(ctx context.Context, status string, limit, offset int) ([]Order, error) {
	query := `SELECT id, shopify_id, name, email, total_price, subtotal_price, total_tax, total_discounts, currency, financial_status, fulfillment_status, customer_id, line_items, shipping_address, billing_address, note, tags, gateway, confirmed, cancelled_at, cancel_reason, closed_at, created_at, updated_at, processed_at, source_account_id, synced_at
		FROM np_shopify_orders WHERE source_account_id = $1`
	args := []interface{}{db.SourceAccountID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(" AND financial_status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.ShopifyID, &o.Name, &o.Email, &o.TotalPrice, &o.SubtotalPrice, &o.TotalTax, &o.TotalDiscounts, &o.Currency, &o.FinancialStatus, &o.FulfillmentStatus, &o.CustomerID, &o.LineItems, &o.ShippingAddress, &o.BillingAddress, &o.Note, &o.Tags, &o.Gateway, &o.Confirmed, &o.CancelledAt, &o.CancelReason, &o.ClosedAt, &o.CreatedAt, &o.UpdatedAt, &o.ProcessedAt, &o.SourceAccountID, &o.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, o)
	}
	return results, rows.Err()
}

// CountOrders returns the total number of orders, optionally filtered by status.
func (db *DB) CountOrders(ctx context.Context, status string) (int, error) {
	var count int
	if status != "" {
		err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_shopify_orders WHERE source_account_id = $1 AND financial_status = $2`, db.SourceAccountID, status).Scan(&count)
		return count, err
	}
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_shopify_orders WHERE source_account_id = $1`, db.SourceAccountID).Scan(&count)
	return count, err
}

// GetOrderItems returns all line items for an order.
func (db *DB) GetOrderItems(ctx context.Context, orderID string) ([]OrderItem, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, shopify_id, order_id, product_id, variant_id, title, quantity, price, sku, vendor, fulfillment_status, source_account_id, synced_at
		FROM np_shopify_order_items WHERE order_id = $1 AND source_account_id = $2
	`, orderID, db.SourceAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []OrderItem
	for rows.Next() {
		var oi OrderItem
		if err := rows.Scan(&oi.ID, &oi.ShopifyID, &oi.OrderID, &oi.ProductID, &oi.VariantID, &oi.Title, &oi.Quantity, &oi.Price, &oi.SKU, &oi.Vendor, &oi.FulfillmentStatus, &oi.SourceAccountID, &oi.SyncedAt); err != nil {
			return nil, err
		}
		results = append(results, oi)
	}
	return results, rows.Err()
}

// UpsertOrderItem inserts or updates an order item record.
func (db *DB) UpsertOrderItem(ctx context.Context, oi *OrderItem) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_shopify_order_items (shopify_id, order_id, product_id, variant_id, title, quantity, price, sku, vendor, fulfillment_status, source_account_id, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (shopify_id, source_account_id) DO UPDATE SET
			order_id           = EXCLUDED.order_id,
			product_id         = EXCLUDED.product_id,
			variant_id         = EXCLUDED.variant_id,
			title              = EXCLUDED.title,
			quantity           = EXCLUDED.quantity,
			price              = EXCLUDED.price,
			sku                = EXCLUDED.sku,
			vendor             = EXCLUDED.vendor,
			fulfillment_status = EXCLUDED.fulfillment_status,
			synced_at          = NOW()
	`, oi.ShopifyID, oi.OrderID, oi.ProductID, oi.VariantID, oi.Title, oi.Quantity, oi.Price, oi.SKU, oi.Vendor, oi.FulfillmentStatus, db.SourceAccountID)
	return err
}

// DeleteOrderByShopifyID removes an order and its items by Shopify ID.
func (db *DB) DeleteOrderByShopifyID(ctx context.Context, shopifyID int64) error {
	var orderID string
	err := db.Pool.QueryRow(ctx, `SELECT id FROM np_shopify_orders WHERE shopify_id = $1 AND source_account_id = $2`, shopifyID, db.SourceAccountID).Scan(&orderID)
	if err != nil {
		return err
	}
	_, err = db.Pool.Exec(ctx, `DELETE FROM np_shopify_order_items WHERE order_id = $1 AND source_account_id = $2`, orderID, db.SourceAccountID)
	if err != nil {
		return err
	}
	_, err = db.Pool.Exec(ctx, `DELETE FROM np_shopify_orders WHERE shopify_id = $1 AND source_account_id = $2`, shopifyID, db.SourceAccountID)
	return err
}

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
