package internal

import (
	"context"
	"encoding/json"
	"fmt"
)

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

