# plugin-sms

SMS messaging plugin for nSelf. Sends, tracks, and manages opt-outs via the Twilio API.

**Port:** 9009 | **Bundle:** ɳChat | **License:** Pro (requires_license=true)

---

## Overview

plugin-sms provides Twilio-backed SMS delivery for nSelf tenants. It enforces E.164 phone
number format, tracks per-account rate limits, maintains opt-out lists, and records all
message history in Postgres.

Key properties:
- All phone numbers validated as E.164 before any Twilio call
- Per-account rate limiting (default 10 SMS/minute)
- Opt-out list checked before every send
- All DB rows scoped by `source_account_id` (Multi-App Isolation Convention)
- Twilio credentials come from env only — never from request parameters

---

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness check (pings DB) |
| POST | `/sms/send` | Send an SMS message |
| GET | `/sms/messages` | List sent messages for account |
| POST | `/sms/opt-out` | Add number to opt-out list |
| DELETE | `/sms/opt-out/{number}` | Remove number from opt-out list |
| GET | `/sms/opt-outs` | List all opted-out numbers |

### POST /sms/send

Request body:
```json
{ "to": "+14155552671", "body": "Your verification code is 123456" }
```

Response (202):
```json
{ "id": "<uuid>", "status": "sent", "provider_sid": "SM..." }
```

Errors:
- `400` — missing fields or invalid E.164 format
- `403` — recipient has opted out
- `429` — rate limit exceeded

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NSELF_DB_URL` | Yes | — | PostgreSQL connection string |
| `TWILIO_ACCOUNT_SID` | Yes | — | Twilio Account SID |
| `TWILIO_AUTH_TOKEN` | Yes | — | Twilio Auth Token |
| `TWILIO_FROM_NUMBER` | Yes | — | Sending number in E.164 format |
| `SMS_PLUGIN_PORT` | No | `9009` | HTTP server port |
| `SMS_RATE_LIMIT_PER_MIN` | No | `10` | Max SMS per account per minute |
| `NSELF_LICENSE_KEY` | Yes | — | nSelf license key |

---

## Twilio Setup

1. Create a Twilio account at https://twilio.com
2. Purchase a phone number with SMS capability
3. Copy Account SID and Auth Token from the Twilio console
4. Set all three environment variables (`TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM_NUMBER`)

The plugin connects to `api.twilio.com` only. SSRF protection blocks all other outbound hosts.

---

## Database Tables

| Table | Purpose |
|-------|---------|
| `np_sms_messages` | Message history with status tracking |
| `np_sms_opt_outs` | Per-account opt-out list |

Both tables include `source_account_id TEXT NOT NULL DEFAULT 'primary'` with Hasura row filters
scoping all queries to the requesting account.

---

## Quickstart

```bash
nself plugin install sms

# Set required env vars
nself env set TWILIO_ACCOUNT_SID=ACxxx
nself env set TWILIO_AUTH_TOKEN=xxx
nself env set TWILIO_FROM_NUMBER=+14155550100

# Verify
curl http://localhost:9009/health
# {"status":"ok"}

# Send a message
curl -X POST http://localhost:9009/sms/send \
  -H "Content-Type: application/json" \
  -H "X-Hasura-Source-Account-Id: my-account" \
  -d '{"to":"+14155552671","body":"Hello from nSelf"}'
```

---

## Security Notes

- SSRF guard: only outbound calls to `api.twilio.com` are permitted
- E.164 validation runs before any DB write or Twilio call
- Twilio credentials are read from env at startup; never logged or passed through requests
- All rows are tenant-scoped via `source_account_id`
