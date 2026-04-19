# Analytics Plugin

Event tracking, counters, funnels, and quota management analytics engine for nself. Track user behavior, measure conversion rates, enforce rate limits, and build real-time dashboards.

| Property | Value |
|----------|-------|
| **Port** | `3304` |
| **Category** | `infrastructure` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run analytics init
nself plugin run analytics server
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ANALYTICS_PLUGIN_PORT` | `3304` | Server port |
| `ANALYTICS_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `ANALYTICS_BATCH_SIZE` | `100` | Maximum events per batch request |
| `ANALYTICS_ROLLUP_INTERVAL_MS` | `3600000` | Counter rollup interval (ms) |
| `ANALYTICS_EVENT_RETENTION_DAYS` | `90` | Days to retain raw events |
| `ANALYTICS_COUNTER_RETENTION_DAYS` | `365` | Days to retain counter data |
| `ANALYTICS_API_KEY` | - | API key for authentication |
| `ANALYTICS_RATE_LIMIT_MAX` | `500` | Max requests per window |
| `ANALYTICS_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (ms) |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (6 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`, `-h`/`--host`) |
| `status` | Show event/counter/funnel/quota counts |
| `track` | Track an event (`-n`/`--name`, `-c`/`--category`, `-u`/`--user`, `-s`/`--session`, `-p`/`--properties`) |
| `counters` | Manage counters: `list`, `get`, `increment` (`-n`/`--name`, `-d`/`--dimension`, `-p`/`--period`, `-i`/`--increment`) |
| `funnels` | Manage funnels: `list`, `show`, `create`, `analyze` (`-n`/`--name`, `-s`/`--steps`, `-w`/`--window`) |
| `quotas` | Manage quotas: `list`, `create`, `check` (`-n`/`--name`, `-c`/`--counter`, `-m`/`--max`, `-p`/`--period`, `-s`/`--scope`) |
| `rollup` | Trigger counter rollup (hourly -> daily -> monthly) |
| `dashboard` | Show analytics dashboard with top events, quota status |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |
| `GET` | `/live` | Liveness with memory/uptime/stats |
| `GET` | `/v1/status` | Plugin status with counts |

### Event Tracking

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/events` | Track single event (body: `event_name`, `event_category?`, `user_id?`, `session_id?`, `properties?`, `context?`, `source_plugin?`, `timestamp?`) |
| `POST` | `/v1/events/batch` | Track multiple events (body: `events[]`, max `ANALYTICS_BATCH_SIZE`) |
| `GET` | `/v1/events` | Query events (query: `event_name?`, `user_id?`, `session_id?`, `start_date?`, `end_date?`, `limit?`, `offset?`) |

### Counters

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/counters/increment` | Increment a counter (body: `counter_name`, `dimension?`, `increment?`, `metadata?`) |
| `GET` | `/v1/counters` | Get counter value (query: `counter_name` (required), `dimension?`, `period?`) |
| `GET` | `/v1/counters/:name/timeseries` | Get counter timeseries (query: `dimension?`, `period?`, `start_date?`, `end_date?`) |
| `POST` | `/v1/counters/rollup` | Trigger counter rollup |

### Funnels

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/funnels` | Create funnel (body: `name`, `steps[]`, `description?`, `window_hours?`, `enabled?`) |
| `GET` | `/v1/funnels` | List funnels (query: `limit?`, `offset?`) |
| `GET` | `/v1/funnels/:id` | Get funnel details |
| `GET` | `/v1/funnels/:id/analyze` | Analyze funnel (returns step-by-step conversion and drop-off rates) |
| `PUT` | `/v1/funnels/:id` | Update funnel |
| `DELETE` | `/v1/funnels/:id` | Delete funnel |

### Quotas

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/quotas` | Create quota (body: `name`, `counter_name`, `max_value`, `period`, `scope?`, `scope_id?`, `action_on_exceed?`, `enabled?`) |
| `GET` | `/v1/quotas` | List quotas (query: `limit?`, `offset?`) |
| `PUT` | `/v1/quotas/:id` | Update quota |
| `DELETE` | `/v1/quotas/:id` | Delete quota |
| `POST` | `/v1/quotas/check` | Check quota (body: `counter_name`, `scope_id?`, `increment?`) |
| `GET` | `/v1/violations` | List quota violations (query: `limit?`, `offset?`) |

### Dashboard

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/dashboard` | Full dashboard (total events, unique users/sessions, top events, quota status, recent events) |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `event.tracked` | New event tracked |
| `counter.incremented` | Counter incremented |
| `quota.exceeded` | Quota limit exceeded |
| `funnel.completed` | Funnel completed |
| `rollup.completed` | Counter rollup completed |

---

## Counter System

Counters auto-increment across four time periods simultaneously:

| Period | Description |
|--------|-------------|
| `hourly` | Per-hour buckets |
| `daily` | Per-day buckets |
| `monthly` | Per-month buckets |
| `all_time` | Single cumulative counter (epoch start) |

When an event is tracked via `POST /v1/events`, a matching counter is automatically incremented for the event name. Counters use upsert logic with `ON CONFLICT ... DO UPDATE SET value = value + increment`.

### Rollup

The rollup operation aggregates hourly counters into daily and daily into monthly using `SUM()` with `GROUP BY DATE_TRUNC()`. Trigger manually via `POST /v1/counters/rollup` or the `rollup` CLI command.

---

## Funnel Analysis

Funnels define a sequence of steps (events) that users should complete in order. The analysis engine:

1. Counts unique `user_id` values for the first step event
2. For each subsequent step, joins on `user_id` where the next event occurred within `window_hours` of the previous event
3. Returns per-step user counts, conversion rates, and drop-off rates

### Funnel Step Format

```json
{
  "name": "Sign Up",
  "event_name": "user.signup",
  "filters": { "plan": "pro" }
}
```

### Analysis Response

```json
{
  "funnel_id": "...",
  "funnel_name": "Onboarding",
  "steps": [
    { "step_number": 1, "step_name": "Visit", "event_name": "page.view", "users": 1000, "conversion_rate": 100, "drop_off_rate": 0 },
    { "step_number": 2, "step_name": "Sign Up", "event_name": "user.signup", "users": 400, "conversion_rate": 40, "drop_off_rate": 60 },
    { "step_number": 3, "step_name": "Purchase", "event_name": "order.placed", "users": 80, "conversion_rate": 20, "drop_off_rate": 80 }
  ],
  "total_entered": 1000,
  "total_completed": 80,
  "overall_conversion_rate": 8
}
```

---

## Quota System

Quotas enforce limits on counter values. When a quota check finds the counter would exceed `max_value`, the system records a violation and returns `allowed: false`.

| Scope | Description |
|-------|-------------|
| `app` | Applies to the entire application |
| `user` | Per-user limit (uses `scope_id`) |
| `device` | Per-device limit (uses `scope_id`) |

| Action on Exceed | Description |
|-------------------|-------------|
| `warn` | Allow but flag the violation |
| `block` | Deny the operation |
| `throttle` | Slow down the operation |

### Quota Check Response

```json
{
  "allowed": false,
  "quota_name": "Daily API Calls",
  "current_value": 1001,
  "max_value": 1000,
  "remaining": -1,
  "action": "block"
}
```

---

## Database Schema

### `analytics_events`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Event ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `event_name` | `VARCHAR(255)` | Event name (e.g., `page.view`, `user.signup`) |
| `event_category` | `VARCHAR(128)` | Optional category grouping |
| `user_id` | `VARCHAR(255)` | User who triggered the event |
| `session_id` | `VARCHAR(255)` | Session identifier |
| `properties` | `JSONB` | Event-specific properties |
| `context` | `JSONB` | Request context (device, browser, etc.) |
| `source_plugin` | `VARCHAR(128)` | Plugin that generated the event |
| `timestamp` | `TIMESTAMPTZ` | Event timestamp |
| `created_at` | `TIMESTAMPTZ` | Record creation time |

### `analytics_counters`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Counter record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `counter_name` | `VARCHAR(255)` | Counter name |
| `dimension` | `VARCHAR(255)` | Counter dimension (default: `total`) |
| `period` | `VARCHAR(32)` | `hourly`, `daily`, `monthly`, `all_time` |
| `period_start` | `TIMESTAMPTZ` | Start of the time bucket |
| `value` | `BIGINT` | Counter value |
| `metadata` | `JSONB` | Arbitrary metadata |
| `updated_at` | `TIMESTAMPTZ` | Last update |

Unique constraint: `(source_account_id, counter_name, dimension, period, period_start)`

### `analytics_funnels`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Funnel ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Funnel name |
| `description` | `TEXT` | Description |
| `steps` | `JSONB` | Array of `{ name, event_name, filters? }` |
| `window_hours` | `INTEGER` | Conversion window (default: 24) |
| `enabled` | `BOOLEAN` | Whether funnel is active |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `analytics_quotas`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Quota ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Quota name |
| `scope` | `VARCHAR(32)` | `app`, `user`, `device` |
| `scope_id` | `VARCHAR(255)` | Scope identifier (user ID, device ID) |
| `counter_name` | `VARCHAR(255)` | Counter to monitor |
| `max_value` | `BIGINT` | Maximum allowed value |
| `period` | `VARCHAR(32)` | Counter period to check |
| `action_on_exceed` | `VARCHAR(32)` | `warn`, `block`, `throttle` |
| `enabled` | `BOOLEAN` | Whether quota is active |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `analytics_quota_violations`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Violation ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `quota_id` | `UUID` (FK) | References `analytics_quotas` |
| `scope_id` | `VARCHAR(255)` | Scope identifier |
| `current_value` | `BIGINT` | Value at violation time |
| `max_value` | `BIGINT` | Quota limit |
| `action_taken` | `VARCHAR(32)` | Action taken (`warn`, `block`, `throttle`) |
| `notified` | `BOOLEAN` | Whether notification was sent |
| `created_at` | `TIMESTAMPTZ` | Violation timestamp |

### `analytics_webhook_events`

Standard webhook event tracking table with `id`, `source_account_id`, `event_type`, `payload` (JSONB), `processed`, `processed_at`, `error`, `created_at`.

---

## Dashboard

The `GET /v1/dashboard` endpoint returns a comprehensive view:

| Field | Description |
|-------|-------------|
| `total_events` | Total tracked events |
| `unique_users` | Distinct `user_id` count |
| `unique_sessions` | Distinct `session_id` count |
| `active_quotas` | Enabled quota count |
| `quota_violations` | Violations in last 24 hours |
| `top_events` | Top 10 events by count |
| `recent_events` | Last 10 events |
| `quota_status` | Quota usage percentages, sorted by utilization |

---

## Troubleshooting

**Events not appearing** -- Verify `event_name` is provided in the request body. Check that the API key matches if `ANALYTICS_API_KEY` is set.

**Counter values seem wrong** -- Counters auto-increment across all four periods. Use the `period` query parameter to check specific time buckets. Run `rollup` to re-aggregate.

**Funnel shows 0 users** -- Funnel analysis requires events with non-null `user_id`. Verify events exist for all step event names.

**Quota not enforcing** -- Confirm the quota is `enabled: true` and the `counter_name` matches an existing counter. Quotas match by `scope_id` first, then fall back to `scope_id IS NULL`.

**Batch rejected** -- Ensure the events array does not exceed `ANALYTICS_BATCH_SIZE` (default: 100).
