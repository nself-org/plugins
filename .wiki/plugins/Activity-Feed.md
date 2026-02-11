# Activity Feed Plugin

Universal activity feed system with fan-out-on-read/write strategies, activity aggregation, and real-time subscriptions for nself applications.

## Overview

The Activity Feed plugin provides a complete activity stream system for tracking and displaying user activities across your application. It supports both fan-out-on-read and fan-out-on-write strategies, activity aggregation, user subscriptions, and personalized feeds.

### Key Features

- **Flexible Feed Strategies**: Choose between fan-out-on-read (real-time queries) or fan-out-on-write (pre-materialized feeds)
- **Activity Tracking**: Record all user actions with actor, verb, object, and target pattern
- **User Subscriptions**: Users can subscribe to actors/entities to receive their activities
- **Feed Aggregation**: Automatically group similar activities (e.g., "10 users liked your post")
- **Read/Unread Tracking**: Track which feed items users have viewed
- **Entity Feeds**: View all activities related to any object (post, comment, etc.)
- **Multi-App Support**: Isolated feeds per source account
- **Configurable Retention**: Automatically clean up old activities
- **Real-time Updates**: Webhook support for instant feed updates

### Use Cases

- **Social Networks**: News feeds, activity streams, notifications
- **Collaboration Tools**: Team activity tracking, project updates
- **E-commerce**: Order updates, product reviews, inventory changes
- **Content Platforms**: Content creation, comments, reactions
- **SaaS Applications**: User action logs, audit trails
- **Gaming**: Player achievements, leaderboard updates

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install activity-feed

# Initialize database schema
nself activity-feed init

# Start the server
nself activity-feed server
```

### Basic Usage

```bash
# Create an activity
nself activity-feed create-activity \
  --actor user123 \
  --verb created \
  --object post:abc123 \
  --message "Created a new blog post"

# Subscribe a user to another user's activities
nself activity-feed subscribe user456 user user123

# View a user's feed
nself activity-feed feed user456

# Check statistics
nself activity-feed status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `FEED_PLUGIN_PORT` | No | `3503` | HTTP server port |
| `FEED_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `FEED_STRATEGY` | No | `read` | Feed strategy: `read` or `write` |
| `FEED_MAX_FEED_SIZE` | No | `200` | Maximum items per feed request |
| `FEED_AGGREGATION_WINDOW_MINUTES` | No | `60` | Time window for activity aggregation |
| `FEED_RETENTION_DAYS` | No | `90` | Days to retain activities |
| `FEED_API_KEY` | No | - | API key for authentication |
| `FEED_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `FEED_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level |

### Feed Strategies

#### Fan-out-on-Read (Default)
- Activities are queried in real-time based on subscriptions
- Lower write overhead
- Higher read latency
- Best for: Small to medium user bases, frequently changing subscriptions

#### Fan-out-on-Write
- Activities are pre-materialized into user feeds
- Higher write overhead
- Lower read latency
- Best for: High-traffic applications, social networks

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
FEED_PLUGIN_PORT=3503
FEED_STRATEGY=write
FEED_MAX_FEED_SIZE=100
FEED_AGGREGATION_WINDOW_MINUTES=30
FEED_RETENTION_DAYS=180
FEED_API_KEY=your-secret-key
```

---

## CLI Commands

### `init`
Initialize the database schema.

```bash
nself activity-feed init
```

### `server`
Start the HTTP API server.

```bash
nself activity-feed server [options]

Options:
  -p, --port <port>    Server port (default: 3503)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

**Example:**
```bash
nself activity-feed server --port 3503 --host 0.0.0.0
```

### `status`
Show activity feed statistics.

```bash
nself activity-feed status
```

**Output:**
```
Activity Feed Statistics:
=========================
Total Activities:        15234
Total Subscriptions:     892
Total Feed Items:        45678
Unread Feed Items:       1234
Recent (24h):            456
Recent (7d):             3421
Last Activity:           2026-02-11T10:30:00Z

Activities by Verb:
  created              8234
  updated              3421
  liked                2134
  commented            1445

Activities by Actor Type:
  user                 14890
  system               344
```

### `activities`
List recent activities.

```bash
nself activity-feed activities [options]

Options:
  -l, --limit <limit>      Number of activities (default: 20)
  -a, --actor <actorId>    Filter by actor ID
  -v, --verb <verb>        Filter by verb
  -o, --object <type:id>   Filter by object
```

**Examples:**
```bash
# List all recent activities
nself activity-feed activities --limit 50

# Filter by actor
nself activity-feed activities --actor user123

# Filter by verb
nself activity-feed activities --verb liked

# Filter by object
nself activity-feed activities --object post:abc123
```

### `feed`
View a user's activity feed.

```bash
nself activity-feed feed <userId> [options]

Options:
  -l, --limit <limit>    Number of items (default: 20)
  --unread-only          Show only unread items
```

**Examples:**
```bash
# View user's feed
nself activity-feed feed user123

# View unread items only
nself activity-feed feed user123 --unread-only

# Limit results
nself activity-feed feed user123 --limit 50
```

### `subscriptions`
List a user's subscriptions.

```bash
nself activity-feed subscriptions <userId>
```

**Example:**
```bash
nself activity-feed subscriptions user123
```

**Output:**
```
Subscriptions for user123:
============================

✓ user:user456
  ID: abc-123-def
  Created: 2026-01-15T08:00:00Z

✓ user:user789
  ID: xyz-987-ghi
  Created: 2026-01-20T12:00:00Z
```

### `subscribe`
Subscribe a user to a target.

```bash
nself activity-feed subscribe <userId> <targetType> <targetId>
```

**Example:**
```bash
nself activity-feed subscribe user123 user user456
```

### `create-activity`
Create a new activity.

```bash
nself activity-feed create-activity [options]

Options:
  --actor <actorId>        Actor ID (required)
  --verb <verb>            Activity verb (required)
  --object <type:id>       Object type:id (required)
  --target <type:id>       Target type:id
  --message <message>      Activity message
  --plugin <plugin>        Source plugin name
```

**Examples:**
```bash
# Create a post activity
nself activity-feed create-activity \
  --actor user123 \
  --verb created \
  --object post:abc123 \
  --message "Created a new blog post"

# Create a like activity with target
nself activity-feed create-activity \
  --actor user456 \
  --verb liked \
  --object post:abc123 \
  --target user:user123 \
  --message "Liked your post"
```

### `fanout`
Manually trigger fan-out for an activity.

```bash
nself activity-feed fanout <activityId> [options]

Options:
  -f, --force    Force refresh existing feed items
```

**Example:**
```bash
nself activity-feed fanout abc-123-def-456
```

### `cleanup`
Clean up old activities based on retention policy.

```bash
nself activity-feed cleanup [options]

Options:
  -d, --days <days>    Retention days (default: from config)
```

**Example:**
```bash
# Use default retention from config
nself activity-feed cleanup

# Custom retention
nself activity-feed cleanup --days 30
```

### `stats`
Show detailed activity feed statistics (alias for `status`).

```bash
nself activity-feed stats
```

---

## REST API

All endpoints support multi-app isolation via `X-Source-Account-Id` header.

### Health & Status

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "activity-feed",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /ready`
Readiness check with database connectivity.

**Response:**
```json
{
  "ready": true,
  "plugin": "activity-feed",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /live`
Liveness check with detailed stats.

**Response:**
```json
{
  "alive": true,
  "plugin": "activity-feed",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 52428800,
    "heapTotal": 20971520,
    "heapUsed": 15728640
  },
  "config": {
    "strategy": "write",
    "maxFeedSize": 200,
    "aggregationWindowMinutes": 60
  },
  "stats": {
    "totalActivities": 15234,
    "totalSubscriptions": 892,
    "unreadFeedItems": 1234,
    "lastActivity": "2026-02-11T10:25:00Z"
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/status`
Plugin status and configuration.

**Response:**
```json
{
  "plugin": "activity-feed",
  "version": "1.0.0",
  "status": "running",
  "config": {
    "strategy": "write",
    "maxFeedSize": 200,
    "aggregationWindowMinutes": 60,
    "retentionDays": 90
  },
  "stats": {
    "totalActivities": 15234,
    "totalSubscriptions": 892,
    "totalFeedItems": 45678,
    "unreadFeedItems": 1234,
    "activitiesByVerb": {
      "created": 8234,
      "updated": 3421,
      "liked": 2134
    },
    "activitiesByActorType": {
      "user": 14890,
      "system": 344
    },
    "recentActivityCount24h": 456,
    "recentActivityCount7d": 3421,
    "lastActivityAt": "2026-02-11T10:25:00Z"
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

### Activities

#### `POST /v1/activities`
Create a new activity.

**Request:**
```json
{
  "actor_id": "user123",
  "actor_type": "user",
  "verb": "created",
  "object_type": "post",
  "object_id": "abc123",
  "target_type": "channel",
  "target_id": "general",
  "source_plugin": "cms",
  "message": "Created a new blog post",
  "data": {
    "title": "My First Post",
    "category": "technology"
  },
  "is_aggregatable": true
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "actor_id": "user123",
  "actor_type": "user",
  "verb": "created",
  "object_type": "post",
  "object_id": "abc123",
  "target_type": "channel",
  "target_id": "general",
  "source_plugin": "cms",
  "message": "Created a new blog post",
  "data": {
    "title": "My First Post",
    "category": "technology"
  },
  "is_aggregatable": true,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/activities`
List activities with optional filters.

**Query Parameters:**
- `actorId`: Filter by actor ID
- `verb`: Filter by verb
- `objectType`: Filter by object type
- `objectId`: Filter by object ID
- `targetType`: Filter by target type
- `targetId`: Filter by target ID
- `limit`: Results per page (default: 100, max: config.maxFeedSize)
- `offset`: Pagination offset (default: 0)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "actor_id": "user123",
      "verb": "created",
      "object_type": "post",
      "object_id": "abc123",
      "message": "Created a new blog post",
      "created_at": "2026-02-11T10:30:00Z"
    }
  ],
  "total": 150,
  "limit": 100,
  "offset": 0,
  "hasMore": true
}
```

#### `GET /v1/activities/:id`
Get a single activity by ID.

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "actor_id": "user123",
  "actor_type": "user",
  "verb": "created",
  "object_type": "post",
  "object_id": "abc123",
  "message": "Created a new blog post",
  "data": {},
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `DELETE /v1/activities/:id`
Delete an activity.

**Response:**
```json
{
  "success": true
}
```

### User Feeds

#### `GET /v1/feed/:userId`
Get a user's personalized activity feed.

**Query Parameters:**
- `limit`: Results per page (default: 100)
- `offset`: Pagination offset (default: 0)
- `includeRead`: Include read items (default: true)
- `includeHidden`: Include hidden items (default: false)

**Response:**
```json
{
  "data": [
    {
      "id": "feed-item-uuid",
      "user_id": "user456",
      "activity_id": "activity-uuid",
      "is_read": false,
      "read_at": null,
      "is_hidden": false,
      "created_at": "2026-02-11T10:30:00Z",
      "activity": {
        "id": "activity-uuid",
        "actor_id": "user123",
        "verb": "created",
        "object_type": "post",
        "object_id": "abc123",
        "message": "Created a new blog post"
      }
    }
  ],
  "total": 245,
  "unreadCount": 12,
  "limit": 100,
  "offset": 0,
  "hasMore": true
}
```

#### `GET /v1/feed/:userId/unread`
Get unread count for a user.

**Response:**
```json
{
  "userId": "user456",
  "unreadCount": 12
}
```

#### `POST /v1/feed/:userId/read`
Mark feed items as read.

**Request:**
```json
{
  "activityIds": ["activity-uuid-1", "activity-uuid-2"]
}
```

Leave `activityIds` empty to mark all as read.

**Response:**
```json
{
  "success": true,
  "updated": 2
}
```

#### `POST /v1/feed/:userId/hide`
Hide a feed item.

**Request:**
```json
{
  "activityId": "activity-uuid"
}
```

**Response:**
```json
{
  "success": true
}
```

#### `GET /v1/feed/:userId/stats`
Get user feed statistics.

**Response:**
```json
{
  "userId": "user456",
  "sourceAccountId": "primary",
  "totalItems": 245,
  "unreadCount": 12,
  "subscriptionCount": 15,
  "lastActivityAt": "2026-02-11T10:30:00Z"
}
```

### Entity Feeds

#### `GET /v1/entity/:type/:id/feed`
Get all activities for a specific entity.

**Query Parameters:**
- `limit`: Results per page (default: 100)
- `offset`: Pagination offset (default: 0)

**Example:** `GET /v1/entity/post/abc123/feed`

**Response:**
```json
{
  "data": [
    {
      "id": "activity-uuid",
      "actor_id": "user789",
      "verb": "commented",
      "object_type": "post",
      "object_id": "abc123",
      "message": "Great post!",
      "created_at": "2026-02-11T10:32:00Z"
    }
  ],
  "total": 45,
  "limit": 100,
  "offset": 0,
  "hasMore": false
}
```

### Subscriptions

#### `POST /v1/subscriptions`
Create a subscription.

**Request:**
```json
{
  "user_id": "user456",
  "target_type": "user",
  "target_id": "user123",
  "enabled": true
}
```

**Response:**
```json
{
  "id": "subscription-uuid",
  "source_account_id": "primary",
  "user_id": "user456",
  "target_type": "user",
  "target_id": "user123",
  "enabled": true,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/subscriptions/:userId`
List a user's subscriptions.

**Response:**
```json
{
  "data": [
    {
      "id": "subscription-uuid",
      "user_id": "user456",
      "target_type": "user",
      "target_id": "user123",
      "enabled": true,
      "created_at": "2026-02-11T10:00:00Z"
    }
  ],
  "total": 15
}
```

#### `DELETE /v1/subscriptions/:id`
Delete a subscription.

**Response:**
```json
{
  "success": true
}
```

### Fan-out

#### `POST /v1/fanout`
Manually trigger fan-out for an activity.

**Request:**
```json
{
  "activityId": "activity-uuid",
  "forceRefresh": false
}
```

**Response:**
```json
{
  "activityId": "activity-uuid",
  "subscribersCount": 50,
  "feedItemsCreated": 50,
  "duration": 234,
  "success": true
}
```

### Statistics

#### `GET /v1/stats`
Get comprehensive feed statistics.

**Response:**
```json
{
  "totalActivities": 15234,
  "totalSubscriptions": 892,
  "totalFeedItems": 45678,
  "unreadFeedItems": 1234,
  "activitiesByVerb": {
    "created": 8234,
    "updated": 3421,
    "liked": 2134,
    "commented": 1445
  },
  "activitiesByActorType": {
    "user": 14890,
    "system": 344
  },
  "recentActivityCount24h": 456,
  "recentActivityCount7d": 3421,
  "lastActivityAt": "2026-02-11T10:25:00Z"
}
```

---

## Webhook Events

The Activity Feed plugin emits webhook events for various feed operations.

### `activity.created`
Triggered when a new activity is created.

**Payload:**
```json
{
  "event": "activity.created",
  "activity": {
    "id": "activity-uuid",
    "actor_id": "user123",
    "verb": "created",
    "object_type": "post",
    "object_id": "abc123"
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

### `activity.updated`
Triggered when an activity is updated.

### `activity.deleted`
Triggered when an activity is deleted.

### `subscription.created`
Triggered when a user subscribes to a target.

### `subscription.deleted`
Triggered when a subscription is removed.

### `feed.read`
Triggered when feed items are marked as read.

### `feed.hidden`
Triggered when a feed item is hidden.

---

## Database Schema

### `feed_activities`
Stores all activities in the system.

```sql
CREATE TABLE feed_activities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  actor_id VARCHAR(255) NOT NULL,
  actor_type VARCHAR(32) DEFAULT 'user',
  verb VARCHAR(64) NOT NULL,
  object_type VARCHAR(64) NOT NULL,
  object_id VARCHAR(255) NOT NULL,
  target_type VARCHAR(64),
  target_id VARCHAR(255),
  source_plugin VARCHAR(64),
  message TEXT,
  data JSONB DEFAULT '{}',
  is_aggregatable BOOLEAN DEFAULT true,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_feed_activities_source_account ON feed_activities(source_account_id);
CREATE INDEX idx_feed_activities_actor ON feed_activities(source_account_id, actor_id);
CREATE INDEX idx_feed_activities_object ON feed_activities(source_account_id, object_type, object_id);
CREATE INDEX idx_feed_activities_target ON feed_activities(source_account_id, target_type, target_id);
CREATE INDEX idx_feed_activities_created ON feed_activities(source_account_id, created_at DESC);
CREATE INDEX idx_feed_activities_verb ON feed_activities(source_account_id, verb);
CREATE INDEX idx_feed_activities_aggregatable ON feed_activities(
  source_account_id, is_aggregatable, verb, object_type, object_id, created_at DESC
) WHERE is_aggregatable = true;
```

**Columns:**
- `id`: Unique activity identifier (UUID)
- `source_account_id`: Multi-app isolation column
- `actor_id`: ID of the user/entity performing the action
- `actor_type`: Type of actor (user, system, bot, etc.)
- `verb`: Action verb (created, updated, liked, commented, etc.)
- `object_type`: Type of object acted upon
- `object_id`: ID of the object
- `target_type`: Optional target type
- `target_id`: Optional target ID
- `source_plugin`: Plugin that generated this activity
- `message`: Human-readable activity message
- `data`: Additional JSON metadata
- `is_aggregatable`: Whether this activity can be aggregated
- `created_at`: Activity creation timestamp

### `feed_user_feeds`
Pre-materialized user feeds (fan-out-on-write strategy).

```sql
CREATE TABLE feed_user_feeds (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  activity_id UUID NOT NULL REFERENCES feed_activities(id) ON DELETE CASCADE,
  is_read BOOLEAN DEFAULT false,
  read_at TIMESTAMP WITH TIME ZONE,
  is_hidden BOOLEAN DEFAULT false,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, user_id, activity_id)
);

CREATE INDEX idx_feed_user_feeds_source_account ON feed_user_feeds(source_account_id);
CREATE INDEX idx_feed_user_feeds_user ON feed_user_feeds(source_account_id, user_id, created_at DESC);
CREATE INDEX idx_feed_user_feeds_activity ON feed_user_feeds(activity_id);
CREATE INDEX idx_feed_user_feeds_unread ON feed_user_feeds(source_account_id, user_id, is_read) WHERE is_read = false;
CREATE INDEX idx_feed_user_feeds_visible ON feed_user_feeds(source_account_id, user_id, is_hidden) WHERE is_hidden = false;
```

**Columns:**
- `id`: Unique feed item identifier (UUID)
- `source_account_id`: Multi-app isolation column
- `user_id`: User this feed item belongs to
- `activity_id`: Reference to the activity
- `is_read`: Whether user has read this item
- `read_at`: When the item was read
- `is_hidden`: Whether user has hidden this item
- `created_at`: When item was added to feed

### `feed_subscriptions`
User subscriptions to actors/entities.

```sql
CREATE TABLE feed_subscriptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  target_type VARCHAR(64) NOT NULL,
  target_id VARCHAR(255) NOT NULL,
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, user_id, target_type, target_id)
);

CREATE INDEX idx_feed_subscriptions_source_account ON feed_subscriptions(source_account_id);
CREATE INDEX idx_feed_subscriptions_user ON feed_subscriptions(source_account_id, user_id);
CREATE INDEX idx_feed_subscriptions_target ON feed_subscriptions(source_account_id, target_type, target_id);
CREATE INDEX idx_feed_subscriptions_enabled ON feed_subscriptions(source_account_id, enabled) WHERE enabled = true;
```

**Columns:**
- `id`: Unique subscription identifier (UUID)
- `source_account_id`: Multi-app isolation column
- `user_id`: Subscribing user
- `target_type`: Type of target being subscribed to
- `target_id`: ID of the target
- `enabled`: Whether subscription is active
- `created_at`: Subscription creation timestamp

### `feed_webhook_events`
Webhook event log.

```sql
CREATE TABLE feed_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128),
  payload JSONB,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_feed_webhook_events_source_account ON feed_webhook_events(source_account_id);
CREATE INDEX idx_feed_webhook_events_type ON feed_webhook_events(source_account_id, event_type);
CREATE INDEX idx_feed_webhook_events_processed ON feed_webhook_events(source_account_id, processed);
CREATE INDEX idx_feed_webhook_events_created ON feed_webhook_events(source_account_id, created_at DESC);
```

---

## Examples

### Example 1: Social Media Activity Stream

```bash
# User creates a post
curl -X POST http://localhost:3503/v1/activities \
  -H "Content-Type: application/json" \
  -d '{
    "actor_id": "user123",
    "verb": "created",
    "object_type": "post",
    "object_id": "post456",
    "message": "Shared a new photo"
  }'

# Another user likes the post
curl -X POST http://localhost:3503/v1/activities \
  -H "Content-Type: application/json" \
  -d '{
    "actor_id": "user789",
    "verb": "liked",
    "object_type": "post",
    "object_id": "post456",
    "target_type": "user",
    "target_id": "user123"
  }'

# User subscribes to follow another user
curl -X POST http://localhost:3503/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user789",
    "target_type": "user",
    "target_id": "user123"
  }'

# Get user's personalized feed
curl http://localhost:3503/v1/feed/user789
```

### Example 2: Collaboration Platform

```bash
# Team member updates a document
curl -X POST http://localhost:3503/v1/activities \
  -H "Content-Type: application/json" \
  -d '{
    "actor_id": "user456",
    "verb": "updated",
    "object_type": "document",
    "object_id": "doc789",
    "target_type": "project",
    "target_id": "proj123",
    "message": "Updated project requirements",
    "data": {
      "section": "requirements",
      "changes": 15
    }
  }'

# Subscribe team members to project activities
curl -X POST http://localhost:3503/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user111",
    "target_type": "project",
    "target_id": "proj123"
  }'

# Get all activities for a project
curl http://localhost:3503/v1/entity/project/proj123/feed
```

### Example 3: E-commerce Order Tracking

```bash
# Create order placed activity
curl -X POST http://localhost:3503/v1/activities \
  -H "Content-Type: application/json" \
  -d '{
    "actor_id": "user999",
    "verb": "created",
    "object_type": "order",
    "object_id": "order123",
    "message": "Placed order #123",
    "data": {
      "total": 99.99,
      "items": 3
    }
  }'

# Order shipped
curl -X POST http://localhost:3503/v1/activities \
  -H "Content-Type: application/json" \
  -d '{
    "actor_id": "system",
    "actor_type": "system",
    "verb": "shipped",
    "object_type": "order",
    "object_id": "order123",
    "target_type": "user",
    "target_id": "user999",
    "message": "Your order has shipped"
  }'

# Get order activity history
curl http://localhost:3503/v1/entity/order/order123/feed
```

### Example 4: Content Aggregation

```bash
# Multiple users like the same post
for i in {1..10}; do
  curl -X POST http://localhost:3503/v1/activities \
    -H "Content-Type: application/json" \
    -d "{
      \"actor_id\": \"user${i}\",
      \"verb\": \"liked\",
      \"object_type\": \"post\",
      \"object_id\": \"post123\",
      \"is_aggregatable\": true
    }"
done

# Get aggregated feed (shows "10 users liked this post")
curl http://localhost:3503/v1/feed/author123
```

### Example 5: Read/Unread Management

```bash
# Get unread count
curl http://localhost:3503/v1/feed/user123/unread

# Mark specific items as read
curl -X POST http://localhost:3503/v1/feed/user123/read \
  -H "Content-Type: application/json" \
  -d '{
    "activityIds": ["activity-uuid-1", "activity-uuid-2"]
  }'

# Mark all as read
curl -X POST http://localhost:3503/v1/feed/user123/read \
  -H "Content-Type: application/json" \
  -d '{}'

# Hide a feed item
curl -X POST http://localhost:3503/v1/feed/user123/hide \
  -H "Content-Type: application/json" \
  -d '{
    "activityId": "activity-uuid"
  }'
```

---

## Troubleshooting

### High Memory Usage with Fan-out-on-Write

**Problem:** Server memory grows with large subscriber counts.

**Solution:**
- Switch to fan-out-on-read strategy: `FEED_STRATEGY=read`
- Implement pagination when querying subscribers
- Use background jobs for fan-out operations
- Consider Redis for feed caching

### Slow Feed Queries

**Problem:** User feeds take too long to load.

**Solution:**
- Ensure indexes are created: run `nself activity-feed init`
- Switch to fan-out-on-write for better read performance
- Reduce `FEED_MAX_FEED_SIZE` for faster queries
- Add database connection pooling
- Use read replicas for feed queries

### Activities Not Appearing in Feeds

**Problem:** Users don't see expected activities.

**Solution:**
- Check subscription status: `nself activity-feed subscriptions <userId>`
- Verify feed strategy matches expected behavior
- For fan-out-on-write, manually trigger: `nself activity-feed fanout <activityId>`
- Check `is_hidden` flag on feed items
- Verify `source_account_id` matches

### Duplicate Activities

**Problem:** Same activity appears multiple times.

**Solution:**
- Check webhook event processing logs
- Ensure idempotent activity creation with unique IDs
- Review application logic creating activities
- Check for race conditions in subscription processing

### Database Connection Issues

**Problem:** "Connection refused" or timeout errors.

**Solution:**
```bash
# Test database connection
psql $DATABASE_URL -c "SELECT 1"

# Check PostgreSQL is running
systemctl status postgresql

# Verify credentials
echo $DATABASE_URL

# Test with CLI
nself activity-feed status
```

### Aggregation Not Working

**Problem:** Activities not being aggregated.

**Solution:**
- Verify `is_aggregatable: true` when creating activities
- Check `FEED_AGGREGATION_WINDOW_MINUTES` is set appropriately
- Ensure similar activities have same verb, object_type, object_id
- Query aggregated feed endpoint directly

### Rate Limiting Errors

**Problem:** 429 Too Many Requests responses.

**Solution:**
```bash
# Increase rate limits
export FEED_RATE_LIMIT_MAX=500
export FEED_RATE_LIMIT_WINDOW_MS=60000

# Or disable rate limiting (not recommended for production)
unset FEED_API_KEY
```

### Old Activities Not Cleaned Up

**Problem:** Database growing with old activities.

**Solution:**
```bash
# Run cleanup manually
nself activity-feed cleanup

# Schedule with cron
0 2 * * * nself activity-feed cleanup --days 90

# Adjust retention period
export FEED_RETENTION_DAYS=30
```

---

## Performance Tips

1. **Choose the Right Strategy**
   - Fan-out-on-write: Better for read-heavy workloads
   - Fan-out-on-read: Better for write-heavy workloads

2. **Index Optimization**
   - All indexes are created by `init` command
   - Monitor slow queries with PostgreSQL logs
   - Consider composite indexes for complex queries

3. **Pagination**
   - Always use `limit` and `offset` parameters
   - Keep page sizes reasonable (< 200 items)
   - Use cursor-based pagination for large datasets

4. **Caching**
   - Cache user feeds in Redis for high-traffic scenarios
   - Set appropriate TTLs based on activity frequency
   - Invalidate cache on new activities

5. **Cleanup**
   - Run regular cleanup to remove old activities
   - Schedule during low-traffic periods
   - Monitor database size growth

6. **Multi-App Isolation**
   - Use `X-Source-Account-Id` header consistently
   - Indexes are optimized for multi-tenancy
   - Consider separate databases for very large tenants

---

## Integration Guide

### With Auth Plugin

```typescript
// After user login, create login activity
await fetch('http://localhost:3503/v1/activities', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    actor_id: userId,
    verb: 'logged_in',
    object_type: 'session',
    object_id: sessionId,
    message: 'User logged in'
  })
});
```

### With Social Plugin

```typescript
// When user creates a post
const postActivity = await createActivity({
  actor_id: userId,
  verb: 'created',
  object_type: 'post',
  object_id: postId,
  source_plugin: 'social'
});

// When someone likes the post
await createActivity({
  actor_id: likerId,
  verb: 'liked',
  object_type: 'post',
  object_id: postId,
  target_type: 'user',
  target_id: userId
});
```

### With CMS Plugin

```typescript
// Content published
await createActivity({
  actor_id: authorId,
  verb: 'published',
  object_type: 'article',
  object_id: articleId,
  message: `Published: ${articleTitle}`,
  source_plugin: 'cms'
});
```

---

## License

Source-Available License - See LICENSE file for details.

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/activity-feed
