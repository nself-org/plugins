# LinkedIn Plugin

> LinkedIn publishing integration. **Free — MIT licensed.**

## Install

```bash
nself plugin install linkedin
```

No license key required.

## Description

LinkedIn publishing integration. OAuth 2.0 connection, post to LinkedIn feed with optional image attachments, post history, and Claw tool descriptor.

Category: `content`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `LINKEDIN_CLIENT_ID` | `(required)` | - |
| `LINKEDIN_CLIENT_SECRET` | `(required)` | - |
| `LINKEDIN_REDIRECT_URI` | `(required)` | - |
| `LINKEDIN_INTERNAL_SECRET` | `(required)` | - |
| `PORT` | `3722` | HTTP server port |
| `BIND_ADDRESS` | `-` | Bind address for the HTTP server |
| `NSELF_PLUGIN_LICENSE_KEY` | `-` | nSelf plugin license key |

## Ports

| Port | Purpose |
|------|---------|
| 3722 | LinkedIn service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_linkedin_tokens`
- `np_linkedin_posts`

## REST API

```
GET    /health
GET    /internal/tools
DELETE /linkedin/auth
GET    /linkedin/auth/callback
GET    /linkedin/auth/start
GET    /linkedin/posts
POST   /linkedin/publish
GET    /linkedin/status
```

## Examples

### Health check

```bash
curl http://localhost:3722/health
```

## Source

[`plugins/linkedin/`](https://github.com/nself-org/plugins/tree/main/linkedin)

Manifest: [`plugins/linkedin/plugin.json`](https://github.com/nself-org/plugins/tree/main/linkedin/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
