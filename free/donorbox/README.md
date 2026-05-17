# donorbox

Donorbox donation data sync with real-time webhook handling and full campaign/donor/subscription coverage.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install donorbox
nself build
nself start
```

## Overview

The `donorbox` plugin syncs campaigns, donors, donations, subscription plans, events, and tickets from the Donorbox API into your PostgreSQL database. Webhook handling keeps data current as donations arrive in real time, without waiting for the next scheduled sync.

Initial sync imports all historical data. Subsequent syncs are incremental. The `reconcile` command does a bounded re-sync of recent records to catch any events missed due to webhook failures.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `DONORBOX_API_KEY` | Yes | — | Donorbox API key (from Donorbox dashboard → API & Zapier → API Key) |
| `DONORBOX_EMAIL` | Yes | — | Donorbox account email (used as HTTP Basic auth username with API key as password) |
| `PORT` | No | `3074` | HTTP server port |
| `DONORBOX_WEBHOOK_SECRET` | No | — | Donorbox webhook signing secret for payload verification |
| `PLUGIN_INTERNAL_SECRET` | No | — | Shared secret for `X-Plugin-Secret` header authentication |
| `DONORBOX_SYNC_INTERVAL` | No | `3600` | Seconds between scheduled full syncs (default: 1 hour) |
| `DONORBOX_RETRY_ATTEMPTS` | No | `3` | Retry count for failed API calls |
| `DONORBOX_RETRY_DELAY` | No | `1000` | Delay in milliseconds between retries |

## Webhook Setup

Register your nSelf instance as a Donorbox webhook endpoint:

1. In Donorbox dashboard, go to **Settings → Integrations → Webhooks**
2. Set the URL to `https://your-domain.com/webhooks/donorbox`
3. Copy the webhook secret and set `DONORBOX_WEBHOOK_SECRET` in your `.env.secrets`

The plugin verifies HMAC signatures on all incoming webhook payloads.

## HTTP API

All endpoints require the `X-Plugin-Secret` header.

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |
| `POST` | `/webhooks/donorbox` | Donorbox webhook receiver (no auth required — uses HMAC verification) |
| `POST` | `/sync` | Trigger a full data sync |
| `POST` | `/reconcile` | Re-sync recent records to catch missed webhooks |
| `GET` | `/status` | Show sync status, last sync time, record counts |

## Database Tables

| Table | Purpose |
|---|---|
| `np_donorbox_campaigns` | Fundraising campaigns — goal, currency, start/end dates, totals |
| `np_donorbox_donors` | Donor records — name, email, phone, address, donor since date |
| `np_donorbox_donations` | Individual donations — amount, currency, campaign, donor, payment method, status |
| `np_donorbox_plans` | Recurring subscription plans — interval, amount, status |
| `np_donorbox_events` | Campaign events — name, date, location, capacity |
| `np_donorbox_tickets` | Event tickets — holder name, email, ticket type, check-in status |
| `np_donorbox_webhook_events` | Incoming webhook log — event type, payload, processed timestamp |

## Usage

```bash
# Sync all Donorbox data to database
nself plugin run donorbox sync

# Re-sync recent data to catch gaps from missed webhooks
nself plugin run donorbox reconcile

# Start the Donorbox plugin server
nself plugin run donorbox server

# Show sync status and statistics
nself plugin run donorbox status
```

## Webhook Events

| Event | Description |
|---|---|
| `donation.created` | Sync new donation to database |

Additional events (updates, refunds, subscription changes) are synced on the next scheduled poll or via `reconcile`.

## Multi-App Isolation

Each app in a multi-app deployment stores Donorbox data scoped by `source_account_id`. Multiple apps can connect to different Donorbox accounts without data leakage.

## Port

The plugin binds to `127.0.0.1:3074`. It is never exposed directly — access via Nginx proxy.

## See also

- [plugin-webhooks](plugin-webhooks.md) — outbound webhook delivery for Donorbox events
- [plugin-notify](plugin-notify.md) — notify your team when large donations arrive
- [nSelf CLI: nself plugin](cmd-plugin.md) — plugin management
