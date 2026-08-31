# Post Plugin

> Multi-platform content publishing for WordPress, Ghost, Twitter/X, LinkedIn, Telegram, Dev.to, and Hashnode with optional scheduling. **Pro plugin — requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Any bundle | $0.99/mo | $9.99/yr | If in bundle |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes |

**Minimum tier:** Pro (this is a `tier: max` plugin per F07-PRICING-TIERS, Basic does not unlock).

## Bundle membership

This plugin is currently sold via tier subscription only (Pro and up) and via the **ɳSelf+** super-bundle ($49.99/yr). PPI flagged `post` as a candidate for the ɳClaw bundle's "publishing tools" expansion; F06-BUNDLE-INVENTORY will reflect the final mapping.

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install post
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

The post plugin is the publishing back-end for nSelf-powered content workflows. Add an account for each platform, queue or schedule a post, and the plugin dispatches to the right API. WordPress and Ghost dispatchers ship today; X, LinkedIn, Telegram, Dev.to, and Hashnode dispatchers are routed but return `501 Not Implemented` until each lands.

A scheduler polls the queue every minute (configurable via `schedulerPollSeconds`) and dispatches posts whose `publish_at` has arrived. All credential material is encrypted at rest using the AES key supplied via `POST_ENCRYPTION_KEY`. ɳClaw can drive the plugin via its tool descriptor, letting users say "schedule this for tomorrow morning" and have the assistant call into `post` directly.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `POST_INTERNAL_SECRET` | Yes | — | Shared secret for inter-plugin calls |
| `POST_ENCRYPTION_KEY` | Yes | — | AES-256 key for encrypting platform credentials |
| `PORT` | No | `3129` | Listen port |
| `BIND_ADDRESS` | No | `127.0.0.1` | Bind address |
| `NSELF_PLUGIN_LICENSE_KEY` | No | — | License key (read by the plugin loader) |

Scheduler tunables live in `plugin.json` `config` (`schedulerPollSeconds`, `schedulerBatchSize`).

## Ports

- Default port: `3129` (override via `PORT`)
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx.

## Database Schema

Tables created (prefix `np_post_`):

- `np_post_accounts`: one row per connected platform account, encrypted credentials
- `np_post_queue`: queued / scheduled / sent / failed posts with status + retry metadata

Both tables use `source_account_id` for multi-app isolation.

## REST API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/accounts` | List connected platform accounts |
| POST | `/accounts` | Add a platform account (credentials encrypted) |
| DELETE | `/accounts/:id` | Remove an account |
| POST | `/posts` | Queue a post (`publish_at` optional for scheduling) |
| GET | `/posts` | List queued + sent posts |
| GET | `/posts/:id` | Get one post with status + last error |
| POST | `/posts/:id/cancel` | Cancel a queued post |

OAuth callback endpoints (one per platform that needs OAuth) are registered automatically.

## Examples

Connect a WordPress account:

```bash
curl -X POST https://api.example.com/post/accounts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "platform": "wordpress",
    "site_url": "https://blog.example.com",
    "username": "editor",
    "app_password": "xxxx-xxxx-xxxx"
  }'
```

Queue a post for tomorrow morning:

```bash
curl -X POST https://api.example.com/post/posts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "account_id": "acc_xxx",
    "title": "Plugin update",
    "body_markdown": "## What changed...",
    "publish_at": "2026-04-18T09:00:00Z"
  }'
```

Dispatch immediately:

```bash
curl -X POST https://api.example.com/post/posts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"account_id":"acc_xxx","title":"Now","body_markdown":"..."}'
```

## Source

Source-available (license required to run): [`plugins-pro/paid/post/`](https://github.com/nself-org/plugins-pro/tree/main/paid/post)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- `linkedin` plugin, direct LinkedIn integration (subset of `post`)
- `mux` plugin, webhook + email pipeline that can trigger posts
- `claw` plugin, uses `post` as a tool when ɳClaw is asked to publish
- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the 7-tier pricing matrix
