# notify

Multi-channel notification service for nself.

## Overview

The `notify` plugin provides a notification delivery service with support for Email (SMTP) and Webhook (HMAC-signed) channels. It exposes HTTP endpoints for sending notifications, managing templates, and viewing delivery history.

## Installation

```bash
nself plugin install notify
```

## Configuration

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `PORT` | No | Server port (default: 3052) |
| `PLUGIN_INTERNAL_SECRET` | No | Internal API secret |
| `SMTP_HOST` | No | SMTP server hostname |
| `SMTP_PORT` | No | SMTP server port |
| `SMTP_USER` | No | SMTP username |
| `SMTP_PASSWORD` | No | SMTP password |
| `SMTP_FROM` | No | Default sender address |
| `WEBHOOK_HMAC_SECRET` | No | HMAC secret for webhook signing |

## Usage

```bash
# Start the notification server
nself plugin run notify server
```

## REST API

- `POST /notifications` — Send a notification
- `GET /notifications` — List delivery history
- `POST /templates` — Create a notification template
- `GET /templates` — List templates

## Database Tables

- `np_notify_notifications` — Notification delivery records
- `np_notify_templates` — Message templates

## License

MIT
