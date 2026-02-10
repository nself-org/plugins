# Donorbox Plugin

Complete Donorbox donation data integration for nself. Syncs campaigns, donors, donations, recurring plans, events, and tickets to PostgreSQL with webhook support and cross-plugin references to Stripe and PayPal.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Cross-Plugin Integration](#cross-plugin-integration)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Donorbox plugin provides complete synchronization of your Donorbox account data to a local PostgreSQL database. It supports:

- **7 Database Tables** - Campaigns, donors, donations, plans, events, tickets, webhook events
- **1 Webhook Event** - `donation.created` with HMAC-SHA256 verification
- **5 Analytics Views** - Pre-built SQL views for fundraising metrics
- **Full REST API** - Query synced data via HTTP endpoints
- **CLI Interface** - Manage everything from the command line
- **Multi-Account Support** - Sync multiple Donorbox accounts into one database
- **Cross-Plugin References** - `stripe_charge_id` and `paypal_transaction_id` on donations

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Campaigns | Fundraising campaigns | `donorbox_campaigns` |
| Donors | Donor profiles | `donorbox_donors` |
| Donations | Individual donations | `donorbox_donations` |
| Plans | Recurring donation plans | `donorbox_plans` |
| Events | Donorbox events | `donorbox_events` |
| Tickets | Event tickets | `donorbox_tickets` |
| Webhook Events | Raw event log | `donorbox_webhook_events` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install donorbox

# Configure environment
echo "DONORBOX_EMAIL=admin@charity.org" >> .env
echo "DONORBOX_API_KEY=your_api_key" >> .env
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env

# Sync all data
nself plugin donorbox sync

# Start webhook server
nself plugin donorbox server --port 3005
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `DONORBOX_EMAIL` | Yes* | - | Donorbox account email |
| `DONORBOX_API_KEY` | Yes* | - | Donorbox API key |
| `DONORBOX_EMAILS` | No | - | Comma-separated emails for multi-account sync |
| `DONORBOX_API_KEYS` | No | - | Comma-separated API keys matching `DONORBOX_EMAILS` |
| `DONORBOX_ACCOUNT_LABELS` | No | - | Comma-separated labels matching `DONORBOX_EMAILS` |
| `DONORBOX_WEBHOOK_SECRET` | No | - | Webhook HMAC-SHA256 signing secret |
| `DONORBOX_WEBHOOK_SECRETS` | No | - | Comma-separated secrets for multi-account |
| `DONORBOX_SYNC_INTERVAL` | No | `3600` | Sync interval in seconds |
| `PORT` | No | `3005` | HTTP server port |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

\* `DONORBOX_EMAIL` and `DONORBOX_API_KEY` are required when `DONORBOX_EMAILS` is not set.

### API Key Setup

1. Log in to [Donorbox](https://donorbox.org)
2. Go to **Account Settings** > **API & Webhooks**
3. Copy your API key
4. The API uses Basic HTTP auth (`email:api_key`)

### Rate Limiting

Donorbox limits API requests to **60 per minute**. The plugin enforces a 1 request/second rate limit to stay within this budget.

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Donorbox API
DONORBOX_EMAIL=admin@mycharity.org
DONORBOX_API_KEY=abc123def456...
DONORBOX_WEBHOOK_SECRET=whsec_xyz789...

# Server
PORT=3005
LOG_LEVEL=info
```

### Multi-Account Example

```bash
DONORBOX_EMAILS=admin@charity-a.org,admin@charity-b.org
DONORBOX_API_KEYS=key_charity_a,key_charity_b
DONORBOX_ACCOUNT_LABELS=charity-a,charity-b
DONORBOX_WEBHOOK_SECRETS=secret_a,secret_b
```

Each synced record stores its origin in `source_account_id`.

---

## CLI Commands

### Data Synchronization

```bash
# Full sync (all resources)
nself plugin donorbox sync

# Incremental sync (only recent data)
nself plugin donorbox sync --incremental

# Sync specific account only
nself plugin donorbox sync --account charity-a
```

### Reconciliation

```bash
# Re-sync recent data (default 7-day lookback)
nself plugin donorbox reconcile

# Custom lookback window
nself plugin donorbox reconcile --days 14

# Reconcile specific account
nself plugin donorbox reconcile --account charity-b
```

### Server Management

```bash
# Start HTTP server
nself plugin donorbox server

# Start on custom port
nself plugin donorbox server --port 3005
```

### Status

```bash
# Show sync status and statistics
nself plugin donorbox status
```

---

## REST API

### Base URL

```
http://localhost:3005
```

### Health & Status

```http
GET /health          # Basic liveness check
GET /ready           # Readiness check (verifies database)
GET /live            # Liveness check with sync info
GET /status          # Full status with account info and stats
```

### Sync

```http
POST /sync           # Trigger full data sync
POST /reconcile      # Reconcile recent data
```

Both endpoints accept optional JSON body:
```json
{
  "accounts": ["charity-a"]
}
```

### Webhooks

```http
POST /webhooks/donorbox    # Donorbox webhook receiver
```

Donorbox webhooks use HMAC-SHA256 signature verification via the `Donorbox-Signature` header.

### Data Queries

```http
GET /api/campaigns    # List campaigns (limit, offset)
GET /api/donors       # List donors (limit, offset)
GET /api/donations    # List donations (limit, offset, status)
GET /api/plans        # List recurring plans (limit, offset, status)
GET /api/stats        # Aggregated statistics
GET /api/events       # List webhook events (limit)
```

---

## Webhook Events

Donorbox supports one webhook event:

### donation.created

Triggered when a new donation is made. The webhook payload is verified using HMAC-SHA256 with the `Donorbox-Signature` header.

| Field | Description |
|-------|-------------|
| `id` | Donation ID |
| `amount` | Donation amount |
| `currency` | Currency code |
| `donor.email` | Donor email |
| `campaign.name` | Campaign name |
| `stripe_charge_id` | Associated Stripe charge (if applicable) |
| `paypal_transaction_id` | Associated PayPal transaction (if applicable) |

**Action**: Upserts the donation into `donorbox_donations` with cross-reference IDs.

### Webhook Setup

1. Go to Donorbox **Account Settings** > **API & Webhooks**
2. Add your webhook URL: `https://your-domain.com/webhooks/donorbox`
3. Copy the signing secret and set `DONORBOX_WEBHOOK_SECRET`

---

## Database Schema

All tables include `source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'` for multi-account support.

### donorbox_campaigns

```sql
CREATE TABLE donorbox_campaigns (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255),
    slug VARCHAR(255),
    currency VARCHAR(10) DEFAULT 'USD',
    goal_amount NUMERIC(20, 2),
    total_raised NUMERIC(20, 2) DEFAULT 0,
    donations_count INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### donorbox_donors

```sql
CREATE TABLE donorbox_donors (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    email VARCHAR(255),
    phone VARCHAR(50),
    address TEXT,
    city VARCHAR(255),
    state VARCHAR(100),
    zip_code VARCHAR(20),
    country VARCHAR(100),
    employer VARCHAR(255),
    donations_count INTEGER DEFAULT 0,
    last_donation_at TIMESTAMP WITH TIME ZONE,
    total NUMERIC(20, 2) DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### donorbox_donations

```sql
CREATE TABLE donorbox_donations (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    campaign_id INTEGER,
    campaign_name VARCHAR(255),
    donor_id INTEGER,
    donor_email VARCHAR(255),
    donor_name VARCHAR(255),
    amount NUMERIC(20, 2) DEFAULT 0,
    converted_amount NUMERIC(20, 2),
    converted_net_amount NUMERIC(20, 2),
    amount_refunded NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    donation_type VARCHAR(50),
    donation_date TIMESTAMP WITH TIME ZONE,
    processing_fee NUMERIC(20, 2),
    status VARCHAR(50),
    recurring BOOLEAN DEFAULT false,
    comment TEXT,
    designation VARCHAR(255),
    stripe_charge_id VARCHAR(255),       -- Cross-reference to stripe_charges.id
    paypal_transaction_id VARCHAR(255),  -- Cross-reference to paypal_transactions.id
    questions JSONB DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### donorbox_plans

```sql
CREATE TABLE donorbox_plans (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    campaign_id INTEGER,
    campaign_name VARCHAR(255),
    donor_id INTEGER,
    donor_email VARCHAR(255),
    type VARCHAR(50),
    amount NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    status VARCHAR(50),
    started_at TIMESTAMP WITH TIME ZONE,
    last_donation_date TIMESTAMP WITH TIME ZONE,
    next_donation_date TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### donorbox_events

```sql
CREATE TABLE donorbox_events (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255),
    slug VARCHAR(255),
    description TEXT,
    start_date TIMESTAMP WITH TIME ZONE,
    end_date TIMESTAMP WITH TIME ZONE,
    timezone VARCHAR(50),
    venue_name VARCHAR(255),
    address TEXT,
    city VARCHAR(255),
    state VARCHAR(100),
    country VARCHAR(100),
    zip_code VARCHAR(20),
    currency VARCHAR(10) DEFAULT 'USD',
    tickets_count INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### donorbox_tickets

```sql
CREATE TABLE donorbox_tickets (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    event_id INTEGER,
    event_name VARCHAR(255),
    donor_id INTEGER,
    donor_email VARCHAR(255),
    ticket_type VARCHAR(100),
    quantity INTEGER DEFAULT 0,
    amount NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    status VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### donorbox_webhook_events

```sql
CREATE TABLE donorbox_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(255),
    payload JSONB DEFAULT '{}',
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    processed BOOLEAN DEFAULT false,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Analytics Views

### donorbox_unified_donations

All donations with campaign and donor details, excluding refunded donations.

```sql
SELECT * FROM donorbox_unified_donations;
-- donation_id, source_account_id, campaign_id, campaign_name,
-- donor_id, donor_email, donor_name, amount, amount_refunded, net_amount,
-- currency, donation_type, donation_date, status, recurring,
-- stripe_charge_id, paypal_transaction_id, processing_fee
```

### donorbox_campaign_summary

Per-campaign totals and donor counts.

```sql
SELECT * FROM donorbox_campaign_summary;
-- campaign_id, name, source_account_id, goal_amount, total_raised,
-- donations_count, is_active, currency, unique_donors
```

### donorbox_daily_donations

Daily donation aggregation.

```sql
SELECT * FROM donorbox_daily_donations;
-- source_account_id, donation_day, currency, donation_count,
-- total_amount, net_amount
```

### donorbox_recurring_summary

Recurring plan statistics.

```sql
SELECT * FROM donorbox_recurring_summary;
-- source_account_id, status, currency, plan_count, total_recurring_amount
```

### donorbox_top_donors

Ranked donors by total giving.

```sql
SELECT * FROM donorbox_top_donors;
-- donor_id, source_account_id, email, name, total,
-- donations_count, last_donation_at
```

---

## Cross-Plugin Integration

Donorbox stores the underlying payment processor IDs on each donation, enabling joins across all three plugins (Stripe, PayPal, Donorbox).

### Join Donorbox Donations with Stripe Charges

```sql
SELECT
    dd.donor_name,
    dd.amount AS donorbox_amount,
    sc.amount / 100.0 AS stripe_amount,
    sc.status AS stripe_status,
    sc.payment_method_details
FROM donorbox_donations dd
JOIN stripe_charges sc ON dd.stripe_charge_id = sc.id
WHERE dd.stripe_charge_id IS NOT NULL;
```

### Join Donorbox Donations with PayPal Transactions

```sql
SELECT
    dd.donor_name,
    dd.amount AS donorbox_amount,
    pt.amount AS paypal_amount,
    pt.fee_amount AS paypal_fee
FROM donorbox_donations dd
JOIN paypal_transactions pt ON dd.paypal_transaction_id = pt.id
WHERE dd.paypal_transaction_id IS NOT NULL;
```

### Unified Giving Report (All Platforms)

```sql
-- Total giving across all three platforms
SELECT 'stripe' AS source, SUM(amount / 100.0) AS total
FROM stripe_charges WHERE status = 'succeeded'
UNION ALL
SELECT 'paypal', SUM(amount)
FROM paypal_transactions WHERE status = 'S' AND amount > 0
UNION ALL
SELECT 'donorbox', SUM(amount)
FROM donorbox_donations WHERE status != 'refunded';
```

---

## Troubleshooting

### "Unauthorized" or 401 Error

Verify your `DONORBOX_EMAIL` and `DONORBOX_API_KEY` are correct. The API uses Basic HTTP auth with `email:api_key`.

### Slow Sync

Donorbox's API rate limit is 60 requests/minute (1/sec). A full sync of large accounts will take time. The plugin respects this limit automatically.

### Webhook Signature Mismatch

Ensure `DONORBOX_WEBHOOK_SECRET` matches the secret configured in your Donorbox webhook settings. The plugin uses HMAC-SHA256 verification.

### Missing Cross-Reference IDs

`stripe_charge_id` and `paypal_transaction_id` are only populated when Donorbox includes them in the API response. These depend on the payment method the donor used.

### Debug Mode

```bash
LOG_LEVEL=debug nself plugin donorbox sync
```

---

*Last Updated: February 10, 2026*
*Plugin Version: 1.0.0*
*nself Version: 0.4.8+*
