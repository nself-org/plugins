# notify

Multi-channel notification service for nself.

## Overview

The `notify` plugin provides a notification delivery service with support for Email (SMTP) and Webhook (HMAC-signed) channels. It exposes HTTP endpoints for sending notifications, managing reusable templates, and viewing delivery history.

The plugin is written in Go (per `plugin.json`) and ships as a single binary. It binds to `127.0.0.1` by default and is reverse-proxied through Nginx by `nself build`. For the older multi-channel implementation (Email + Push placeholder + SMS placeholder), see the separate `notifications` plugin; new projects should prefer `notify`.

## Installation

```bash
nself plugin install notify
```

No license key required. MIT-licensed.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `PORT` | No | `3052` | Server port (also `defaultPort` in plugin.json) |
| `PLUGIN_INTERNAL_SECRET` | No | — | Internal API secret for trusted callers |
| `SMTP_HOST` | No | — | SMTP server hostname (required for Email channel) |
| `SMTP_PORT` | No | `587` | SMTP server port |
| `SMTP_USER` | No | — | SMTP username |
| `SMTP_PASSWORD` | No | — | SMTP password |
| `SMTP_FROM` | No | — | Default sender address |
| `WEBHOOK_HMAC_SECRET` | No | — | HMAC secret for outbound webhook signing (required for Webhook channel) |

## Usage

```bash
# Start the notification server
nself plugin run notify server
```

The server accepts notification requests via REST and dispatches to the configured channel. Delivery records are persisted in `np_notify_notifications` for auditing.

## REST API

```
POST   /notifications     — Send a notification (Email or Webhook)
GET    /notifications     — List delivery history (paginated)
GET    /notifications/:id — Fetch a single notification by ID
POST   /templates         — Create a notification template
GET    /templates         — List templates
GET    /templates/:id     — Fetch a single template
DELETE /templates/:id     — Remove a template
GET    /health            — Health check (returns 200 OK)
```

### Send an email

```bash
curl -X POST http://localhost:3052/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "channel": "email",
    "to": "user@example.com",
    "subject": "Welcome",
    "body": "Thanks for signing up."
  }'
```

### Dispatch a signed webhook

```bash
curl -X POST http://localhost:3052/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "channel": "webhook",
    "url": "https://api.example.com/events",
    "payload": {"event": "user.created", "user_id": "123"}
  }'
```

## Database Tables

Two tables added to your Postgres database (prefix `np_notify_`):

- `np_notify_notifications` — Notification delivery records with status, timestamp, channel, target
- `np_notify_templates` — Reusable message templates with variable substitution

## Common Workflows

- **Transactional email**: send welcome / password-reset / receipt emails directly from app code.
- **Outbound webhooks**: sign payloads with HMAC and dispatch to integrating services.
- **Templated notifications**: define a template once, send many notifications with variable substitution.

## Troubleshooting

- **Email not sending**: verify `SMTP_HOST`, `SMTP_USER`, and `SMTP_PASSWORD` are set, then `curl http://localhost:3052/health`.
- **Webhook signature mismatch**: confirm `WEBHOOK_HMAC_SECRET` is set on both sender and receiver.
- **Delivery history grows large**: prune `np_notify_notifications` on a schedule using the `cron` plugin.

## License

MIT
