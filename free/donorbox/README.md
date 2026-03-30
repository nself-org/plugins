# donorbox

Donorbox donation data sync with webhook handling.

## Overview

The `donorbox` plugin syncs campaigns, donors, donations, subscription plans, and tickets from the Donorbox API into your PostgreSQL database. Real-time webhook handling keeps data current as new donations arrive.

## Installation

```bash
nself plugin install donorbox
```

## Configuration

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `DONORBOX_API_KEY` | Yes | Donorbox API key |
| `DONORBOX_EMAIL` | Yes | Donorbox account email |
| `PORT` | No | Server port (default: 3074) |
| `DONORBOX_WEBHOOK_SECRET` | No | Donorbox webhook signing secret |
| `PLUGIN_INTERNAL_SECRET` | No | Internal API secret |

## Usage

```bash
# Sync all Donorbox data to database
nself plugin run donorbox sync

# Re-sync recent data to catch gaps
nself plugin run donorbox reconcile

# Start the Donorbox plugin server
nself plugin run donorbox server

# Show sync status and statistics
nself plugin run donorbox status
```

## Database Tables

- `np_donorbox_campaigns` — Fundraising campaigns
- `np_donorbox_donors` — Donor records
- `np_donorbox_donations` — Individual donations
- `np_donorbox_plans` — Subscription plans
- `np_donorbox_events` — Campaign events
- `np_donorbox_tickets` — Event tickets
- `np_donorbox_webhook_events` — Incoming webhook log

## License

MIT
