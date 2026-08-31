# nself-saas-gateway

Unified tenant-scoped REST facade for the ɳSentry SaaS, served at
`api.sentry.nself.org`. It is the single public API the `nself sentry` CLI,
the MCP sentry tools, and the sentry.nself.org SPA talk to. Port **3848**
(F10 registry: `nself-sentry-api`).

## What it does

- Authenticates every `/v1` request through the shared `saas` layer:
  `Authorization: Bearer nsk_<key>` API keys, nself-auth HS256 JWTs with a
  tenant claim, or the nginx-injected `X-Hasura-Tenant-Id` header. Cloud
  mode is fail-closed; free-tier keys are clamped read-only.
- Maps the v1 contract (see `cli/internal/sentryapi`) onto the bundle
  plugins over loopback:

| v1 route | Upstream |
|---|---|
| `GET /v1/me` | np_saas_* control tables (tier, quota used/limit) |
| `GET/POST /v1/monitors`, `DELETE /v1/monitors/{id}`, `POST .../pause\|resume` | nself-uptime-monitor `/api/v1/targets` (3831) |
| `GET /v1/incidents?status=`, `POST .../ack\|resolve` | nself-incident-mgmt `/incidents` (3833) |
| `GET/POST /v1/status-pages` | gateway-owned `np_saas_status_pages` registry (quota-gated; W2 provisions instances) |
| `GET /v1/alerts/channels`, `POST .../{id}/test` | nself-alert-router `/routes` + `/api/alerts` (3834) |
| `GET/POST /v1/ci/events` | gateway-owned `np_saas_ci_events` (CI-failure ingest; see § CI-failure ingest below) |
| `POST /v1/signup` | creates tenant + owner email + first API key (shown once); optional `password`/`name` store a bcrypt login hash and additionally return a session JWT |
| `POST /v1/login` | email+password → HS256 session JWT (signed with `SAAS_JWT_HS256_SECRET`; claims tenant_id/email/tier/name, exp 7d); per-IP rate-limited |
| `GET /v1/session` | `Bearer <jwt>` → verified identity echo |
| `GET /v1/join/{token}` | public, token-gated invite info (tenant name, email, role) for the SPA accept page; bad/expired/consumed tokens → generic 404 |
| `POST /v1/join` | `{token,password,name?}` consumes a team-invite token: activates the member under the INVITING tenant, stores a bcrypt login hash, re-checks the seat quota (402 when full), returns a session JWT with role `member` |
| `POST /internal/billing/tenant-tier` | shared-secret (`NSENTRY_INTERNAL_API_KEY`) tier upsert for the Stripe webhook (W4) |

- Errors always arrive as `{"error":{"code","message"}}`; plugin quota
  rejections (402/429) pass through with `code: quota_exceeded`.

## Configuration

| Env | Default | Purpose |
|---|---|---|
| `PORT` | `3848` | Listen port |
| `DATABASE_URL` | — | ops-Postgres with np_saas_* tables (required in cloud mode) |
| `SAAS_GATEWAY_MODE` | `cloud` | `selfhost` disables tenant auth (never for the SaaS box) |
| `SAAS_GATEWAY_UPTIME_URL` | `http://127.0.0.1:3831` | uptime-monitor base |
| `SAAS_GATEWAY_INCIDENT_URL` | `http://127.0.0.1:3833` | incident-mgmt base |
| `SAAS_GATEWAY_ALERT_URL` | `http://127.0.0.1:3834` | alert-router base |
| `SAAS_GATEWAY_STATUS_PAGE_BASE` | `https://sentry.nself.org/s` | public status-page URL base |
| `NSENTRY_INTERNAL_API_KEY` | — | shared secret for `/internal/billing/tenant-tier` (unset = 503) |
| `SAAS_JWT_HS256_SECRET` | — | HS256 secret signing `/v1/login` session JWTs; the SPA's `AUTH_HS256_KEY` must equal it (unset = password auth 503; falls back to `NSELF_JWT_SECRET`) |
| `ALERT_ROUTER_INBOUND_HMAC_SECRET` | — | signs synthetic test alerts when the router requires HMAC |

`/internal/*` must NOT be routed by the public nginx vhost — loopback only.

## CI-failure ingest (`/v1/ci/events`)

The SaaS equivalent of what self-hosted Sentry Bundle users get from the
GitHub-Actions bridge (CI-failure report aggregation). We do not host
multi-tenant git — tenants push one event per pipeline run from their own
CI; the gateway stores it tenant-scoped and, on `status: failure`, fires an
alert through the same router path `monitor.down` uses (email/webhook/
Slack/Telegram per the tenant's configured channels).

| Route | Auth | Notes |
|---|---|---|
| `POST /v1/ci/events` | `Authorization: Bearer nsk_<key>` or session JWT | One event per call. Quota-metered monthly like error events (429 past cap). |
| `GET /v1/ci/events` | same | Tenant-scoped list, newest first, capped at 100. |

Request body (`POST`):

```json
{
  "repo": "org/repo",
  "workflow": "ci.yml",
  "status": "success | failure | cancelled",
  "run_url": "https://github.com/org/repo/actions/runs/123",
  "sha": "abc1234",
  "title": "optional short description"
}
```

Response (`201`): `{"ci_event": {...}}` with a server-assigned `id` and
`created_at`. `GET` returns `{"ci_events": [...]}`.

### GitHub Actions snippet (copy-paste)

Add this as the last step of any job — it fires only on failure and is the
entire "bridge" a SaaS tenant needs:

```yaml
      - name: Report CI failure to nSentry
        if: failure()
        run: |
          curl -sS -X POST https://api.sentry.nself.org/v1/ci/events \
            -H "Authorization: Bearer ${{ secrets.NSENTRY_API_KEY }}" \
            -H "Content-Type: application/json" \
            -d '{
                  "repo": "${{ github.repository }}",
                  "workflow": "${{ github.workflow }}",
                  "status": "failure",
                  "run_url": "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}",
                  "sha": "${{ github.sha }}",
                  "title": "${{ github.workflow }} failed on ${{ github.ref_name }}"
                }'
```

Store the tenant's `nsk_` API key as the `NSENTRY_API_KEY` repo/org secret.
No git hosting, no webhook receiver on the tenant's side — just one curl.

## Local-first dev (W8)

```sh
DATABASE_URL=postgres://... go run ./cmd/seed   # dev tenant + deterministic key
export NSELF_SENTRY_API_KEY=nsk_dev_local_000000000000000000000000000000000000000000000000000000
nself sentry monitors --api-url http://localhost:3848
```

The seeder refuses non-local databases (the dev key is deterministic).

## Tests

`go test ./...` — httptest fakes stand in for the three plugins; sqlmock
covers the DB-backed surfaces (me, status pages, signup, billing).
