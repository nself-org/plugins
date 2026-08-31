# Push Plugin

> Relays push notifications to iOS (APNs) and Android (FCM) with retry and delivery tracking. **Free — MIT licensed.**

## Install

```bash
nself plugin install push
```

No license key required.

## Description

APNs + FCM push notification relay. Hasura event-trigger fan-out, delivery state tracking, exponential backoff retry. Handles iOS (Apple Push Notification service) and Android (Firebase Cloud Messaging v1 API).

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `communication`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | *(required)* | Required. |
| `REDIS_URL` | *(required)* | Required. |
| `PORT` | *(see plugin.json)* | Optional. |
| `PUSH_APNS_TEAM_ID` | *(see plugin.json)* | Optional. |
| `PUSH_APNS_KEY_ID` | — | Optional. |
| `PUSH_APNS_KEY_PEM` | — | Optional. |
| `PUSH_APNS_BUNDLE_ID` | *(see plugin.json)* | Optional. |
| `PUSH_FCM_PROJECT_ID` | *(see plugin.json)* | Optional. |
| `PUSH_FCM_SERVICE_ACCOUNT_JSON` | *(see plugin.json)* | Optional. |
| `PUSH_RETRY_MAX_ATTEMPTS` | *(see plugin.json)* | Optional. |
| `PUSH_RETRY_BACKOFF_BASE_MS` | *(see plugin.json)* | Optional. |

## Ports

| Port | Purpose |
|------|---------|
| 3053 | Push service |

## Database Schema

2 table(s) added to your Postgres database (prefix: `np_push_`):

- `np_push_outbox`
- `np_push_devices`

## REST API

```
POST /devices             — Register a device token
DELETE /devices/{id}      — Remove a device token
POST /send                — Queue a push notification
GET  /outbox              — Delivery state for queued pushes
GET  /health              — Health check
```

## Nginx Routes

| Route | Target |
|-------|--------|
| `/push/` | Push plugin REST API |

## Examples

### Check health

```bash
curl http://localhost:3053/health
```

## Source

[`plugins/push/`](https://github.com/nself-org/plugins/tree/main/push)

Manifest: [`plugins/push/plugin.json`](https://github.com/nself-org/plugins/tree/main/push/plugin.json)

## See Also

- [[Notify]] — multi-channel notification service
- [[Webhooks]] — outbound webhook delivery

← [[Home]] →
