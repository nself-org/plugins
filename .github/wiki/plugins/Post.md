# Post Plugin

> Multi-platform content publishing. **Free — MIT licensed.**

## Install

```bash
nself plugin install post
```

No license key required.

## Description

Multi-platform content publishing. Publish to WordPress, Ghost, Twitter/X, LinkedIn, Telegram channels, Dev.to, and Hashnode with optional scheduling.

Category: `content`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `POST_INTERNAL_SECRET` | `(required)` | - |
| `POST_ENCRYPTION_KEY` | `(required)` | - |
| `PORT` | `3129` | HTTP server port |
| `BIND_ADDRESS` | `-` | Bind address for the HTTP server |
| `NSELF_PLUGIN_LICENSE_KEY` | `-` | nSelf plugin license key |

## Ports

| Port | Purpose |
|------|---------|
| 3129 | Post service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_post_accounts`
- `np_post_queue`

## REST API

```
GET    /health
GET    /post/accounts
POST   /post/accounts
DELETE /post/accounts/{id}
POST   /post/publish
GET    /post/queue
```

## Examples

### Health check

```bash
curl http://localhost:3129/health
```

## Source

[`plugins/post/`](https://github.com/nself-org/plugins/tree/main/post)

Manifest: [`plugins/post/plugin.json`](https://github.com/nself-org/plugins/tree/main/post/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
