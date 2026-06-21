package internal

import (
	"context"
	"encoding/json"
)

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

