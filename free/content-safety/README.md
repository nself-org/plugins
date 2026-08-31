# ɳSelf Content Safety

Trust-and-safety backend plugin for nSelf. Provides evidence collection, legal
holds, spam detection, raid protection, and abuse scoring for self-hosted nSelf
deployments. Pro (license-gated) plugin on port `3213`.

## What it does

Content Safety is the moderation and abuse-defense service for an nSelf backend.
It is distinct from `nself-csam` (PhotoDNA hashing) and from the always-free core
hardening that ships with `nself install`. It exposes five capability groups:

| Group | Purpose |
|-------|---------|
| Trust-safety evidence | Capture, list, and export moderation evidence records and exports |
| Legal holds | Create and list legal holds that preserve evidence under retention |
| Spam detection | Analyze content, manage spam rules, configs, and rate limits |
| Raid protection | Track raid events, raid status, and lockdown state |
| Abuse scoring | Register and update per-actor trust/abuse scores |

## Filter modes & configuration

Spam analysis runs against a configurable rule set. Each deployment manages its
own spam configuration and rate-limit thresholds through the API, so operators
choose how aggressive filtering is per source account:

- **Rule-based filtering** — operator-defined spam rules (`POST /api/v1/spam/rules`).
- **Rate limiting** — per-action thresholds (`POST /api/v1/spam/rate-limits`).
- **Adaptive raid mode** — lockdowns triggered by raid events for burst defense.
- **Abuse trust scoring** — per-actor scores that downstream apps can gate on.

All records are isolated per `source_account_id` (Multi-App Isolation), set via
the `X-Hasura-Source-Account-Id` header, so independent apps in one deployment
never see each other's evidence or rules.

## Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `CONTENT_SAFETY_PLUGIN_PORT` | `3213` | HTTP listen port |
| `CONTENT_SAFETY_PLUGIN_HOST` | `0.0.0.0` | Listen host |
| `DATABASE_URL` | — | Postgres connection string |
| `CONTENT_SAFETY_API_KEY` | — | Optional `X-API-Key` gate for the API |

## API

Health: `GET /health`, `GET /ready`. All business endpoints live under
`/api/v1`:

- `POST|GET /evidence`, `POST|GET /evidence/exports`
- `POST|GET /legal-holds`
- `GET /trust-safety/stats`
- `POST /spam/analyze`, `GET|PUT /spam/config`
- `GET|POST /spam/rules`, `DELETE /spam/rules/{id}`
- `GET|POST /spam/rate-limits`, `DELETE /spam/rate-limits/{id}`
- `GET|POST /raid/status`, `PUT /raid/status`
- `GET|POST /raid/lockdown`, `DELETE /raid/lockdown/{id}`
- `GET|POST|PUT /abuse/trust`

## Install

```bash
nself plugin install content-safety
```

## License

Pro plugin — `requires_license: true`. **Rationale (Security-Always-Free
doctrine):** Core security hardening (RLS, rate limits on the platform itself,
MFA throttle, SSRF guard, JWT rotation, WAF basics, audit logs, TLS) ships free
and automatic. Content Safety is a *platform moderation product* — operator-facing
trust-safety tooling (evidence retention, legal holds, configurable spam rules,
raid lockdowns, abuse scoring), not core deployment hardening. It is therefore a
licensed Pro feature, consistent with the doctrine. See
`.claude/docs/doctrines/security-always-free.md`.

## Development

```bash
go test ./...
go build ./cmd/
```
