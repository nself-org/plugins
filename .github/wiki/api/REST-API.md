# REST API Reference

Complete API reference for all nself plugins. All plugins follow consistent patterns for authentication, pagination, error handling, and response formats.

**Last Updated**: January 30, 2026
**Version**: 1.0.0

---

## Table of Contents

1. [Overview](#overview)
2. [Authentication](#authentication)
3. [Common Patterns](#common-patterns)
4. [Response Formats](#response-formats)
5. [Error Codes](#error-codes)
6. [Rate Limiting](#rate-limiting)
7. [Plugin APIs](#plugin-apis)
   - [Stripe](#stripe-api)
   - [GitHub](#github-api)
   - [Shopify](#shopify-api)
   - [Realtime](#realtime-api)
   - [File Processing](#file-processing-api)
   - [Jobs](#jobs-api)
   - [Notifications](#notifications-api)
   - [ID.me](#idme-api)

---

## Overview

All nself plugins expose REST APIs built with Fastify. Each plugin runs on its own port and provides:

- Health check endpoints
- Data query endpoints
- Webhook receivers
- Administrative endpoints

### Base URLs

| Plugin | Default Port | Base URL |
|--------|--------------|----------|
| Stripe | 3001 | `http://localhost:3001` |
| GitHub | 3002 | `http://localhost:3002` |
| Shopify | 3003 | `http://localhost:3003` |
| ID.me | 3010 | `http://localhost:3010` |
| Realtime | 3101 | `http://localhost:3101` |
| Notifications | 3102 | `http://localhost:3102` |
| File Processing | 3104 | `http://localhost:3104` |
| Jobs | 3105 | `http://localhost:3105` |

---

## Authentication

### API Key Authentication

Most plugins support optional API key authentication via the `PLUGIN_API_KEY` environment variable.

**Header Format**:
```
X-API-Key: your_api_key_here
```

**Example**:
```bash
curl -H "X-API-Key: sk_test_123..." http://localhost:3001/api/customers
```

### Webhook Signature Verification

Webhooks use provider-specific signature verification:

#### Stripe
- Header: `Stripe-Signature`
- Method: HMAC-SHA256 with timestamp
- Secret: `STRIPE_WEBHOOK_SECRET`

#### GitHub
- Header: `X-Hub-Signature-256`
- Method: HMAC-SHA256
- Secret: `GITHUB_WEBHOOK_SECRET`

#### Shopify
- Header: `X-Shopify-Hmac-Sha256`
- Method: HMAC-SHA256, Base64-encoded
- Secret: `SHOPIFY_WEBHOOK_SECRET`

#### ID.me
- Header: `X-IDme-Signature`
- Method: HMAC-SHA256
- Secret: `IDME_WEBHOOK_SECRET`

---

## Common Patterns

### Pagination

List endpoints support offset-based pagination:

**Query Parameters**:
- `limit` (default: 100, max: 1000) - Number of items to return
- `offset` (default: 0) - Number of items to skip

**Response Format**:
```json
{
  "data": [...],
  "total": 1523,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# Get first 50 customers
curl "http://localhost:3001/api/customers?limit=50&offset=0"

# Get next 50 customers
curl "http://localhost:3001/api/customers?limit=50&offset=50"
```

### Filtering

Many list endpoints support filtering via query parameters:

**Common Filters**:
- `status` - Filter by status (e.g., active, inactive, pending)
- `state` - Filter by state (e.g., open, closed)
- `type` - Filter by type
- `created_after` - Filter by creation date
- `updated_after` - Filter by update date

**Example**:
```bash
# Get active subscriptions
curl "http://localhost:3001/api/subscriptions?status=active"

# Get open issues
curl "http://localhost:3002/api/issues?state=open"
```

### Sorting

Results are typically sorted by most relevant field:

- Customer lists: `created_at DESC`
- Issues/PRs: `created_at DESC`
- Orders: `created_at DESC`
- Events: `received_at DESC`

### Expansion

Some endpoints support expanding related resources:

**Example**:
```bash
# Get product with variants
curl "http://localhost:3003/api/products/123"
# Returns: { product: {...}, variants: [...] }
```

---

## Response Formats

### Success Response

```json
{
  "data": {...},
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

### List Response

```json
{
  "data": [...],
  "total": 1523,
  "limit": 100,
  "offset": 0
}
```

### Error Response

```json
{
  "error": "Customer not found",
  "code": "NOT_FOUND",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

### Webhook Response

```json
{
  "received": true
}
```

---

## Error Codes

### HTTP Status Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 200 | OK | Request succeeded |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Invalid request parameters |
| 401 | Unauthorized | Invalid or missing API key |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not Found | Resource not found |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Server error |
| 503 | Service Unavailable | Database or service unavailable |

### Common Error Codes

| Code | Description |
|------|-------------|
| `INVALID_REQUEST` | Request validation failed |
| `NOT_FOUND` | Resource not found |
| `UNAUTHORIZED` | Authentication failed |
| `RATE_LIMIT_EXCEEDED` | Too many requests |
| `INTERNAL_ERROR` | Server error |
| `DATABASE_ERROR` | Database operation failed |
| `INVALID_SIGNATURE` | Webhook signature verification failed |

---

## Rate Limiting

### Default Limits

All plugins use configurable rate limiting:

- **Default**: 100 requests per minute per IP
- **Configurable via**: `PLUGIN_RATE_LIMIT_MAX` and `PLUGIN_RATE_LIMIT_WINDOW_MS`

### Rate Limit Headers

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1706616000
```

### Rate Limit Response

```json
{
  "error": "Rate limit exceeded",
  "code": "RATE_LIMIT_EXCEEDED",
  "retryAfter": 60
}
```

---

## Plugin APIs

---

## Stripe API

Stripe plugin syncs billing data and handles webhooks.

**Base URL**: `http://localhost:3001`
**Port**: 3001

### Health Endpoints

#### GET /health

Basic health check.

**Response**:
```json
{
  "status": "ok",
  "plugin": "stripe",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Example**:
```bash
curl http://localhost:3001/health
```

#### GET /ready

Readiness check with database verification.

**Response**:
```json
{
  "ready": true,
  "plugin": "stripe",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Error Response** (503):
```json
{
  "ready": false,
  "plugin": "stripe",
  "error": "Database unavailable",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Example**:
```bash
curl http://localhost:3001/ready
```

#### GET /live

Liveness check with statistics.

**Response**:
```json
{
  "alive": true,
  "plugin": "stripe",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 45678912,
    "heapTotal": 20971520,
    "heapUsed": 18874368,
    "external": 1234567
  },
  "stats": {
    "customers": 1250,
    "subscriptions": 350,
    "lastSync": "2026-01-30T11:45:00.000Z"
  },
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Example**:
```bash
curl http://localhost:3001/live
```

#### GET /status

Detailed status with statistics.

**Response**:
```json
{
  "plugin": "stripe",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "customers": 1250,
    "products": 45,
    "prices": 120,
    "subscriptions": 350,
    "invoices": 2300,
    "paymentIntents": 5600,
    "paymentMethods": 1400,
    "charges": 5800,
    "refunds": 230,
    "disputes": 12,
    "coupons": 25,
    "promotionCodes": 40,
    "balanceTransactions": 6100,
    "taxRates": 8,
    "webhookEvents": 15600,
    "lastSyncedAt": "2026-01-30T11:45:00.000Z"
  },
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Example**:
```bash
curl http://localhost:3001/status
```

### Webhook Endpoint

#### POST /webhooks/stripe

Receive Stripe webhook events.

**Headers**:
- `Stripe-Signature` (required) - Webhook signature
- `Content-Type: application/json`

**Request Body**: Stripe event payload

**Response**:
```json
{
  "received": true
}
```

**Error Responses**:
- 400: Missing signature
- 401: Invalid signature
- 500: Processing failed

**Example**:
```bash
curl -X POST http://localhost:3001/webhooks/stripe \
  -H "Stripe-Signature: t=1706616000,v1=abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "id": "evt_1234",
    "type": "customer.created",
    "data": {...}
  }'
```

### Sync Endpoint

#### POST /sync

Trigger data synchronization.

**Request Body**:
```json
{
  "resources": ["customers", "products", "subscriptions"],
  "incremental": true
}
```

**Parameters**:
- `resources` (optional) - Array of resources to sync. Omit to sync all.
  - Valid values: `customers`, `products`, `prices`, `subscriptions`, `invoices`, `payment_intents`, `payment_methods`
- `incremental` (optional, default: false) - Only sync data updated since last sync

**Response**:
```json
{
  "success": true,
  "synced": {
    "customers": 150,
    "products": 12,
    "subscriptions": 45
  },
  "duration": 12.5,
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Example**:
```bash
# Sync all resources
curl -X POST http://localhost:3001/sync \
  -H "Content-Type: application/json"

# Sync specific resources incrementally
curl -X POST http://localhost:3001/sync \
  -H "Content-Type: application/json" \
  -d '{
    "resources": ["customers", "subscriptions"],
    "incremental": true
  }'
```

### Customers

#### GET /api/customers

List all customers.

**Query Parameters**:
- `limit` (default: 100) - Number of results
- `offset` (default: 0) - Pagination offset

**Response**:
```json
{
  "data": [
    {
      "id": "cus_1234",
      "email": "customer@example.com",
      "name": "John Doe",
      "description": null,
      "phone": "+15551234567",
      "address": {
        "line1": "123 Main St",
        "city": "San Francisco",
        "state": "CA",
        "postal_code": "94102",
        "country": "US"
      },
      "currency": "usd",
      "balance": 0,
      "delinquent": false,
      "invoice_prefix": "ABCD1234",
      "default_source": "card_5678",
      "metadata": {},
      "created_at": "2026-01-15T10:30:00.000Z",
      "updated_at": "2026-01-30T11:45:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 1250,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/customers?limit=50&offset=0
```

#### GET /api/customers/:id

Get a specific customer.

**Path Parameters**:
- `id` - Customer ID (e.g., `cus_1234`)

**Response**:
```json
{
  "id": "cus_1234",
  "email": "customer@example.com",
  "name": "John Doe",
  ...
}
```

**Error Response** (404):
```json
{
  "error": "Customer not found"
}
```

**Example**:
```bash
curl http://localhost:3001/api/customers/cus_1234
```

### Products

#### GET /api/products

List all products.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "data": [
    {
      "id": "prod_1234",
      "name": "Premium Plan",
      "description": "Full access to all features",
      "active": true,
      "type": "service",
      "url": "https://example.com/premium",
      "metadata": {},
      "created_at": "2025-12-01T00:00:00.000Z",
      "updated_at": "2026-01-15T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 45,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/products
```

#### GET /api/products/:id

Get a specific product.

**Response**:
```json
{
  "id": "prod_1234",
  "name": "Premium Plan",
  "description": "Full access to all features",
  ...
}
```

**Example**:
```bash
curl http://localhost:3001/api/products/prod_1234
```

### Prices

#### GET /api/prices

List all prices.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "data": [
    {
      "id": "price_1234",
      "product_id": "prod_1234",
      "active": true,
      "currency": "usd",
      "type": "recurring",
      "unit_amount": 2999,
      "recurring_interval": "month",
      "recurring_interval_count": 1,
      "billing_scheme": "per_unit",
      "metadata": {},
      "created_at": "2025-12-01T00:00:00.000Z",
      "updated_at": "2026-01-15T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 120,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/prices
```

#### GET /api/prices/:id

Get a specific price.

**Example**:
```bash
curl http://localhost:3001/api/prices/price_1234
```

### Subscriptions

#### GET /api/subscriptions

List all subscriptions.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `status` (optional) - Filter by status: `active`, `past_due`, `canceled`, `incomplete`, `incomplete_expired`, `trialing`, `unpaid`

**Response**:
```json
{
  "data": [
    {
      "id": "sub_1234",
      "customer_id": "cus_1234",
      "status": "active",
      "currency": "usd",
      "current_period_start": "2026-01-01T00:00:00.000Z",
      "current_period_end": "2026-02-01T00:00:00.000Z",
      "cancel_at_period_end": false,
      "canceled_at": null,
      "ended_at": null,
      "trial_start": null,
      "trial_end": null,
      "collection_method": "charge_automatically",
      "metadata": {},
      "created_at": "2025-12-15T10:00:00.000Z",
      "updated_at": "2026-01-15T12:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 350,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All subscriptions
curl http://localhost:3001/api/subscriptions

# Active subscriptions only
curl "http://localhost:3001/api/subscriptions?status=active"
```

#### GET /api/subscriptions/:id

Get a specific subscription.

**Example**:
```bash
curl http://localhost:3001/api/subscriptions/sub_1234
```

### Invoices

#### GET /api/invoices

List all invoices.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `status` (optional) - Filter by status: `draft`, `open`, `paid`, `void`, `uncollectible`

**Response**:
```json
{
  "data": [
    {
      "id": "in_1234",
      "customer_id": "cus_1234",
      "subscription_id": "sub_1234",
      "status": "paid",
      "currency": "usd",
      "amount_due": 2999,
      "amount_paid": 2999,
      "amount_remaining": 0,
      "subtotal": 2999,
      "tax": 0,
      "total": 2999,
      "paid": true,
      "attempted": true,
      "number": "ABCD1234-0001",
      "hosted_invoice_url": "https://invoice.stripe.com/i/...",
      "invoice_pdf": "https://pay.stripe.com/invoice/.../pdf",
      "due_date": "2026-02-01T00:00:00.000Z",
      "period_start": "2026-01-01T00:00:00.000Z",
      "period_end": "2026-02-01T00:00:00.000Z",
      "metadata": {},
      "created_at": "2026-01-01T00:00:00.000Z",
      "updated_at": "2026-01-01T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 2300,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All invoices
curl http://localhost:3001/api/invoices

# Paid invoices
curl "http://localhost:3001/api/invoices?status=paid"
```

#### GET /api/invoices/:id

Get a specific invoice.

**Example**:
```bash
curl http://localhost:3001/api/invoices/in_1234
```

### Charges

#### GET /api/charges

List all charges.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "data": [
    {
      "id": "ch_1234",
      "customer_id": "cus_1234",
      "amount": 2999,
      "currency": "usd",
      "status": "succeeded",
      "paid": true,
      "refunded": false,
      "amount_refunded": 0,
      "payment_method_id": "pm_5678",
      "receipt_url": "https://pay.stripe.com/receipts/...",
      "failure_code": null,
      "failure_message": null,
      "metadata": {},
      "created_at": "2026-01-01T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 5800,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/charges
```

#### GET /api/charges/:id

Get a specific charge.

**Example**:
```bash
curl http://localhost:3001/api/charges/ch_1234
```

### Refunds

#### GET /api/refunds

List all refunds.

**Response**:
```json
{
  "data": [
    {
      "id": "re_1234",
      "charge_id": "ch_1234",
      "amount": 2999,
      "currency": "usd",
      "status": "succeeded",
      "reason": "requested_by_customer",
      "metadata": {},
      "created_at": "2026-01-15T12:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 230,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/refunds
```

#### GET /api/refunds/:id

Get a specific refund.

**Example**:
```bash
curl http://localhost:3001/api/refunds/re_1234
```

### Disputes

#### GET /api/disputes

List all disputes.

**Response**:
```json
{
  "data": [
    {
      "id": "dp_1234",
      "charge_id": "ch_1234",
      "amount": 2999,
      "currency": "usd",
      "status": "needs_response",
      "reason": "fraudulent",
      "is_charge_refundable": true,
      "due_by": "2026-02-15T23:59:59.000Z",
      "metadata": {},
      "created_at": "2026-01-20T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 12,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/disputes
```

#### GET /api/disputes/:id

Get a specific dispute.

**Example**:
```bash
curl http://localhost:3001/api/disputes/dp_1234
```

### Payment Intents

#### GET /api/payment-intents

List all payment intents.

**Response**:
```json
{
  "data": [
    {
      "id": "pi_1234",
      "customer_id": "cus_1234",
      "amount": 2999,
      "currency": "usd",
      "status": "succeeded",
      "payment_method_id": "pm_5678",
      "confirmation_method": "automatic",
      "capture_method": "automatic",
      "receipt_email": "customer@example.com",
      "metadata": {},
      "created_at": "2026-01-01T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 5600,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/payment-intents
```

#### GET /api/payment-intents/:id

Get a specific payment intent.

**Example**:
```bash
curl http://localhost:3001/api/payment-intents/pi_1234
```

### Payment Methods

#### GET /api/payment-methods

List all payment methods.

**Response**:
```json
{
  "data": [
    {
      "id": "pm_1234",
      "customer_id": "cus_1234",
      "type": "card",
      "card_brand": "visa",
      "card_last4": "4242",
      "card_exp_month": 12,
      "card_exp_year": 2027,
      "card_funding": "credit",
      "billing_details": {
        "name": "John Doe",
        "email": "customer@example.com",
        "phone": "+15551234567",
        "address": {
          "line1": "123 Main St",
          "city": "San Francisco",
          "state": "CA",
          "postal_code": "94102",
          "country": "US"
        }
      },
      "metadata": {},
      "created_at": "2025-12-15T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 1400,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/payment-methods
```

#### GET /api/payment-methods/:id

Get a specific payment method.

**Example**:
```bash
curl http://localhost:3001/api/payment-methods/pm_1234
```

### Coupons

#### GET /api/coupons

List all coupons.

**Response**:
```json
{
  "data": [
    {
      "id": "25OFF",
      "name": "25% off",
      "percent_off": 25,
      "amount_off": null,
      "currency": null,
      "duration": "repeating",
      "duration_in_months": 3,
      "max_redemptions": null,
      "times_redeemed": 45,
      "valid": true,
      "redeem_by": null,
      "metadata": {},
      "created_at": "2025-12-01T00:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 25,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/coupons
```

#### GET /api/coupons/:id

Get a specific coupon.

**Example**:
```bash
curl http://localhost:3001/api/coupons/25OFF
```

### Promotion Codes

#### GET /api/promotion-codes

List all promotion codes.

**Response**:
```json
{
  "data": [
    {
      "id": "promo_1234",
      "code": "WINTER25",
      "coupon_id": "25OFF",
      "active": true,
      "max_redemptions": 1000,
      "times_redeemed": 145,
      "expires_at": "2026-03-01T00:00:00.000Z",
      "metadata": {},
      "created_at": "2025-12-01T00:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 40,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/promotion-codes
```

#### GET /api/promotion-codes/:id

Get a specific promotion code.

**Example**:
```bash
curl http://localhost:3001/api/promotion-codes/promo_1234
```

### Balance Transactions

#### GET /api/balance-transactions

List all balance transactions.

**Response**:
```json
{
  "data": [
    {
      "id": "txn_1234",
      "type": "charge",
      "amount": 2879,
      "currency": "usd",
      "net": 2879,
      "fee": 120,
      "status": "available",
      "description": "Payment for invoice ABCD1234-0001",
      "source_id": "ch_5678",
      "available_on": "2026-01-03T00:00:00.000Z",
      "created_at": "2026-01-01T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 6100,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/balance-transactions
```

#### GET /api/balance-transactions/:id

Get a specific balance transaction.

**Example**:
```bash
curl http://localhost:3001/api/balance-transactions/txn_1234
```

### Tax Rates

#### GET /api/tax-rates

List all tax rates.

**Response**:
```json
{
  "data": [
    {
      "id": "txr_1234",
      "display_name": "Sales Tax",
      "description": "California sales tax",
      "jurisdiction": "CA",
      "percentage": 8.5,
      "inclusive": false,
      "active": true,
      "metadata": {},
      "created_at": "2025-12-01T00:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 8,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3001/api/tax-rates
```

#### GET /api/tax-rates/:id

Get a specific tax rate.

**Example**:
```bash
curl http://localhost:3001/api/tax-rates/txr_1234
```

### Webhook Events

#### GET /api/events

List webhook events.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `type` (optional) - Filter by event type (e.g., `customer.created`)

**Response**:
```json
{
  "data": [
    {
      "id": "evt_1234",
      "type": "customer.created",
      "received_at": "2026-01-30T12:00:00.000Z",
      "processed_at": "2026-01-30T12:00:01.000Z",
      "processed": true,
      "error_message": null,
      "payload": {...}
    }
  ],
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All events
curl http://localhost:3001/api/events

# Customer events only
curl "http://localhost:3001/api/events?type=customer.created"
```

### Statistics

#### GET /api/stats

Get database statistics.

**Response**:
```json
{
  "customers": 1250,
  "products": 45,
  "prices": 120,
  "subscriptions": 350,
  "invoices": 2300,
  "paymentIntents": 5600,
  "paymentMethods": 1400,
  "charges": 5800,
  "refunds": 230,
  "disputes": 12,
  "coupons": 25,
  "promotionCodes": 40,
  "balanceTransactions": 6100,
  "taxRates": 8,
  "webhookEvents": 15600,
  "lastSyncedAt": "2026-01-30T11:45:00.000Z"
}
```

**Example**:
```bash
curl http://localhost:3001/api/stats
```

---

## GitHub API

GitHub plugin syncs repository data and handles webhooks.

**Base URL**: `http://localhost:3002`
**Port**: 3002

### Health Endpoints

#### GET /health

**Response**:
```json
{
  "status": "ok",
  "plugin": "github",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

#### GET /ready

**Response**:
```json
{
  "ready": true,
  "plugin": "github",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

#### GET /live

**Response**:
```json
{
  "alive": true,
  "plugin": "github",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {...},
  "stats": {
    "repositories": 45,
    "issues": 230,
    "pullRequests": 120,
    "lastSync": "2026-01-30T11:45:00.000Z"
  },
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

#### GET /status

**Response**:
```json
{
  "plugin": "github",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "repositories": 45,
    "issues": 230,
    "pullRequests": 120,
    "commits": 5600,
    "releases": 28,
    "branches": 180,
    "tags": 35,
    "milestones": 12,
    "labels": 65,
    "workflows": 15,
    "workflowRuns": 1250,
    "workflowJobs": 3400,
    "checkSuites": 890,
    "checkRuns": 2300,
    "deployments": 145,
    "teams": 8,
    "collaborators": 42,
    "prReviews": 340,
    "issueComments": 1200,
    "prReviewComments": 890,
    "commitComments": 120,
    "webhookEvents": 4500,
    "lastSyncedAt": "2026-01-30T11:45:00.000Z"
  },
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

### Webhook Endpoint

#### POST /webhooks/github

Receive GitHub webhook events.

**Headers**:
- `X-Hub-Signature-256` (required) - Webhook signature
- `X-GitHub-Event` (required) - Event type
- `X-GitHub-Delivery` (required) - Delivery ID
- `Content-Type: application/json`

**Response**:
```json
{
  "received": true
}
```

**Example**:
```bash
curl -X POST http://localhost:3002/webhooks/github \
  -H "X-Hub-Signature-256: sha256=abc123..." \
  -H "X-GitHub-Event: push" \
  -H "X-GitHub-Delivery: 12345678-1234-1234-1234-123456789012" \
  -H "Content-Type: application/json" \
  -d '{
    "ref": "refs/heads/main",
    "repository": {...},
    "commits": [...]
  }'
```

### Sync Endpoint

#### POST /sync

Trigger data synchronization.

**Request Body**:
```json
{
  "resources": ["repositories", "issues", "pull_requests"],
  "repos": ["owner/repo1", "owner/repo2"],
  "since": "2026-01-01T00:00:00.000Z"
}
```

**Parameters**:
- `resources` (optional) - Array of resources to sync
  - Valid values: `repositories`, `issues`, `pull_requests`, `commits`, `releases`, `workflow_runs`, `deployments`
- `repos` (optional) - Array of specific repos to sync (format: `owner/repo`)
- `since` (optional) - ISO timestamp to sync from

**Response**:
```json
{
  "success": true,
  "synced": {
    "repositories": 45,
    "issues": 23,
    "pull_requests": 12
  },
  "duration": 18.3,
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Example**:
```bash
# Sync all resources
curl -X POST http://localhost:3002/sync

# Sync specific resources for specific repos
curl -X POST http://localhost:3002/sync \
  -H "Content-Type: application/json" \
  -d '{
    "resources": ["issues", "pull_requests"],
    "repos": ["acamarata/nself-plugins"],
    "since": "2026-01-15T00:00:00.000Z"
  }'
```

### Repositories

#### GET /api/repos

List all repositories.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "data": [
    {
      "id": 12345,
      "name": "nself-plugins",
      "full_name": "acamarata/nself-plugins",
      "owner_login": "acamarata",
      "description": "Official plugin repository for nself CLI",
      "private": false,
      "fork": false,
      "language": "TypeScript",
      "default_branch": "main",
      "homepage": "https://nself.org",
      "stargazers_count": 234,
      "watchers_count": 45,
      "forks_count": 23,
      "open_issues_count": 12,
      "topics": ["plugins", "cli", "typescript"],
      "visibility": "public",
      "archived": false,
      "disabled": false,
      "pushed_at": "2026-01-30T11:45:00.000Z",
      "created_at": "2025-01-01T00:00:00.000Z",
      "updated_at": "2026-01-30T11:45:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 45,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/repos
```

#### GET /api/repos/:fullName

Get a specific repository by full name.

**Path Parameters**:
- `fullName` - Repository full name (URL-encoded, e.g., `acamarata%2Fnself-plugins`)

**Response**:
```json
{
  "id": 12345,
  "name": "nself-plugins",
  "full_name": "acamarata/nself-plugins",
  ...
}
```

**Example**:
```bash
curl http://localhost:3002/api/repos/acamarata%2Fnself-plugins
```

### Issues

#### GET /api/issues

List all issues.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `state` (optional) - Filter by state: `open`, `closed`, `all`
- `repo_id` (optional) - Filter by repository ID

**Response**:
```json
{
  "data": [
    {
      "id": 567890,
      "repo_id": 12345,
      "number": 42,
      "title": "Add support for custom webhooks",
      "body": "It would be great if...",
      "state": "open",
      "user_login": "johndoe",
      "assignee_login": "janedoe",
      "labels": ["enhancement", "good first issue"],
      "milestone_id": 123,
      "comments_count": 5,
      "locked": false,
      "closed_at": null,
      "created_at": "2026-01-20T10:00:00.000Z",
      "updated_at": "2026-01-29T15:30:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 230,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All issues
curl http://localhost:3002/api/issues

# Open issues only
curl "http://localhost:3002/api/issues?state=open"

# Issues for specific repo
curl "http://localhost:3002/api/issues?repo_id=12345"
```

### Pull Requests

#### GET /api/prs

List all pull requests.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `state` (optional) - Filter by state: `open`, `closed`, `merged`, `all`
- `repo_id` (optional) - Filter by repository ID

**Response**:
```json
{
  "data": [
    {
      "id": 789012,
      "repo_id": 12345,
      "number": 85,
      "title": "Fix: Handle rate limiting in GitHub client",
      "body": "This PR adds proper rate limit handling...",
      "state": "open",
      "user_login": "johndoe",
      "head_ref": "fix/rate-limiting",
      "base_ref": "main",
      "draft": false,
      "merged": false,
      "mergeable": true,
      "merged_at": null,
      "merged_by_login": null,
      "comments_count": 3,
      "review_comments_count": 8,
      "commits_count": 4,
      "additions": 145,
      "deletions": 32,
      "changed_files": 6,
      "closed_at": null,
      "created_at": "2026-01-28T14:00:00.000Z",
      "updated_at": "2026-01-30T10:15:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 120,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All PRs
curl http://localhost:3002/api/prs

# Open PRs
curl "http://localhost:3002/api/prs?state=open"
```

### Commits

#### GET /api/commits

List all commits.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "data": [
    {
      "sha": "abc123def456...",
      "repo_id": 12345,
      "message": "fix: Handle rate limiting in GitHub client",
      "author_name": "John Doe",
      "author_email": "john@example.com",
      "author_date": "2026-01-28T14:30:00.000Z",
      "committer_name": "John Doe",
      "committer_email": "john@example.com",
      "committer_date": "2026-01-28T14:30:00.000Z",
      "tree_sha": "def456abc789...",
      "parents": ["xyz789..."],
      "verified": true,
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 5600,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/commits
```

### Releases

#### GET /api/releases

List all releases.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "data": [
    {
      "id": 456789,
      "repo_id": 12345,
      "tag_name": "v1.0.0",
      "name": "Version 1.0.0",
      "body": "## Changes\n\n- Added feature X\n- Fixed bug Y",
      "draft": false,
      "prerelease": false,
      "author_login": "johndoe",
      "published_at": "2026-01-15T10:00:00.000Z",
      "created_at": "2026-01-15T09:45:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 28,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/releases
```

### Branches

#### GET /api/branches

List all branches.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `repo_id` (optional) - Filter by repository ID

**Response**:
```json
{
  "data": [
    {
      "name": "main",
      "repo_id": 12345,
      "sha": "abc123...",
      "protected": true,
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 180,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/branches?repo_id=12345
```

### Tags

#### GET /api/tags

List all tags.

**Response**:
```json
{
  "data": [
    {
      "name": "v1.0.0",
      "repo_id": 12345,
      "sha": "abc123...",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 35,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/tags
```

### Milestones

#### GET /api/milestones

List all milestones.

**Response**:
```json
{
  "data": [
    {
      "id": 234567,
      "repo_id": 12345,
      "number": 5,
      "title": "v1.1.0 Release",
      "description": "Next minor release",
      "state": "open",
      "open_issues": 8,
      "closed_issues": 12,
      "due_on": "2026-02-15T00:00:00.000Z",
      "created_at": "2026-01-01T00:00:00.000Z",
      "updated_at": "2026-01-29T16:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 12,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/milestones
```

### Labels

#### GET /api/labels

List all labels.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `repo_id` (optional) - Filter by repository ID

**Response**:
```json
{
  "data": [
    {
      "id": 345678,
      "repo_id": 12345,
      "name": "bug",
      "color": "d73a4a",
      "description": "Something isn't working",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 65,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/labels?repo_id=12345
```

### Workflows

#### GET /api/workflows

List all workflows.

**Response**:
```json
{
  "data": [
    {
      "id": 456789,
      "repo_id": 12345,
      "name": "CI",
      "path": ".github/workflows/ci.yml",
      "state": "active",
      "badge_url": "https://github.com/.../badge.svg",
      "created_at": "2025-01-01T00:00:00.000Z",
      "updated_at": "2026-01-15T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 15,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/workflows
```

### Workflow Runs

#### GET /api/workflow-runs

List all workflow runs.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `status` (optional) - Filter by status: `queued`, `in_progress`, `completed`
- `conclusion` (optional) - Filter by conclusion: `success`, `failure`, `cancelled`, `skipped`

**Response**:
```json
{
  "data": [
    {
      "id": 567890,
      "repo_id": 12345,
      "workflow_id": 456789,
      "name": "CI",
      "head_branch": "main",
      "head_sha": "abc123...",
      "run_number": 1234,
      "status": "completed",
      "conclusion": "success",
      "event": "push",
      "run_attempt": 1,
      "run_started_at": "2026-01-30T11:45:00.000Z",
      "created_at": "2026-01-30T11:44:55.000Z",
      "updated_at": "2026-01-30T11:48:30.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 1250,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All runs
curl http://localhost:3002/api/workflow-runs

# Failed runs only
curl "http://localhost:3002/api/workflow-runs?conclusion=failure"
```

### Workflow Jobs

#### GET /api/workflow-jobs

List all workflow jobs.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `run_id` (optional) - Filter by workflow run ID

**Response**:
```json
{
  "data": [
    {
      "id": 678901,
      "run_id": 567890,
      "name": "build",
      "status": "completed",
      "conclusion": "success",
      "started_at": "2026-01-30T11:45:10.000Z",
      "completed_at": "2026-01-30T11:48:25.000Z",
      "runner_name": "ubuntu-latest",
      "runner_group_name": "GitHub Actions",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 3400,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl "http://localhost:3002/api/workflow-jobs?run_id=567890"
```

### Check Suites

#### GET /api/check-suites

List all check suites.

**Response**:
```json
{
  "data": [
    {
      "id": 789012,
      "repo_id": 12345,
      "head_branch": "main",
      "head_sha": "abc123...",
      "status": "completed",
      "conclusion": "success",
      "app_name": "GitHub Actions",
      "created_at": "2026-01-30T11:44:55.000Z",
      "updated_at": "2026-01-30T11:48:30.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 890,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/check-suites
```

### Check Runs

#### GET /api/check-runs

List all check runs.

**Response**:
```json
{
  "data": [
    {
      "id": 890123,
      "check_suite_id": 789012,
      "name": "build / ubuntu-latest",
      "status": "completed",
      "conclusion": "success",
      "started_at": "2026-01-30T11:45:10.000Z",
      "completed_at": "2026-01-30T11:48:25.000Z",
      "output_title": "All checks passed",
      "output_summary": "Build completed successfully",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 2300,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/check-runs
```

### Deployments

#### GET /api/deployments

List all deployments.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `environment` (optional) - Filter by environment

**Response**:
```json
{
  "data": [
    {
      "id": 901234,
      "repo_id": 12345,
      "sha": "abc123...",
      "ref": "main",
      "task": "deploy",
      "environment": "production",
      "description": "Deploy to production",
      "creator_login": "johndoe",
      "created_at": "2026-01-30T11:50:00.000Z",
      "updated_at": "2026-01-30T11:55:30.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 145,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All deployments
curl http://localhost:3002/api/deployments

# Production deployments
curl "http://localhost:3002/api/deployments?environment=production"
```

### Teams

#### GET /api/teams

List all teams.

**Response**:
```json
{
  "data": [
    {
      "id": 123456,
      "name": "Engineering",
      "slug": "engineering",
      "description": "Engineering team",
      "privacy": "closed",
      "permission": "push",
      "members_count": 12,
      "repos_count": 25,
      "created_at": "2025-01-01T00:00:00.000Z",
      "updated_at": "2026-01-15T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 8,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/teams
```

### Collaborators

#### GET /api/collaborators

List all collaborators.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `repo_id` (optional) - Filter by repository ID

**Response**:
```json
{
  "data": [
    {
      "login": "johndoe",
      "repo_id": 12345,
      "permission": "admin",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 42,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl "http://localhost:3002/api/collaborators?repo_id=12345"
```

### PR Reviews

#### GET /api/pr-reviews

List all pull request reviews.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `pr_id` (optional) - Filter by pull request ID

**Response**:
```json
{
  "data": [
    {
      "id": 234567,
      "pull_request_id": 789012,
      "user_login": "janedoe",
      "state": "approved",
      "body": "LGTM! Nice work.",
      "submitted_at": "2026-01-29T16:30:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 340,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl "http://localhost:3002/api/pr-reviews?pr_id=789012"
```

### Issue Comments

#### GET /api/issue-comments

List all issue comments.

**Response**:
```json
{
  "data": [
    {
      "id": 345678,
      "issue_id": 567890,
      "user_login": "johndoe",
      "body": "This looks like a great feature!",
      "created_at": "2026-01-25T14:00:00.000Z",
      "updated_at": "2026-01-25T14:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 1200,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/issue-comments
```

### PR Review Comments

#### GET /api/pr-review-comments

List all pull request review comments.

**Response**:
```json
{
  "data": [
    {
      "id": 456789,
      "pull_request_id": 789012,
      "review_id": 234567,
      "user_login": "janedoe",
      "path": "src/client.ts",
      "position": 45,
      "original_position": 45,
      "commit_id": "abc123...",
      "body": "Consider using async/await here",
      "created_at": "2026-01-29T16:15:00.000Z",
      "updated_at": "2026-01-29T16:15:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 890,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/pr-review-comments
```

### Commit Comments

#### GET /api/commit-comments

List all commit comments.

**Response**:
```json
{
  "data": [
    {
      "id": 567890,
      "commit_sha": "abc123...",
      "user_login": "johndoe",
      "body": "Nice refactoring!",
      "path": null,
      "position": null,
      "line": null,
      "created_at": "2026-01-28T15:00:00.000Z",
      "updated_at": "2026-01-28T15:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 120,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3002/api/commit-comments
```

### Webhook Events

#### GET /api/events

List webhook events.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `event` (optional) - Filter by event type

**Response**:
```json
{
  "data": [
    {
      "id": "12345678-1234-1234-1234-123456789012",
      "event": "push",
      "received_at": "2026-01-30T12:00:00.000Z",
      "processed_at": "2026-01-30T12:00:01.000Z",
      "processed": true,
      "error_message": null,
      "payload": {...}
    }
  ],
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All events
curl http://localhost:3002/api/events

# Push events only
curl "http://localhost:3002/api/events?event=push"
```

### Statistics

#### GET /api/stats

Get database statistics.

**Response**:
```json
{
  "repositories": 45,
  "issues": 230,
  "pullRequests": 120,
  "commits": 5600,
  "releases": 28,
  "branches": 180,
  "tags": 35,
  "milestones": 12,
  "labels": 65,
  "workflows": 15,
  "workflowRuns": 1250,
  "workflowJobs": 3400,
  "checkSuites": 890,
  "checkRuns": 2300,
  "deployments": 145,
  "teams": 8,
  "collaborators": 42,
  "prReviews": 340,
  "issueComments": 1200,
  "prReviewComments": 890,
  "commitComments": 120,
  "webhookEvents": 4500,
  "lastSyncedAt": "2026-01-30T11:45:00.000Z"
}
```

**Example**:
```bash
curl http://localhost:3002/api/stats
```

---

## Shopify API

Shopify plugin syncs store data and handles webhooks.

**Base URL**: `http://localhost:3003`
**Port**: 3003

### Health Endpoints

#### GET /health

**Response**:
```json
{
  "status": "ok",
  "plugin": "shopify",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

#### GET /ready

**Response**:
```json
{
  "ready": true,
  "plugin": "shopify",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

#### GET /live

**Response**:
```json
{
  "alive": true,
  "plugin": "shopify",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {...},
  "shop": {
    "name": "My Shop",
    "domain": "myshop.myshopify.com"
  },
  "stats": {
    "products": 450,
    "customers": 1250,
    "orders": 3400,
    "lastSync": "2026-01-30T11:45:00.000Z"
  },
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

### Webhook Endpoint

#### POST /webhook

Receive Shopify webhook events.

**Headers**:
- `X-Shopify-Topic` (required) - Event topic
- `X-Shopify-Shop-Domain` (required) - Shop domain
- `X-Shopify-Hmac-Sha256` (required) - Webhook signature
- `X-Shopify-Webhook-Id` (optional) - Webhook ID
- `Content-Type: application/json`

**Response**:
```json
{
  "received": true
}
```

**Example**:
```bash
curl -X POST http://localhost:3003/webhook \
  -H "X-Shopify-Topic: orders/create" \
  -H "X-Shopify-Shop-Domain: myshop.myshopify.com" \
  -H "X-Shopify-Hmac-Sha256: abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "id": 1234567890,
    "email": "customer@example.com",
    "total_price": "99.99",
    ...
  }'
```

### Sync Endpoint

#### POST /api/sync

Trigger data synchronization.

**Request Body**:
```json
{
  "resources": ["products", "customers", "orders"]
}
```

**Parameters**:
- `resources` (optional) - Array of resources to sync
  - Valid values: `shop`, `products`, `collections`, `customers`, `orders`, `inventory`

**Response**:
```json
{
  "success": true,
  "synced": {
    "products": 450,
    "customers": 150,
    "orders": 250
  },
  "duration": 25.8,
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Example**:
```bash
curl -X POST http://localhost:3003/api/sync \
  -H "Content-Type: application/json" \
  -d '{
    "resources": ["products", "customers"]
  }'
```

### Status

#### GET /api/status

Get sync status.

**Response**:
```json
{
  "shop": {
    "name": "My Shop",
    "domain": "myshop.myshopify.com"
  },
  "stats": {
    "products": 450,
    "variants": 1200,
    "collections": 25,
    "customers": 1250,
    "orders": 3400,
    "orderItems": 8900,
    "inventoryLevels": 1200,
    "webhookEvents": 12000,
    "lastSyncedAt": "2026-01-30T11:45:00.000Z"
  }
}
```

**Example**:
```bash
curl http://localhost:3003/api/status
```

### Shop

#### GET /api/shop

Get shop information.

**Response**:
```json
{
  "shop": {
    "id": 12345678,
    "name": "My Shop",
    "domain": "myshop.myshopify.com",
    "email": "shop@example.com",
    "phone": "+15551234567",
    "address": "123 Main St",
    "city": "San Francisco",
    "province": "CA",
    "country": "US",
    "zip": "94102",
    "currency": "USD",
    "timezone": "America/Los_Angeles",
    "iana_timezone": "America/Los_Angeles",
    "plan_name": "shopify_plus",
    "primary_locale": "en",
    "enabled_presentment_currencies": ["USD", "EUR", "GBP"],
    "created_at": "2024-01-01T00:00:00.000Z",
    "updated_at": "2026-01-15T10:00:00.000Z",
    "synced_at": "2026-01-30T12:00:00.000Z"
  }
}
```

**Example**:
```bash
curl http://localhost:3003/api/shop
```

### Products

#### GET /api/products

List all products.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "products": [
    {
      "id": 1234567890,
      "title": "Premium T-Shirt",
      "body_html": "<p>Comfortable cotton t-shirt</p>",
      "vendor": "My Brand",
      "product_type": "Apparel",
      "tags": ["clothing", "t-shirt", "premium"],
      "status": "active",
      "published_at": "2025-12-01T00:00:00.000Z",
      "handle": "premium-t-shirt",
      "template_suffix": null,
      "options": ["Size", "Color"],
      "images": [{
        "src": "https://cdn.shopify.com/.../image.jpg",
        "position": 1,
        "width": 1200,
        "height": 1200
      }],
      "created_at": "2025-12-01T00:00:00.000Z",
      "updated_at": "2026-01-15T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 450,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3003/api/products
```

#### GET /api/products/:id

Get a specific product with variants.

**Response**:
```json
{
  "product": {
    "id": 1234567890,
    "title": "Premium T-Shirt",
    ...
  },
  "variants": [
    {
      "id": 9876543210,
      "product_id": 1234567890,
      "title": "Small / Black",
      "price": "29.99",
      "sku": "TSHIRT-SM-BLK",
      "position": 1,
      "inventory_policy": "deny",
      "compare_at_price": "39.99",
      "fulfillment_service": "manual",
      "inventory_management": "shopify",
      "option1": "Small",
      "option2": "Black",
      "option3": null,
      "taxable": true,
      "barcode": "123456789012",
      "grams": 200,
      "weight": 0.2,
      "weight_unit": "kg",
      "inventory_quantity": 45,
      "requires_shipping": true,
      "created_at": "2025-12-01T00:00:00.000Z",
      "updated_at": "2026-01-15T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ]
}
```

**Example**:
```bash
curl http://localhost:3003/api/products/1234567890
```

### Customers

#### GET /api/customers

List all customers.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "customers": [
    {
      "id": 2345678901,
      "email": "customer@example.com",
      "first_name": "John",
      "last_name": "Doe",
      "phone": "+15551234567",
      "accepts_marketing": true,
      "accepts_marketing_updated_at": "2025-12-15T10:00:00.000Z",
      "marketing_opt_in_level": "single_opt_in",
      "state": "enabled",
      "verified_email": true,
      "tags": ["vip", "newsletter"],
      "orders_count": 12,
      "total_spent": "1249.88",
      "currency": "USD",
      "tax_exempt": false,
      "note": "VIP customer",
      "addresses": [{
        "address1": "123 Main St",
        "city": "San Francisco",
        "province": "CA",
        "country": "US",
        "zip": "94102"
      }],
      "created_at": "2024-06-15T10:00:00.000Z",
      "updated_at": "2026-01-29T16:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 1250,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3003/api/customers
```

#### GET /api/customers/:id

Get a specific customer.

**Example**:
```bash
curl http://localhost:3003/api/customers/2345678901
```

### Orders

#### GET /api/orders

List all orders.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)
- `status` (optional) - Filter by status: `open`, `closed`, `cancelled`, `any`

**Response**:
```json
{
  "orders": [
    {
      "id": 3456789012,
      "order_number": 1001,
      "name": "#1001",
      "email": "customer@example.com",
      "customer_id": 2345678901,
      "financial_status": "paid",
      "fulfillment_status": "fulfilled",
      "currency": "USD",
      "subtotal_price": "99.98",
      "total_tax": "8.50",
      "total_discounts": "10.00",
      "total_price": "98.48",
      "total_weight": 400,
      "confirmed": true,
      "closed_at": "2026-01-15T12:00:00.000Z",
      "cancelled_at": null,
      "cancel_reason": null,
      "note": "Gift wrap requested",
      "tags": "gift",
      "shipping_address": {
        "name": "John Doe",
        "address1": "123 Main St",
        "city": "San Francisco",
        "province": "CA",
        "country": "US",
        "zip": "94102"
      },
      "billing_address": {...},
      "created_at": "2026-01-15T10:00:00.000Z",
      "updated_at": "2026-01-15T12:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 3400,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
# All orders
curl http://localhost:3003/api/orders

# Open orders only
curl "http://localhost:3003/api/orders?status=open"
```

#### GET /api/orders/:id

Get a specific order with line items.

**Response**:
```json
{
  "order": {
    "id": 3456789012,
    "order_number": 1001,
    ...
  },
  "items": [
    {
      "id": 4567890123,
      "order_id": 3456789012,
      "product_id": 1234567890,
      "variant_id": 9876543210,
      "title": "Premium T-Shirt - Small / Black",
      "quantity": 2,
      "price": "29.99",
      "sku": "TSHIRT-SM-BLK",
      "vendor": "My Brand",
      "fulfillment_status": "fulfilled",
      "requires_shipping": true,
      "taxable": true,
      "name": "Premium T-Shirt - Small / Black",
      "properties": [],
      "product_exists": true,
      "fulfillable_quantity": 0,
      "grams": 200,
      "total_discount": "5.00",
      "tax_lines": [{
        "price": "4.25",
        "rate": 0.085,
        "title": "CA State Tax"
      }],
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ]
}
```

**Example**:
```bash
curl http://localhost:3003/api/orders/3456789012
```

### Collections

#### GET /api/collections

List all collections.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "collections": [
    {
      "id": 5678901234,
      "title": "New Arrivals",
      "handle": "new-arrivals",
      "body_html": "<p>Latest products</p>",
      "published_at": "2026-01-01T00:00:00.000Z",
      "sort_order": "best-selling",
      "template_suffix": null,
      "products_count": 45,
      "collection_type": "smart",
      "published_scope": "web",
      "image": {
        "src": "https://cdn.shopify.com/.../collection.jpg",
        "width": 1200,
        "height": 800
      },
      "created_at": "2025-12-15T00:00:00.000Z",
      "updated_at": "2026-01-15T10:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 25,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3003/api/collections
```

### Inventory

#### GET /api/inventory

List inventory levels.

**Query Parameters**:
- `limit` (default: 100)
- `offset` (default: 0)

**Response**:
```json
{
  "inventory": [
    {
      "inventory_item_id": 6789012345,
      "location_id": 7890123456,
      "available": 45,
      "updated_at": "2026-01-29T16:00:00.000Z",
      "synced_at": "2026-01-30T12:00:00.000Z"
    }
  ],
  "total": 1200,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl http://localhost:3003/api/inventory
```

### Webhook Events

#### GET /api/webhook-events

List webhook events.

**Query Parameters**:
- `limit` (default: 50)
- `topic` (optional) - Filter by topic

**Response**:
```json
{
  "events": [
    {
      "id": "evt_abc123...",
      "topic": "orders/create",
      "shop_domain": "myshop.myshopify.com",
      "received_at": "2026-01-30T12:00:00.000Z",
      "processed_at": "2026-01-30T12:00:01.000Z",
      "processed": true,
      "error_message": null,
      "payload": {...}
    }
  ]
}
```

**Example**:
```bash
# All events
curl http://localhost:3003/api/webhook-events

# Order events only
curl "http://localhost:3003/api/webhook-events?topic=orders/create"
```

### Analytics

#### GET /api/analytics/daily-sales

Get daily sales analytics.

**Query Parameters**:
- `days` (default: 30, max: 365) - Number of days to include

**Response**:
```json
{
  "dailySales": [
    {
      "order_date": "2026-01-30",
      "order_count": 45,
      "total_revenue": "4523.50",
      "avg_order_value": "100.52"
    },
    {
      "order_date": "2026-01-29",
      "order_count": 52,
      "total_revenue": "5234.25",
      "avg_order_value": "100.66"
    }
  ]
}
```

**Example**:
```bash
# Last 30 days
curl http://localhost:3003/api/analytics/daily-sales

# Last 7 days
curl "http://localhost:3003/api/analytics/daily-sales?days=7"
```

#### GET /api/analytics/top-products

Get top-selling products.

**Query Parameters**:
- `limit` (default: 10) - Number of products to return

**Response**:
```json
{
  "topProducts": [
    {
      "product_id": 1234567890,
      "product_title": "Premium T-Shirt",
      "units_sold": 234,
      "revenue": "7015.66",
      "avg_price": "29.98"
    }
  ]
}
```

**Example**:
```bash
curl "http://localhost:3003/api/analytics/top-products?limit=10"
```

#### GET /api/analytics/customer-value

Get top customers by lifetime value.

**Response**:
```json
{
  "customers": [
    {
      "customer_id": 2345678901,
      "customer_email": "vip@example.com",
      "customer_name": "John Doe",
      "order_count": 12,
      "total_spent": "1249.88",
      "avg_order_value": "104.16",
      "first_order": "2024-06-15T10:00:00.000Z",
      "last_order": "2026-01-15T10:00:00.000Z"
    }
  ]
}
```

**Example**:
```bash
curl http://localhost:3003/api/analytics/customer-value
```

---

## Realtime API

Realtime plugin provides Socket.io server for real-time communication.

**Base URL**: `http://localhost:3101`
**Port**: 3101

### HTTP Endpoints

#### GET /health

Health check endpoint.

**Response**:
```json
{
  "status": "healthy",
  "timestamp": "2026-01-30T12:00:00.000Z",
  "connections": 150,
  "uptime": 3600.5
}
```

**Example**:
```bash
curl http://localhost:3101/health
```

#### GET /metrics

Server metrics and statistics.

**Response**:
```json
{
  "uptime": 3600.5,
  "connections": {
    "total": 150,
    "active": 150,
    "authenticated": 142,
    "anonymous": 8
  },
  "rooms": {
    "total": 25,
    "active": 25
  },
  "presence": {
    "online": 142,
    "away": 8,
    "busy": 3
  },
  "events": {
    "total": 45000,
    "lastHour": 1250
  },
  "memory": {
    "used": 45678912,
    "total": 134217728,
    "percentage": 34.02
  },
  "cpu": {
    "usage": 12.5
  }
}
```

**Example**:
```bash
curl http://localhost:3101/metrics
```

#### GET /rooms

List active rooms.

**Response**:
```json
{
  "rooms": [
    {
      "id": "room_123",
      "name": "general",
      "description": "General chat",
      "member_count": 45,
      "created_at": "2026-01-29T10:00:00.000Z"
    }
  ]
}
```

**Example**:
```bash
curl http://localhost:3101/rooms
```

#### GET /connections

List active connections.

**Response**:
```json
{
  "connections": [
    {
      "socket_id": "abc123",
      "user_id": "user_456",
      "connected_at": "2026-01-30T11:30:00.000Z",
      "transport": "websocket",
      "ip_address": "192.168.1.100"
    }
  ]
}
```

**Example**:
```bash
curl http://localhost:3101/connections
```

#### POST /broadcast

Broadcast message to a room.

**Request Body**:
```json
{
  "room": "general",
  "event": "announcement",
  "data": {
    "message": "Server maintenance in 10 minutes",
    "priority": "high"
  }
}
```

**Response**:
```json
{
  "success": true,
  "room": "general",
  "recipients": 45
}
```

**Example**:
```bash
curl -X POST http://localhost:3101/broadcast \
  -H "Content-Type: application/json" \
  -d '{
    "room": "general",
    "event": "announcement",
    "data": {"message": "Hello everyone!"}
  }'
```

#### DELETE /connections/:id

Disconnect a specific socket.

**Response**:
```json
{
  "success": true,
  "socket_id": "abc123"
}
```

**Example**:
```bash
curl -X DELETE http://localhost:3101/connections/abc123
```

### Socket.io Events

#### Client → Server Events

##### connect

Establish connection with authentication.

**Authentication**:
```javascript
const socket = io('http://localhost:3101', {
  auth: {
    token: 'jwt_token_here',
    device: {
      type: 'desktop',
      os: 'macOS'
    }
  }
});
```

**Server Response**:
```javascript
socket.on('connected', (data) => {
  // data: { socketId, serverTime, protocolVersion }
});
```

##### room:join

Join a room.

**Payload**:
```javascript
socket.emit('room:join', { roomName: 'general' }, (response) => {
  // response: { success, data: { roomName, memberCount }, error? }
});
```

##### room:leave

Leave a room.

**Payload**:
```javascript
socket.emit('room:leave', { roomName: 'general' }, (response) => {
  // response: { success, error? }
});
```

##### message:send

Send message to room.

**Payload**:
```javascript
socket.emit('message:send', {
  roomName: 'general',
  content: 'Hello everyone!',
  threadId: null,
  metadata: { mentions: ['@user123'] }
}, (response) => {
  // response: { success, error? }
});
```

##### typing:start

Indicate user is typing.

**Payload**:
```javascript
socket.emit('typing:start', {
  roomName: 'general',
  threadId: null
});
```

##### typing:stop

Indicate user stopped typing.

**Payload**:
```javascript
socket.emit('typing:stop', {
  roomName: 'general',
  threadId: null
});
```

##### presence:update

Update user presence status.

**Payload**:
```javascript
socket.emit('presence:update', {
  status: 'away',
  customStatus: 'In a meeting'
});
```

##### ping

Send ping for latency measurement.

**Response**:
```javascript
socket.on('pong', (data) => {
  // data: { timestamp }
});
```

#### Server → Client Events

##### connected

Connection established.

**Data**:
```javascript
{
  socketId: 'abc123',
  serverTime: '2026-01-30T12:00:00.000Z',
  protocolVersion: '1.0'
}
```

##### user:joined

User joined room.

**Data**:
```javascript
{
  roomName: 'general',
  userId: 'user_456'
}
```

##### user:left

User left room.

**Data**:
```javascript
{
  roomName: 'general',
  userId: 'user_456'
}
```

##### message:new

New message received.

**Data**:
```javascript
{
  roomName: 'general',
  userId: 'user_456',
  content: 'Hello everyone!',
  threadId: null,
  timestamp: '2026-01-30T12:00:00.000Z',
  metadata: {}
}
```

##### typing:event

Typing indicator update.

**Data**:
```javascript
{
  roomName: 'general',
  threadId: null,
  users: [
    { userId: 'user_456', startedAt: '2026-01-30T12:00:00.000Z' }
  ]
}
```

##### presence:changed

User presence changed.

**Data**:
```javascript
{
  userId: 'user_456',
  status: 'away',
  customStatus: 'In a meeting',
  customEmoji: null
}
```

---

## File Processing API

File processing plugin handles file uploads, thumbnails, and virus scanning.

**Base URL**: `http://localhost:3104`
**Port**: 3104

### Health Endpoint

#### GET /health

**Response**:
```json
{
  "status": "ok",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Example**:
```bash
curl http://localhost:3104/health
```

### Jobs

#### POST /api/jobs

Create a new file processing job.

**Request Body**:
```json
{
  "fileKey": "uploads/image.jpg",
  "bucket": "my-bucket",
  "mimeType": "image/jpeg",
  "size": 1048576,
  "operations": ["thumbnail", "optimize", "scan"],
  "thumbnailSizes": ["100x100", "400x400"],
  "metadata": {
    "userId": "user_123",
    "uploadedFrom": "web"
  }
}
```

**Parameters**:
- `fileKey` (required) - File key/path in storage
- `bucket` (required) - Storage bucket name
- `mimeType` (required) - File MIME type
- `size` (required) - File size in bytes
- `operations` (required) - Array of operations: `thumbnail`, `optimize`, `scan`
- `thumbnailSizes` (optional) - Array of sizes (e.g., `["100x100", "400x400"]`)
- `metadata` (optional) - Additional metadata

**Response**:
```json
{
  "jobId": "job_abc123",
  "status": "pending",
  "estimatedDuration": 3000
}
```

**Example**:
```bash
curl -X POST http://localhost:3104/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "fileKey": "uploads/photo.jpg",
    "bucket": "my-bucket",
    "mimeType": "image/jpeg",
    "size": 2048576,
    "operations": ["thumbnail", "optimize"]
  }'
```

#### GET /api/jobs/:jobId

Get job status and results.

**Response**:
```json
{
  "job": {
    "id": "job_abc123",
    "file_key": "uploads/image.jpg",
    "bucket": "my-bucket",
    "mime_type": "image/jpeg",
    "file_size": 1048576,
    "status": "completed",
    "operations": ["thumbnail", "optimize", "scan"],
    "progress": 100,
    "error_message": null,
    "started_at": "2026-01-30T12:00:00.000Z",
    "completed_at": "2026-01-30T12:00:03.000Z",
    "created_at": "2026-01-30T12:00:00.000Z"
  },
  "thumbnails": [
    {
      "id": "thumb_123",
      "job_id": "job_abc123",
      "size": "100x100",
      "file_key": "uploads/image_100x100.jpg",
      "width": 100,
      "height": 100,
      "file_size": 5120,
      "mime_type": "image/jpeg",
      "created_at": "2026-01-30T12:00:02.000Z"
    },
    {
      "id": "thumb_124",
      "job_id": "job_abc123",
      "size": "400x400",
      "file_key": "uploads/image_400x400.jpg",
      "width": 400,
      "height": 400,
      "file_size": 45678,
      "mime_type": "image/jpeg",
      "created_at": "2026-01-30T12:00:02.500Z"
    }
  ],
  "metadata": {
    "id": "meta_123",
    "job_id": "job_abc123",
    "width": 1920,
    "height": 1080,
    "format": "jpeg",
    "exif": {
      "Make": "Canon",
      "Model": "EOS R5"
    },
    "created_at": "2026-01-30T12:00:01.000Z"
  },
  "scan": {
    "id": "scan_123",
    "job_id": "job_abc123",
    "status": "clean",
    "threats_found": 0,
    "threats": [],
    "scanned_at": "2026-01-30T12:00:01.500Z"
  }
}
```

**Example**:
```bash
curl http://localhost:3104/api/jobs/job_abc123
```

#### GET /api/jobs

List processing jobs.

**Query Parameters**:
- `status` (optional) - Filter by status: `pending`, `processing`, `completed`, `failed`
- `limit` (default: 50)
- `offset` (default: 0)

**Response**:
```json
{
  "jobs": [
    {
      "id": "job_abc123",
      "file_key": "uploads/image.jpg",
      "status": "completed",
      "progress": 100,
      "created_at": "2026-01-30T12:00:00.000Z",
      "completed_at": "2026-01-30T12:00:03.000Z"
    }
  ]
}
```

**Example**:
```bash
# All jobs
curl http://localhost:3104/api/jobs

# Failed jobs only
curl "http://localhost:3104/api/jobs?status=failed"
```

### Statistics

#### GET /api/stats

Get processing statistics.

**Response**:
```json
{
  "totalJobs": 1250,
  "jobsByStatus": {
    "pending": 5,
    "processing": 3,
    "completed": 1230,
    "failed": 12
  },
  "totalThumbnails": 2460,
  "totalScans": 1250,
  "threatsFound": 2,
  "avgProcessingTime": 2.8,
  "queueLength": 8
}
```

**Example**:
```bash
curl http://localhost:3104/api/stats
```

---

## Jobs API

Jobs plugin provides background job queue with BullMQ and BullBoard dashboard.

**Base URL**: `http://localhost:3105`
**Port**: 3105

### Health Endpoints

#### GET /health

**Response**:
```json
{
  "status": "ok",
  "plugin": "jobs",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

#### GET /ready

**Response**:
```json
{
  "ready": true,
  "plugin": "jobs",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

**Error Response** (503):
```json
{
  "ready": false,
  "plugin": "jobs",
  "error": "Redis unavailable",
  "timestamp": "2026-01-30T12:00:00.000Z"
}
```

### Dashboard

#### GET /dashboard

BullBoard web dashboard for monitoring queues.

**Access**: Open in browser: `http://localhost:3105/dashboard`

**Features**:
- View all queues
- Monitor job states (waiting, active, completed, failed)
- Retry failed jobs
- Delete jobs
- View job details and logs
- Real-time updates

### Statistics

#### GET /api/stats

Get queue statistics.

**Response**:
```json
{
  "totalJobs": 5600,
  "jobsByQueue": {
    "default": 3400,
    "high-priority": 1200,
    "low-priority": 1000
  },
  "jobsByStatus": {
    "waiting": 15,
    "active": 5,
    "completed": 5450,
    "failed": 130
  },
  "jobsByType": {
    "email": 2300,
    "export": 450,
    "import": 680,
    "notification": 2170
  },
  "avgProcessingTime": 1.25,
  "failureRate": 2.32,
  "throughput": 450
}
```

**Example**:
```bash
curl http://localhost:3105/api/stats
```

### Create Job

#### POST /api/jobs

Add a job to the queue.

**Request Body**:
```json
{
  "type": "send_email",
  "queue": "default",
  "payload": {
    "to": "user@example.com",
    "subject": "Welcome!",
    "body": "Thanks for signing up."
  },
  "options": {
    "priority": "high",
    "delay": 0,
    "maxRetries": 3,
    "retryDelay": 5000,
    "timeout": 60000,
    "metadata": {
      "userId": "user_123"
    },
    "tags": ["email", "onboarding"]
  }
}
```

**Parameters**:
- `type` (required) - Job type identifier
- `queue` (optional, default: "default") - Queue name: `default`, `high-priority`, `low-priority`
- `payload` (required) - Job payload data
- `options` (optional) - Job options:
  - `priority` (optional) - Priority: `low`, `normal`, `high`, `critical`
  - `delay` (optional) - Delay in milliseconds before processing
  - `maxRetries` (optional, default: 3) - Maximum retry attempts
  - `retryDelay` (optional, default: 5000) - Delay between retries (ms)
  - `timeout` (optional, default: 60000) - Job timeout (ms)
  - `metadata` (optional) - Additional metadata
  - `tags` (optional) - Array of tags for filtering

**Response**:
```json
{
  "success": true,
  "jobId": "123456",
  "queue": "default",
  "type": "send_email"
}
```

**Example**:
```bash
curl -X POST http://localhost:3105/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "send_email",
    "payload": {
      "to": "user@example.com",
      "subject": "Hello",
      "body": "Test message"
    },
    "options": {
      "priority": "high"
    }
  }'
```

### Get Job

#### GET /api/jobs/:id

Get job status and details.

**Response**:
```json
{
  "id": "123456",
  "bullmq_id": "123456",
  "queue_name": "default",
  "job_type": "send_email",
  "priority": "high",
  "status": "completed",
  "payload": {
    "to": "user@example.com",
    "subject": "Welcome!",
    "body": "Thanks for signing up."
  },
  "result": {
    "messageId": "msg_abc123",
    "status": "sent"
  },
  "error_message": null,
  "attempts_made": 1,
  "max_retries": 3,
  "retry_delay": 5000,
  "scheduled_for": null,
  "started_at": "2026-01-30T12:00:00.000Z",
  "completed_at": "2026-01-30T12:00:01.250Z",
  "failed_at": null,
  "created_at": "2026-01-30T11:59:59.000Z",
  "updated_at": "2026-01-30T12:00:01.250Z",
  "metadata": {
    "userId": "user_123"
  },
  "tags": ["email", "onboarding"]
}
```

**Error Response** (404):
```json
{
  "error": "Job not found"
}
```

**Example**:
```bash
curl http://localhost:3105/api/jobs/123456
```

---

## Notifications API

Notifications plugin handles multi-channel notifications (email, push, SMS).

**Base URL**: `http://localhost:3102`
**Port**: 3102

### Health Endpoint

#### GET /health

**Response**:
```json
{
  "status": "ok",
  "timestamp": "2026-01-30T12:00:00.000Z",
  "service": "notifications"
}
```

**Example**:
```bash
curl http://localhost:3102/health
```

### Send Notification

#### POST /api/notifications/send

Send a notification.

**Request Body**:
```json
{
  "user_id": "user_123",
  "channel": "email",
  "category": "transactional",
  "template": "welcome_email",
  "to": {
    "email": "user@example.com",
    "phone": "+15551234567",
    "push_token": "fcm_token_here"
  },
  "content": {
    "subject": "Welcome!",
    "body": "Thanks for signing up.",
    "html": "<h1>Welcome!</h1><p>Thanks for signing up.</p>"
  },
  "variables": {
    "name": "John Doe",
    "company": "Acme Inc"
  },
  "priority": "high",
  "scheduled_at": "2026-01-31T09:00:00.000Z",
  "metadata": {
    "campaign_id": "campaign_456"
  },
  "tags": ["onboarding", "welcome"]
}
```

**Parameters**:
- `user_id` (required) - User ID
- `channel` (required) - Channel: `email`, `push`, `sms`
- `category` (optional, default: "transactional") - Category: `transactional`, `marketing`, `system`
- `template` (optional) - Template name (if using template)
- `to` (required) - Recipient info:
  - `email` - Email address (for email channel)
  - `phone` - Phone number (for SMS channel)
  - `push_token` - Push notification token (for push channel)
- `content` (optional) - Message content (if not using template):
  - `subject` - Subject line
  - `body` - Plain text body
  - `html` - HTML body
- `variables` (optional) - Template variables
- `priority` (optional) - Priority: `low`, `normal`, `high`
- `scheduled_at` (optional) - ISO timestamp for scheduled delivery
- `metadata` (optional) - Additional metadata
- `tags` (optional) - Array of tags

**Response**:
```json
{
  "success": true,
  "notification_id": "notif_abc123",
  "message": "Notification queued for delivery"
}
```

**Error Response** (400):
```json
{
  "success": false,
  "error": "user_id and channel are required"
}
```

**Example**:
```bash
# Send email using template
curl -X POST http://localhost:3102/api/notifications/send \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_123",
    "channel": "email",
    "template": "welcome_email",
    "to": {"email": "user@example.com"},
    "variables": {"name": "John Doe"}
  }'

# Send immediate email with content
curl -X POST http://localhost:3102/api/notifications/send \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_123",
    "channel": "email",
    "to": {"email": "user@example.com"},
    "content": {
      "subject": "Alert",
      "body": "Your order has shipped!"
    },
    "priority": "high"
  }'
```

### Get Notification

#### GET /api/notifications/:id

Get notification status.

**Response**:
```json
{
  "notification": {
    "id": "notif_abc123",
    "user_id": "user_123",
    "channel": "email",
    "category": "transactional",
    "template_name": "welcome_email",
    "recipient_email": "user@example.com",
    "recipient_phone": null,
    "recipient_push_token": null,
    "subject": "Welcome to Acme!",
    "body_text": "Thanks for signing up.",
    "body_html": "<h1>Welcome!</h1>",
    "status": "delivered",
    "priority": "high",
    "scheduled_at": null,
    "sent_at": "2026-01-30T12:00:00.000Z",
    "delivered_at": "2026-01-30T12:00:01.500Z",
    "opened_at": "2026-01-30T12:05:30.000Z",
    "clicked_at": null,
    "bounced_at": null,
    "failed_at": null,
    "error_message": null,
    "provider": "resend",
    "provider_message_id": "msg_xyz789",
    "metadata": {
      "campaign_id": "campaign_456"
    },
    "tags": ["onboarding", "welcome"],
    "created_at": "2026-01-30T11:59:59.000Z",
    "updated_at": "2026-01-30T12:05:30.000Z"
  }
}
```

**Error Response** (404):
```json
{
  "error": "Notification not found"
}
```

**Example**:
```bash
curl http://localhost:3102/api/notifications/notif_abc123
```

### Templates

#### GET /api/templates

List all notification templates.

**Response**:
```json
{
  "templates": [
    {
      "id": "tmpl_123",
      "name": "welcome_email",
      "channel": "email",
      "category": "transactional",
      "subject": "Welcome to {{company}}!",
      "body_text": "Hi {{name}}, thanks for signing up!",
      "body_html": "<h1>Welcome {{name}}!</h1><p>Thanks for signing up.</p>",
      "variables": ["name", "company"],
      "active": true,
      "created_at": "2025-12-01T00:00:00.000Z",
      "updated_at": "2026-01-15T10:00:00.000Z"
    }
  ],
  "total": 15
}
```

**Example**:
```bash
curl http://localhost:3102/api/templates
```

#### GET /api/templates/:name

Get a specific template.

**Response**:
```json
{
  "template": {
    "id": "tmpl_123",
    "name": "welcome_email",
    "channel": "email",
    "category": "transactional",
    "subject": "Welcome to {{company}}!",
    "body_text": "Hi {{name}}, thanks for signing up!",
    "body_html": "<h1>Welcome {{name}}!</h1><p>Thanks for signing up.</p>",
    "variables": ["name", "company"],
    "active": true,
    "created_at": "2025-12-01T00:00:00.000Z",
    "updated_at": "2026-01-15T10:00:00.000Z"
  }
}
```

**Error Response** (404):
```json
{
  "error": "Template not found"
}
```

**Example**:
```bash
curl http://localhost:3102/api/templates/welcome_email
```

### Statistics

#### GET /api/stats/delivery

Get delivery statistics.

**Query Parameters**:
- `days` (default: 7) - Number of days to include

**Response**:
```json
{
  "stats": {
    "totalSent": 5600,
    "totalDelivered": 5450,
    "totalBounced": 45,
    "totalFailed": 105,
    "deliveryRate": 97.32,
    "bounceRate": 0.80,
    "failureRate": 1.88,
    "byChannel": {
      "email": {
        "sent": 4500,
        "delivered": 4380,
        "bounced": 30,
        "failed": 90
      },
      "push": {
        "sent": 900,
        "delivered": 870,
        "bounced": 15,
        "failed": 15
      },
      "sms": {
        "sent": 200,
        "delivered": 200,
        "bounced": 0,
        "failed": 0
      }
    },
    "byDay": [
      {
        "date": "2026-01-30",
        "sent": 850,
        "delivered": 830,
        "bounced": 5,
        "failed": 15
      }
    ]
  }
}
```

**Example**:
```bash
# Last 7 days
curl http://localhost:3102/api/stats/delivery

# Last 30 days
curl "http://localhost:3102/api/stats/delivery?days=30"
```

#### GET /api/stats/engagement

Get engagement statistics.

**Query Parameters**:
- `days` (default: 7) - Number of days to include

**Response**:
```json
{
  "metrics": {
    "totalOpens": 2300,
    "totalClicks": 890,
    "uniqueOpens": 1950,
    "uniqueClicks": 750,
    "openRate": 42.28,
    "clickRate": 16.30,
    "clickToOpenRate": 38.56,
    "byChannel": {
      "email": {
        "opens": 2100,
        "clicks": 850,
        "openRate": 47.73,
        "clickRate": 19.32
      },
      "push": {
        "opens": 200,
        "clicks": 40,
        "openRate": 22.99,
        "clickRate": 4.60
      }
    },
    "byTemplate": [
      {
        "template": "welcome_email",
        "sends": 450,
        "opens": 250,
        "clicks": 95,
        "openRate": 55.56,
        "clickRate": 21.11
      }
    ]
  }
}
```

**Example**:
```bash
curl "http://localhost:3102/api/stats/engagement?days=30"
```

### Webhook Endpoint

#### POST /webhooks/notifications

Receive delivery status webhooks from notification providers.

**Headers**:
- `X-Provider-Signature` (optional) - Provider signature
- `Content-Type: application/json`

**Response**:
```json
{
  "received": true
}
```

**Example**:
```bash
curl -X POST http://localhost:3102/webhooks/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "type": "delivery.succeeded",
    "notification_id": "notif_abc123",
    "timestamp": "2026-01-30T12:00:01.500Z"
  }'
```

---

## ID.me API

ID.me plugin handles OAuth authentication and identity verification.

**Base URL**: `http://localhost:3010`
**Port**: 3010

### Health Endpoint

#### GET /health

**Response**:
```json
{
  "status": "ok",
  "service": "idme-plugin"
}
```

**Example**:
```bash
curl http://localhost:3010/health
```

### OAuth Flow

#### GET /auth/idme

Start OAuth authorization flow.

**Behavior**: Redirects to ID.me authorization page

**Query Parameters**: None

**Example**:
```bash
# Open in browser
open http://localhost:3010/auth/idme

# Or use curl to get redirect URL
curl -I http://localhost:3010/auth/idme
# Returns: Location: https://api.id.me/oauth/authorize?...
```

#### GET /callback/idme

OAuth callback endpoint (called by ID.me after authorization).

**Query Parameters**:
- `code` - Authorization code
- `state` - State parameter for CSRF protection
- `error` (optional) - Error code if authorization failed

**Response** (Success):
```json
{
  "success": true,
  "profile": {
    "id": "idme_user_123",
    "email": "veteran@example.com",
    "first_name": "John",
    "last_name": "Doe",
    "verified": true,
    "verified_at": "2026-01-15T10:00:00.000Z"
  },
  "verification": {
    "groups": [
      {
        "name": "military",
        "verified": true,
        "verified_at": "2026-01-15T10:00:00.000Z"
      },
      {
        "name": "veteran",
        "verified": true,
        "verified_at": "2026-01-15T10:00:00.000Z"
      }
    ],
    "badges": [
      {
        "type": "military",
        "level": "gold"
      }
    ],
    "attributes": {
      "branch": "Army",
      "rank": "Captain",
      "service_start": "2010-05-15",
      "service_end": "2020-05-14"
    }
  },
  "tokens": {
    "expiresAt": "2026-01-30T13:00:00.000Z"
  }
}
```

**Error Response** (400):
```json
{
  "error": "OAuth authentication failed"
}
```

**Example**:
```
# This endpoint is called automatically by ID.me
# Not meant to be called directly
```

### Verification Status

#### GET /api/verifications/:userId

Get verification status for a user.

**Path Parameters**:
- `userId` - User ID

**Response**:
```json
{
  "id": "ver_abc123",
  "user_id": "user_123",
  "idme_uuid": "idme_user_123",
  "email": "veteran@example.com",
  "first_name": "John",
  "last_name": "Doe",
  "verified": true,
  "verified_at": "2026-01-15T10:00:00.000Z",
  "groups": [
    {
      "name": "military",
      "verified": true,
      "verified_at": "2026-01-15T10:00:00.000Z"
    },
    {
      "name": "veteran",
      "verified": true,
      "verified_at": "2026-01-15T10:00:00.000Z"
    }
  ],
  "badges": [
    {
      "type": "military",
      "level": "gold"
    }
  ],
  "attributes": {
    "branch": "Army",
    "rank": "Captain",
    "service_start": "2010-05-15",
    "service_end": "2020-05-14"
  },
  "created_at": "2026-01-15T10:00:00.000Z",
  "updated_at": "2026-01-15T10:00:00.000Z"
}
```

**Error Response** (404):
```json
{
  "error": "Verification not found"
}
```

**Example**:
```bash
curl http://localhost:3010/api/verifications/user_123
```

### Webhook Endpoint

#### POST /webhook/idme

Receive ID.me webhook events.

**Headers**:
- `X-IDme-Signature` (required) - Webhook signature
- `Content-Type: application/json`

**Request Body**:
```json
{
  "type": "verification.completed",
  "id": "evt_123",
  "user_id": "idme_user_123",
  "data": {
    "groups": [...],
    "verified_at": "2026-01-30T12:00:00.000Z"
  }
}
```

**Response**:
```json
{
  "received": true
}
```

**Error Responses**:
- 401: Missing or invalid signature
- 500: Processing failed

**Example**:
```bash
curl -X POST http://localhost:3010/webhook/idme \
  -H "X-IDme-Signature: abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "type": "verification.completed",
    "id": "evt_123",
    "user_id": "idme_user_123",
    "data": {...}
  }'
```

### Supported Verification Groups

ID.me verifies identity for 7 groups:

| Group | Description |
|-------|-------------|
| `military` | Active duty military personnel |
| `veteran` | Military veterans |
| `first_responder` | Police, fire, EMT |
| `teacher` | K-12 and university educators |
| `student` | College and university students |
| `healthcare` | Healthcare professionals |
| `government` | Government employees |

---

## Appendix

### Common HTTP Headers

All requests should include:
```
Content-Type: application/json
User-Agent: MyApp/1.0.0
```

Optional headers:
```
X-API-Key: your_api_key_here
X-Request-ID: unique_request_id
```

### Timestamp Format

All timestamps use ISO 8601 format with timezone:
```
2026-01-30T12:00:00.000Z
```

### Currency Format

Currency amounts are typically returned as:
- Strings with decimal precision: `"29.99"`
- Integers in smallest unit (cents): `2999`

Always check API documentation for specific field formats.

### Boolean Values

Boolean fields use standard JSON booleans:
```json
{
  "active": true,
  "deleted": false
}
```

### Null Values

Null values are explicitly included in responses:
```json
{
  "description": null,
  "phone": null
}
```

### Array Fields

Empty arrays are returned as `[]`, not `null`:
```json
{
  "tags": [],
  "items": []
}
```

---

## Support

For issues or questions:
- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Email: support@nself.org

---

**Last Updated**: January 30, 2026
**API Version**: 1.0.0
**License**: Source-Available
