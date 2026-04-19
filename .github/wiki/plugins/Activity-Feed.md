# Activity Feed Plugin

Universal activity feed system with fan-out-on-read/write strategies, activity aggregation, subscriptions, and entity feeds. Build Twitter-style timelines, notification feeds, or audit logs.

| Property | Value |
|----------|-------|
| **Port** | `3503` |
| **Category** | `content` |
| **Multi-App** | `source_account_id` (single) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run activity-feed init
nself plugin run activity-feed server
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
| `ACTIVITYFEED_PLUGIN_PORT` | `3503` | Server port |
| `ACTIVITYFEED_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `ACTIVITYFEED_STRATEGY` | `read` | Fan-out strategy: `read` or `write` |
| `ACTIVITYFEED_MAX_FEED_SIZE` | `1000` | Maximum items per user feed |
| `ACTIVITYFEED_AGGREGATION_WINDOW_MINUTES` | `60` | Time window for aggregating similar activities |
| `ACTIVITYFEED_RETENTION_DAYS` | `365` | Days to retain activity records |
| `ACTIVITYFEED_API_KEY` | - | API key for authentication |
| `ACTIVITYFEED_RATE_LIMIT_MAX` | `200` | Max requests per window |
| `ACTIVITYFEED_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (ms) |

---

## Fan-Out Strategies

### Fan-Out-on-Read (default)

Activities are stored once. When a user requests their feed, the system queries all subscriptions and merges activities at read time. Best for systems with few reads relative to writes, or where feeds need to be fully up-to-date.

### Fan-Out-on-Write

When an activity is created, copies are pre-materialized into each subscriber's feed table (`feed_user_feeds`). Best for systems with many reads and fewer writes (like social media timelines).

Set via `ACTIVITYFEED_STRATEGY=write` or the `POST /v1/fanout` endpoint.

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema |
| `server` | Start the HTTP API server (`-p`/`--port`, `-h`/`--host`) |
| `status` | Show feed statistics |
| `activities` | List activities (`-l`/`--limit`, `--actor`, `--verb`) |
| `feed` | View user feed (`<userId>`, `--unread`, `--limit`) |
| `subscriptions` | List subscriptions for a user |
| `subscribe` | Create subscription (`<userId> <targetType> <targetId>`) |
| `fanout` | Trigger fan-out for pending activities |
| `create-activity` | Create activity (`--actor`, `--verb`, `--object`, `--target`) |
| `cleanup` | Remove activities older than retention period |
| `stats` | Show database statistics |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |
| `GET` | `/live` | Liveness with memory/uptime |
| `GET` | `/v1/status` | Plugin status with stats |

### Activities

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/activities` | Create activity (body: `actor_id`, `actor_type`, `verb`, `object_id`, `object_type`, `target_id?`, `target_type?`, `metadata?`, `extra?`) |
| `GET` | `/v1/activities` | List activities (query: `actor_id?`, `verb?`, `object_type?`, `limit?`, `offset?`) |
| `GET` | `/v1/activities/:id` | Get activity by ID |
| `DELETE` | `/v1/activities/:id` | Delete activity |

### User Feeds

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/feed/:userId` | Get user feed (query: `limit?`, `offset?`) -- uses configured fan-out strategy |
| `GET` | `/v1/feed/:userId/unread` | Get unread count |
| `POST` | `/v1/feed/:userId/read` | Mark feed items as read (body: `activity_ids[]` or `all: true`) |
| `POST` | `/v1/feed/:userId/hide` | Hide feed items (body: `activity_ids[]`) |
| `GET` | `/v1/feed/:userId/stats` | Get feed statistics (total, unread, hidden) |

### Entity Feeds

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/entity/:type/:id/feed` | Get activity feed for an entity (e.g., all activities on a project) |

### Subscriptions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/subscriptions` | Create subscription (body: `user_id`, `target_type`, `target_id`, `verb_filter?`) |
| `GET` | `/v1/subscriptions` | List subscriptions (query: `user_id?`, `target_type?`) |
| `DELETE` | `/v1/subscriptions/:id` | Delete subscription |

### Fan-Out

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/fanout` | Trigger fan-out for pending activities (write strategy) |

### Stats

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/stats` | Get global feed statistics |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `activity.created` | New activity published |
| `activity.deleted` | Activity removed |
| `feed.read` | Feed items marked as read |
| `feed.hidden` | Feed items hidden |
| `subscription.created` | New subscription created |
| `subscription.deleted` | Subscription removed |
| `fanout.completed` | Fan-out batch completed |

---

## Activity Verbs

The plugin supports these predefined verbs (extensible with custom strings):

`created`, `updated`, `deleted`, `shared`, `liked`, `commented`, `followed`, `unfollowed`, `mentioned`, `assigned`, `completed`, `approved`, `rejected`, `published`, `archived`

---

## Database Schema

### `feed_activities`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Activity ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `actor_id` | `VARCHAR(255)` | Who performed the action |
| `actor_type` | `VARCHAR(100)` | Actor type (user, system, bot) |
| `verb` | `VARCHAR(100)` | Action verb |
| `object_id` | `VARCHAR(255)` | Target object ID |
| `object_type` | `VARCHAR(100)` | Target object type |
| `target_id` | `VARCHAR(255)` | Context target ID |
| `target_type` | `VARCHAR(100)` | Context target type |
| `metadata` | `JSONB` | Arbitrary metadata |
| `extra` | `JSONB` | Extra data for display (titles, previews) |
| `is_aggregated` | `BOOLEAN` | Whether part of an aggregation group |
| `aggregation_key` | `VARCHAR(255)` | Key for grouping similar activities |
| `created_at` | `TIMESTAMPTZ` | Activity timestamp |

### `feed_user_feeds`

Materialized feed entries for fan-out-on-write strategy.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Feed entry ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | Feed owner |
| `activity_id` | `UUID` (FK) | References `feed_activities` |
| `is_read` | `BOOLEAN` | Whether user has read this item |
| `is_hidden` | `BOOLEAN` | Whether user has hidden this item |
| `read_at` | `TIMESTAMPTZ` | When marked as read |
| `created_at` | `TIMESTAMPTZ` | When added to feed |

### `feed_subscriptions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Subscription ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | Subscriber |
| `target_type` | `VARCHAR(100)` | What to follow (user, project, etc.) |
| `target_id` | `VARCHAR(255)` | Specific entity to follow |
| `verb_filter` | `TEXT[]` | Optional verb filter (only receive specific verbs) |
| `is_active` | `BOOLEAN` | Whether subscription is active |
| `created_at` | `TIMESTAMPTZ` | Subscription creation time |

### `feed_webhook_events`

Standard webhook event tracking table with `id`, `source_account_id`, `event_type`, `payload` (JSONB), `processed`, `processed_at`, `error`, `created_at`.

---

## Activity Aggregation

Activities with the same `aggregation_key` within the `ACTIVITYFEED_AGGREGATION_WINDOW_MINUTES` window are grouped together. For example, "Alice, Bob, and 3 others liked your post" aggregates multiple `liked` activities on the same object.

The aggregation key is typically constructed as `{verb}:{object_type}:{object_id}`.

---

## Troubleshooting

**Feed returns empty** -- Verify the user has active subscriptions. For fan-out-on-read, subscriptions must exist. For fan-out-on-write, run `POST /v1/fanout` after creating activities.

**High latency on feed reads** -- Switch to fan-out-on-write strategy (`ACTIVITYFEED_STRATEGY=write`) if read performance is critical.

**Activities not appearing in feed** -- Check that the activity's `actor_type`/`actor_id` or `object_type`/`object_id` matches a subscription's `target_type`/`target_id`.
