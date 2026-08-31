# Notify Plugin

> Multi-channel notification service: email over SMTP and HMAC-signed outbound webhooks. **Free — MIT licensed.**

## Install

```bash
nself plugin install notify
```

No license key required.

## Description

Multi-channel notification service. Channels: Email (SMTP), Webhook (HMAC-signed). HTTP endpoints for sending notifications, managing templates, and viewing delivery history.

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `communication`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | *(required)* | Required. |
| `PORT` | *(see plugin.json)* | Optional. |
| `PLUGIN_INTERNAL_SECRET` | — | Optional. |
| `SMTP_HOST` | *(see plugin.json)* | Optional. |
| `SMTP_PORT` | *(see plugin.json)* | Optional. |
| `SMTP_USER` | *(see plugin.json)* | Optional. |
| `SMTP_PASSWORD` | — | Optional. |
| `SMTP_FROM` | *(see plugin.json)* | Optional. |
| `WEBHOOK_HMAC_SECRET` | — | Optional. |

## Ports

| Port | Purpose |
|------|---------|
| 3052 | Notify service |

## Database Schema

2 table(s) added to your Postgres database (prefix: `np_notify_`):

- `np_notify_notifications`
- `np_notify_templates`

## REST API

```
POST /send                — Send a notification (email or webhook channel)
GET  /templates           — List notification templates
POST /templates           — Create/update a template
GET  /history             — Delivery history
GET  /health              — Health check
```

## Nginx Routes

| Route | Target |
|-------|--------|
| `/notify/` | Notify plugin REST API |

## Examples

### Check health

```bash
curl http://localhost:3052/health
```

## Source

[`plugins/notify/`](https://github.com/nself-org/plugins/tree/main/notify)

Manifest: [`plugins/notify/plugin.json`](https://github.com/nself-org/plugins/tree/main/notify/plugin.json)

## See Also

- [[Push]] — APNs + FCM push relay
- [[Webhooks]] — outbound webhook delivery

← [[Home]] →
