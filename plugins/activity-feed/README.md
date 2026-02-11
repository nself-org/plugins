# Activity Feed Plugin

Universal activity feed system for nself with fan-out-on-read/write strategies, aggregation, and subscriptions.

## Features

- **Flexible Feed Strategies**
  - Fan-out-on-read: Query subscriptions at read time (default)
  - Fan-out-on-write: Materialize feed items when activities are created

- **Activity Recording**
  - Record any type of activity with actor, verb, object, and optional target
  - Support for 15+ activity verbs (created, updated, liked, followed, etc.)
  - Store activity metadata in JSONB format
  - Track source plugin for cross-plugin activity feeds

- **User Feeds**
  - Personalized activity feeds based on subscriptions
  - Unread/read tracking
  - Hide unwanted activities
  - Pagination support

- **Subscriptions**
  - Subscribe users to actors, groups, channels, tags, or categories
  - Enable/disable subscriptions dynamically
  - Multiple subscription types

- **Activity Aggregation**
  - Group similar activities within time windows
  - "Alice, Bob, and 3 others liked your post" style aggregation
  - Configurable aggregation window

- **Entity Feeds**
  - Get all activities for a specific object (e.g., post:123, issue:456)
  - Useful for comment threads, activity logs, etc.

- **Multi-Account Support**
  - Isolate activities by source_account_id
  - Full multi-app compatibility

## Installation

```bash
cd plugins/activity-feed/ts
npm install
npm run build
```

## Configuration

Create a `.env` file:

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Optional
FEED_PLUGIN_PORT=3503
FEED_STRATEGY=read
FEED_MAX_FEED_SIZE=200
FEED_AGGREGATION_WINDOW_MINUTES=60
FEED_RETENTION_DAYS=90
FEED_API_KEY=your-secret-key
```

## Database Setup

```bash
npm run init
```

This creates 4 tables:
- `feed_activities` - All recorded activities
- `feed_user_feeds` - Materialized user feed items (fan-out-on-write)
- `feed_subscriptions` - User subscription preferences
- `feed_webhook_events` - Webhook event log

## Usage

### Start Server

```bash
npm start
# or
npm run dev  # Development mode with auto-reload
```

### CLI Commands

```bash
# Initialize database
nself-activity-feed init

# Start server
nself-activity-feed server --port 3503

# View statistics
nself-activity-feed status

# List activities
nself-activity-feed activities --limit 20

# View user's feed
nself-activity-feed feed user123 --limit 20

# List user's subscriptions
nself-activity-feed subscriptions user123

# Subscribe user to an actor
nself-activity-feed subscribe user123 user user456

# Create activity
nself-activity-feed create-activity \
  --actor user123 \
  --verb liked \
  --object post:456 \
  --message "Alice liked your post"

# Trigger fan-out
nself-activity-feed fanout activity-uuid

# Clean up old activities
nself-activity-feed cleanup --days 90
```

## API Endpoints

### Health Checks

- `GET /health` - Basic health check
- `GET /ready` - Readiness check with database connectivity
- `GET /live` - Liveness check with stats
- `GET /v1/status` - Full status with configuration and stats

### Activities

- `POST /v1/activities` - Create activity
- `GET /v1/activities` - List activities (with filters)
- `GET /v1/activities/:id` - Get specific activity
- `DELETE /v1/activities/:id` - Delete activity

### User Feeds

- `GET /v1/feed/:userId` - Get user's activity feed
- `GET /v1/feed/:userId/unread` - Get unread count
- `POST /v1/feed/:userId/read` - Mark items as read
- `POST /v1/feed/:userId/hide` - Hide activity
- `GET /v1/feed/:userId/stats` - Get user feed stats

### Entity Feeds

- `GET /v1/entity/:type/:id/feed` - Get all activities for an entity

### Subscriptions

- `POST /v1/subscriptions` - Create subscription
- `GET /v1/subscriptions/:userId` - List user's subscriptions
- `DELETE /v1/subscriptions/:id` - Delete subscription

### Fan-out

- `POST /v1/fanout` - Manually trigger fan-out for activity

### Statistics

- `GET /v1/stats` - Get global feed statistics

## Examples

### Record Activity

```bash
curl -X POST http://localhost:3503/v1/activities \
  -H "Content-Type: application/json" \
  -d '{
    "actor_id": "user123",
    "verb": "created",
    "object_type": "post",
    "object_id": "post456",
    "message": "Alice created a new post",
    "source_plugin": "cms",
    "data": {
      "title": "My First Post",
      "url": "https://example.com/posts/456"
    }
  }'
```

### Get User Feed

```bash
curl http://localhost:3503/v1/feed/user789?limit=20&includeRead=false
```

### Subscribe User

```bash
curl -X POST http://localhost:3503/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user789",
    "target_type": "user",
    "target_id": "user123",
    "enabled": true
  }'
```

### Mark Items as Read

```bash
curl -X POST http://localhost:3503/v1/feed/user789/read \
  -H "Content-Type: application/json" \
  -d '{
    "activityIds": ["activity-uuid-1", "activity-uuid-2"]
  }'
```

## Activity Verbs

Supported verbs:
- `created` - Created a new object
- `updated` - Updated an object
- `deleted` - Deleted an object
- `liked` - Liked an object
- `commented` - Commented on an object
- `followed` - Followed a user/entity
- `shared` - Shared an object
- `joined` - Joined a group/channel
- `left` - Left a group/channel
- `uploaded` - Uploaded a file
- `published` - Published content
- `mentioned` - Mentioned a user
- `invited` - Invited a user
- `completed` - Completed a task
- `started` - Started an activity

## Fan-out Strategies

### Fan-out-on-Read (Default)

Activities are stored centrally. When a user requests their feed, the system queries activities from actors they subscribe to.

**Pros:**
- Low write cost
- No storage overhead
- Instant subscription changes

**Cons:**
- Higher read cost
- Complex queries for large feeds

**Best for:**
- Twitter/Mastodon style feeds
- Low activity volume
- Frequently changing subscriptions

### Fan-out-on-Write

When an activity is created, feed items are created for all subscribers of the actor.

**Pros:**
- Fast feed reads
- Simple queries
- Predictable performance

**Cons:**
- High write cost for popular users
- Storage overhead
- Subscription changes require backfill

**Best for:**
- Instagram/Facebook style feeds
- High activity volume
- Stable subscriptions

## Aggregation

Activities with `is_aggregatable: true` can be grouped within a time window:

```
"Alice, Bob, and 3 others liked your post"
```

Instead of:
```
"Alice liked your post"
"Bob liked your post"
"Carol liked your post"
...
```

Aggregation is based on:
- Same verb
- Same object_type and object_id
- Within aggregation window (default: 60 minutes)

## Multi-Account Support

Isolate activities by `source_account_id`:

```bash
curl -X POST http://localhost:3503/v1/activities \
  -H "Content-Type: application/json" \
  -H "X-Account-ID: prod" \
  -d '{
    "source_account_id": "prod",
    "actor_id": "user123",
    "verb": "created",
    "object_type": "post",
    "object_id": "456"
  }'
```

## Integration with Other Plugins

The activity-feed plugin is designed to receive activities from other plugins:

```typescript
// From stripe plugin
await fetch('http://localhost:3503/v1/activities', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    actor_id: subscription.customer,
    verb: 'created',
    object_type: 'subscription',
    object_id: subscription.id,
    source_plugin: 'stripe',
    message: 'New subscription created',
    data: {
      plan: subscription.plan.name,
      amount: subscription.plan.amount,
    }
  })
});
```

## Performance Considerations

- Use fan-out-on-read for low-write, high-read scenarios
- Use fan-out-on-write for high-write, even higher read scenarios
- Index `source_account_id` for multi-account setups
- Set appropriate `FEED_RETENTION_DAYS` and run cleanup regularly
- Use `FEED_MAX_FEED_SIZE` to limit feed queries

## License

MIT
