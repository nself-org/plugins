# LinkedIn Plugin

> LinkedIn publishing integration. OAuth 2.0 connection, post to your LinkedIn feed with optional image attachments, post history, and ɳClaw tool descriptor. **Pro plugin — requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Any bundle | $0.99/mo | $9.99/yr | If in bundle |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes |

**Minimum tier:** Basic (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

This plugin is currently sold via tier subscription only (Basic and up) and via the **ɳSelf+** super-bundle ($49.99/yr). The broader `post` plugin (Pro tier) covers LinkedIn plus six other platforms; pick `linkedin` if LinkedIn is the only platform you need.

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install linkedin
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

You also need a LinkedIn OAuth app, register one at https://www.linkedin.com/developers/ to obtain client ID, client secret, and redirect URI.

## Description

The linkedin plugin handles the OAuth 2.0 flow for connecting a LinkedIn account, stores the refresh token securely, and exposes a single endpoint for publishing a post. Posts can include text plus one image; the plugin uploads the image to LinkedIn's asset endpoint, then submits the post with the asset URN.

Each post and its result is logged in `np_linkedin_posts` for audit and retry. ɳClaw consumes the plugin via a tool descriptor: when a user says "share this on my LinkedIn", the assistant calls into the plugin's publish endpoint without needing platform-specific code in `claw` itself.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `LINKEDIN_CLIENT_ID` | Yes | — | OAuth 2.0 client ID from your LinkedIn app |
| `LINKEDIN_CLIENT_SECRET` | Yes | — | OAuth 2.0 client secret |
| `LINKEDIN_REDIRECT_URI` | Yes | — | Callback URL registered with LinkedIn |
| `LINKEDIN_INTERNAL_SECRET` | Yes | — | Shared secret for inter-plugin calls |
| `PORT` | No | `3722` | Listen port |
| `BIND_ADDRESS` | No | `127.0.0.1` | Bind address |
| `NSELF_PLUGIN_LICENSE_KEY` | No | — | License key (read by the plugin loader) |

Reference vault credentials. Never hardcode `LINKEDIN_CLIENT_SECRET`.

## Ports

- Default port: `3722` (override via `PORT`)
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx.

## Database Schema

Tables created (prefix `np_linkedin_`):

- `np_linkedin_tokens`: connected accounts with refresh tokens (encrypted)
- `np_linkedin_posts`: published posts with status, asset URNs, last error

Both tables use `source_account_id` for multi-app isolation.

## REST API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/oauth/authorize` | Start OAuth flow (returns redirect URL) |
| GET | `/oauth/callback` | OAuth callback — exchanges code for tokens |
| GET | `/accounts` | List connected accounts for the active user |
| DELETE | `/accounts/:id` | Disconnect an account |
| POST | `/posts` | Publish a post (`text`, optional `image_url`) |
| GET | `/posts` | List published posts |
| GET | `/posts/:id` | Get one post |

## Examples

Start the OAuth flow:

```bash
curl https://api.example.com/linkedin/oauth/authorize \
  -H "Authorization: Bearer $TOKEN"
# Returns: { "redirect_url": "https://www.linkedin.com/oauth/v2/authorization?..." }
```

Open the returned URL in a browser, sign in, approve the scopes, and LinkedIn redirects to `LINKEDIN_REDIRECT_URI` with a code that the callback exchanges for tokens.

Publish a text post:

```bash
curl -X POST https://api.example.com/linkedin/posts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text":"Shipped a new feature today."}'
```

Publish a post with an image:

```bash
curl -X POST https://api.example.com/linkedin/posts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text":"Read the blog post.","image_url":"https://blog.example.com/cover.png"}'
```

## Source

Source-available (license required to run): [`plugins-pro/paid/linkedin/`](https://github.com/nself-org/plugins-pro/tree/main/paid/linkedin)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- `post` plugin, multi-platform publisher (covers LinkedIn plus 6 other platforms)
- `claw` plugin, uses linkedin as a tool descriptor for assistant-driven posting
- `notify` plugin, alert on post failures
- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the 7-tier pricing matrix
