# Shopify Plugin

Complete Shopify e-commerce integration that syncs your store's products, orders, customers, inventory, and more to PostgreSQL with real-time webhook support and analytics views.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Views](#views)
- [Webhooks](#webhooks)
- [Features](#features)
- [Troubleshooting](#troubleshooting)

---

## Overview

| Field | Value |
|-------|-------|
| **Version** | 1.0.0 |
| **Category** | commerce |
| **Port** | 3003 |
| **License** | Source-Available |
| **Min nself Version** | 0.4.8 |
| **Multi-App** | Yes (`source_account_id`, composite PKs) |

The Shopify plugin syncs your entire Shopify store to PostgreSQL: shops, products, variants, collections, customers, orders, order items, fulfillments, transactions, refunds, draft orders, inventory, price rules, discount codes, gift cards, metafields, and checkouts. It uses composite primary keys `(id, source_account_id)` for multi-app isolation and provides real-time updates via HMAC-SHA256-verified webhooks.

---

## Quick Start

```bash
nself plugin install shopify
export SHOPIFY_ACCESS_TOKEN="shpat_..."
export SHOPIFY_SHOP_DOMAIN="mystore.myshopify.com"
nself plugin shopify init
nself plugin shopify sync
nself plugin shopify server
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `SHOPIFY_ACCESS_TOKEN` | Shopify Admin API access token |
| `SHOPIFY_SHOP_DOMAIN` | Shopify store domain (e.g., `mystore.myshopify.com`) |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SHOPIFY_API_VERSION` | `2024-01` | Shopify API version |
| `SHOPIFY_WEBHOOK_SECRET` | - | Webhook HMAC-SHA256 secret for signature verification |
| `SHOPIFY_SYNC_INTERVAL` | `3600` | Sync interval in seconds |
| `SHOPIFY_ACCESS_TOKENS` | - | Comma-separated tokens for multi-store |
| `SHOPIFY_SHOP_DOMAINS` | - | Comma-separated domains for multi-store |
| `SHOPIFY_ACCOUNT_LABELS` | - | Comma-separated labels for multi-store accounts |
| `SHOPIFY_WEBHOOK_SECRETS` | - | Comma-separated webhook secrets for multi-store |

---

## CLI Commands

| Command | Description | Options |
|---------|-------------|---------|
| `init` | Initialize database schema with all 20 tables | - |
| `server` | Start the webhook and API server | `-p, --port <port>`, `-h, --host <host>` |
| `sync` | Sync Shopify data to database | `-r, --resources <resources>` (comma-separated: shop, products, collections, customers, orders, inventory) |
| `status` | Show sync status and record counts | - |
| `products` | List products | `-l, --limit <limit>` |
| `customers` | List customers | `-l, --limit <limit>` |
| `orders` | List orders | `-l, --limit <limit>`, `-s, --status <status>` |
| `collections` | List collections | `-l, --limit <limit>` |
| `inventory` | List inventory levels | `-l, --limit <limit>` |
| `webhooks` | List recent webhook events | `-l, --limit <limit>`, `-t, --topic <topic>` |
| `analytics` | Show daily sales, top products, and top customers | - |

---

## REST API

### Data Sync

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/sync` | Trigger data sync (optional `resources` filter) |
| `GET` | `/api/status` | Get sync status and statistics |

### Shop

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/shop` | Get shop details |

### Products

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/products` | List products with pagination |
| `GET` | `/api/products/:id` | Get product with variants |

### Customers

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/customers` | List customers with pagination |
| `GET` | `/api/customers/:id` | Get customer details |

### Orders

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/orders` | List orders with pagination and status filter |
| `GET` | `/api/orders/:id` | Get order with line items |

### Collections and Inventory

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/collections` | List collections with pagination |
| `GET` | `/api/inventory` | List inventory levels with pagination |

### Webhook Events

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/webhook` | Receive Shopify webhook (HMAC-SHA256 verified) |
| `GET` | `/api/webhook-events` | List recent webhook events |

### Analytics

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/analytics/daily-sales` | Daily sales revenue and order counts |
| `GET` | `/api/analytics/top-products` | Top products by units sold |
| `GET` | `/api/analytics/customer-value` | Top customers by total spend |

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (verifies database) |
| `GET` | `/live` | Liveness check with shop info and stats |

---

## Database Schema

All tables use composite primary keys `(id, source_account_id)` for multi-app isolation.

### `shopify_shops`

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGINT | Shopify shop ID (PK) |
| `source_account_id` | VARCHAR(128) | Multi-app isolation (PK) |
| `name` | VARCHAR(255) | Shop name |
| `email` | VARCHAR(255) | Contact email |
| `domain` | VARCHAR(255) | Primary domain |
| `myshopify_domain` | VARCHAR(255) | Myshopify domain |
| `shop_owner` | VARCHAR(255) | Shop owner name |
| `phone` | VARCHAR(50) | Phone number |
| `address1` / `address2` | TEXT | Address lines |
| `city` / `province` / `country` / `zip` | VARCHAR | Location fields |
| `province_code` / `country_code` | VARCHAR(10) | Location codes |
| `currency` | VARCHAR(10) | Store currency |
| `money_format` | VARCHAR(50) | Money display format |
| `timezone` / `iana_timezone` | VARCHAR | Timezone identifiers |
| `plan_name` / `plan_display_name` | VARCHAR | Shopify plan |
| `weight_unit` | VARCHAR(10) | Weight unit (kg/lb) |
| `primary_locale` | VARCHAR(10) | Primary locale |
| `created_at` / `updated_at` / `synced_at` | TIMESTAMPTZ | Timestamps |

### `shopify_products`

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGINT | Product ID (PK) |
| `source_account_id` | VARCHAR(128) | Multi-app isolation (PK) |
| `title` | VARCHAR(255) | Product title |
| `body_html` | TEXT | Product description (HTML) |
| `vendor` | VARCHAR(255) | Vendor name |
| `product_type` | VARCHAR(255) | Product type |
| `handle` | VARCHAR(255) | URL handle |
| `status` | VARCHAR(50) | Status (active/draft/archived) |
| `tags` | TEXT | Comma-separated tags |
| `images` | JSONB | Image objects array |
| `options` | JSONB | Product options array |
| `published_at` | TIMESTAMPTZ | Publication date |
| `created_at` / `updated_at` / `synced_at` | TIMESTAMPTZ | Timestamps |

### `shopify_variants`

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGINT | Variant ID (PK) |
| `source_account_id` | VARCHAR(128) | Multi-app isolation (PK) |
| `product_id` | BIGINT | Parent product ID |
| `title` | VARCHAR(255) | Variant title |
| `price` / `compare_at_price` | DECIMAL(10,2) | Price fields |
| `sku` | VARCHAR(255) | SKU |
| `barcode` | VARCHAR(255) | Barcode |
| `inventory_item_id` | BIGINT | Inventory item ID |
| `inventory_quantity` | INTEGER | Current stock quantity |
| `inventory_policy` | VARCHAR(50) | Stock policy (deny/continue) |
| `option1` / `option2` / `option3` | VARCHAR(255) | Option values |
| `grams` / `weight` / `weight_unit` | - | Weight fields |
| `requires_shipping` / `taxable` | BOOLEAN | Shipping and tax flags |
| `created_at` / `updated_at` / `synced_at` | TIMESTAMPTZ | Timestamps |

### `shopify_collections`

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGINT | Collection ID (PK) |
| `source_account_id` | VARCHAR(128) | Multi-app isolation (PK) |
| `title` | VARCHAR(255) | Collection title |
| `body_html` | TEXT | Description (HTML) |
| `handle` | VARCHAR(255) | URL handle |
| `collection_type` | VARCHAR(50) | Type (custom/smart) |
| `sort_order` | VARCHAR(50) | Sort order |
| `products_count` | INTEGER | Number of products |
| `rules` | JSONB | Smart collection rules |
| `image` | JSONB | Collection image |
| `published_at` / `updated_at` / `synced_at` | TIMESTAMPTZ | Timestamps |

### `shopify_customers`

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGINT | Customer ID (PK) |
| `source_account_id` | VARCHAR(128) | Multi-app isolation (PK) |
| `email` | VARCHAR(255) | Email address |
| `first_name` / `last_name` | VARCHAR(255) | Name |
| `phone` | VARCHAR(50) | Phone number |
| `orders_count` | INTEGER | Total orders |
| `total_spent` | DECIMAL(12,2) | Total spend |
| `state` | VARCHAR(50) | Account state |
| `accepts_marketing` | BOOLEAN | Marketing opt-in |
| `tax_exempt` | BOOLEAN | Tax exemption |
| `addresses` | JSONB | Address book |
| `default_address` | JSONB | Default address |
| `tags` | TEXT | Customer tags |
| `created_at` / `updated_at` / `synced_at` | TIMESTAMPTZ | Timestamps |

### `shopify_orders`

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGINT | Order ID (PK) |
| `source_account_id` | VARCHAR(128) | Multi-app isolation (PK) |
| `order_number` | INTEGER | Order number |
| `name` | VARCHAR(50) | Display name (#1001) |
| `email` | VARCHAR(255) | Customer email |
| `customer_id` | BIGINT | Customer ID |
| `financial_status` | VARCHAR(50) | Payment status |
| `fulfillment_status` | VARCHAR(50) | Fulfillment status |
| `total_price` / `subtotal_price` | DECIMAL(12,2) | Price totals |
| `total_discounts` / `total_tax` | DECIMAL(12,2) | Discount and tax |
| `currency` | VARCHAR(10) | Currency code |
| `billing_address` / `shipping_address` | JSONB | Address objects |
| `shipping_lines` | JSONB | Shipping methods |
| `discount_codes` | JSONB | Applied discounts |
| `gateway` | VARCHAR(100) | Payment gateway |
| `cancel_reason` | VARCHAR(50) | Cancellation reason |
| `cancelled_at` / `closed_at` / `processed_at` | TIMESTAMPTZ | Status timestamps |
| `created_at` / `updated_at` / `synced_at` | TIMESTAMPTZ | Timestamps |

### `shopify_order_items`

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGINT | Line item ID (PK) |
| `source_account_id` | VARCHAR(128) | Multi-app isolation (PK) |
| `order_id` | BIGINT | Parent order ID |
| `product_id` / `variant_id` | BIGINT | Product references |
| `title` / `variant_title` | VARCHAR(255) | Item names |
| `sku` / `vendor` | VARCHAR(255) | SKU and vendor |
| `quantity` | INTEGER | Quantity ordered |
| `price` | DECIMAL(12,2) | Unit price |
| `total_discount` | DECIMAL(12,2) | Applied discount |
| `fulfillment_status` | VARCHAR(50) | Item fulfillment status |

### Additional Tables

| Table | Description |
|-------|-------------|
| `shopify_locations` | Store locations with address and active status |
| `shopify_fulfillments` | Order fulfillments with tracking numbers and URLs |
| `shopify_transactions` | Payment transactions with gateway, amount, and status |
| `shopify_refunds` | Refund records with line items and adjustments |
| `shopify_draft_orders` | Draft orders with line items, discounts, and invoice URL |
| `shopify_inventory_items` | Inventory items with SKU, cost, and origin codes |
| `shopify_inventory` | Inventory levels per location (PK: inventory_item_id, location_id, source_account_id) |
| `shopify_price_rules` | Discount price rules with targeting and prerequisites |
| `shopify_discount_codes` | Discount codes linked to price rules |
| `shopify_gift_cards` | Gift cards with balance, initial value, and expiration |
| `shopify_metafields` | Custom metafields with namespace, key, and owner |
| `shopify_checkouts` | Abandoned checkouts with cart and billing details |
| `shopify_webhook_events` | Raw webhook event log with topic and processing status |

---

## Views

| View | Description |
|------|-------------|
| `shopify_sales_overview` | Daily revenue, order count, average order value, unique customers (paid, non-test orders) |
| `shopify_top_products` | Products ranked by units sold with order count and revenue |
| `shopify_customer_value` | Customers ranked by total spend with order count |

---

## Webhooks

### Signature Verification

Webhooks are verified using HMAC-SHA256 with the `X-Shopify-Hmac-SHA256` header. Set `SHOPIFY_WEBHOOK_SECRET` to enable verification.

### Supported Events

| Event | Description |
|-------|-------------|
| `orders/create` | New order placed |
| `orders/updated` | Order updated |
| `orders/paid` | Order payment received |
| `orders/fulfilled` | Order fulfilled |
| `orders/cancelled` | Order cancelled |
| `products/create` | Product created |
| `products/update` | Product updated |
| `products/delete` | Product deleted |
| `customers/create` | Customer registered |
| `customers/update` | Customer updated |
| `inventory_levels/update` | Inventory level changed |
| `refunds/create` | Refund issued |
| `fulfillments/create` | Fulfillment created |

---

## Features

- **Full store sync** of 20 resource types to PostgreSQL
- **Real-time webhooks** with HMAC-SHA256 signature verification
- **Composite primary keys** `(id, source_account_id)` for multi-store isolation
- **Multi-store support** via comma-separated credential environment variables
- **Analytics views** for daily sales, top products, and customer lifetime value
- **Selective sync** by resource type (shop, products, collections, customers, orders, inventory)
- **Rate limiting** respecting Shopify API limits (2 req/s default)
- **Complete order data** including line items, fulfillments, transactions, and refunds
- **Inventory tracking** per location with available, incoming, committed, and reserved quantities
- **Abandoned checkout tracking** for cart recovery analysis

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Sync fails with 401 | Verify `SHOPIFY_ACCESS_TOKEN` has required API scopes |
| Webhook signature invalid | Ensure `SHOPIFY_WEBHOOK_SECRET` matches the secret in Shopify admin |
| Missing order items | Run `sync -r orders` to sync orders with their line items |
| Inventory shows zero | Verify inventory tracking is enabled in Shopify for the product |
| Multi-store not working | Ensure `SHOPIFY_ACCESS_TOKENS`, `SHOPIFY_SHOP_DOMAINS`, and `SHOPIFY_ACCOUNT_LABELS` have matching comma-separated counts |
| Analytics views empty | Views require paid, non-test orders; run a sync first |
