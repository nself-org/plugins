# Social Plugin

Universal social features plugin with posts, comments, reactions, follows, and bookmarks for any application.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [TypeScript Implementation](#typescript-implementation)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Social plugin provides complete social networking features that can be integrated into any application. It supports:

- **7 Database Tables** - Posts, comments, reactions, follows, bookmarks, shares
- **13 Webhook Events** - Real-time social activity notifications
- **Full REST API** - Complete social operations
- **CLI Interface** - Command-line social management
- **Flexible Content** - Text, media attachments, hashtags, mentions
- **Nested Comments** - Configurable comment depth
- **Custom Reactions** - Configurable emoji reactions
- **Privacy Controls** - Public, private, followers-only visibility

### Key Features

| Feature | Description |
|---------|-------------|
| Posts | Create text posts with media attachments |
| Comments | Nested comments with configurable depth |
| Reactions | Emoji reactions (like, love, laugh, etc.) |
| Follows | User follow/unfollow relationships |
| Bookmarks | Save posts for later |
| Shares | Repost/share content |
| Hashtags | Tag content with hashtags |
| Mentions | @ mention other users |
| Edit Window | Time-limited post/comment editing |
| Soft Deletes | Preserve deleted content |

---

## Quick Start

```bash
# Install the plugin
nself plugin install social

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env

# Initialize database schema
nself plugin social init

# Start server
nself plugin social server --port 3502
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `SOCIAL_PLUGIN_PORT` | No | `3502` | HTTP server port |
| `SOCIAL_MAX_POST_LENGTH` | No | `5000` | Maximum post length (characters) |
| `SOCIAL_MAX_COMMENT_LENGTH` | No | `2000` | Maximum comment length (characters) |
| `SOCIAL_MAX_COMMENT_DEPTH` | No | `5` | Maximum comment nesting depth |
| `SOCIAL_EDIT_WINDOW_MINUTES` | No | `30` | Post/comment edit window (minutes) |
| `SOCIAL_REACTIONS_ALLOWED` | No | `👍,❤️,😂,😮,😢,🔥` | Comma-separated emoji reactions |
| `SOCIAL_API_KEY` | No | - | API key for authentication |
| `SOCIAL_RATE_LIMIT_MAX` | No | `100` | Max API requests per window |
| `SOCIAL_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Social Settings
SOCIAL_MAX_POST_LENGTH=5000
SOCIAL_MAX_COMMENT_LENGTH=2000
SOCIAL_MAX_COMMENT_DEPTH=5
SOCIAL_EDIT_WINDOW_MINUTES=30
SOCIAL_REACTIONS_ALLOWED=👍,❤️,😂,😮,😢,🔥,🎉,💯

# Server
SOCIAL_PLUGIN_PORT=3502
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin social init

# Start server
nself plugin social server

# Custom port
nself plugin social server --port 8080

# Check status
nself plugin social status

# View statistics
nself plugin social stats
```

### Post Management

```bash
# List posts
nself plugin social posts list

# Filter by author
nself plugin social posts list --author user123

# Filter by hashtag
nself plugin social posts list --hashtag sports

# Get post by ID
nself plugin social posts get <post-id>

# Delete post
nself plugin social posts delete <post-id>
```

### Comment Management

```bash
# List comments for post
nself plugin social comments list --post <post-id>

# Get comment by ID
nself plugin social comments get <comment-id>

# Delete comment
nself plugin social comments delete <comment-id>
```

### Reaction Management

```bash
# List reactions for post
nself plugin social reactions list --post <post-id>

# List reactions by user
nself plugin social reactions list --user user123
```

### Follow Management

```bash
# List followers
nself plugin social follows list --user user123 --type followers

# List following
nself plugin social follows list --user user123 --type following
```

---

## REST API

### Health & Status

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "social",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "social",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /v1/status
Plugin status with statistics.

**Response:**
```json
{
  "plugin": "social",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "posts": 1250,
    "comments": 3500,
    "reactions": 8200,
    "follows": 450,
    "bookmarks": 620
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Posts

#### POST /v1/posts
Create new post.

**Request Body:**
```json
{
  "author_id": "user123",
  "content": "Hello world! #intro #welcome",
  "content_type": "text",
  "visibility": "public",
  "attachments": [
    {
      "type": "image",
      "url": "https://example.com/image.jpg",
      "width": 1920,
      "height": 1080
    }
  ],
  "location": {
    "name": "San Francisco, CA",
    "lat": 37.7749,
    "lng": -122.4194
  },
  "metadata": {}
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "source_account_id": "primary",
    "author_id": "user123",
    "content": "Hello world! #intro #welcome",
    "content_type": "text",
    "visibility": "public",
    "hashtags": ["intro", "welcome"],
    "mentions": [],
    "comment_count": 0,
    "reaction_count": 0,
    "share_count": 0,
    "bookmark_count": 0,
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### GET /v1/posts
List posts.

**Query Parameters:**
- `author_id` (optional) - Filter by author
- `hashtag` (optional) - Filter by hashtag
- `visibility` (optional) - Filter by visibility
- `since` (optional) - Filter by creation date
- `limit` (optional) - Max results (default: 50)
- `offset` (optional) - Pagination offset

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "author_id": "user123",
      "content": "Hello world! #intro",
      "comment_count": 5,
      "reaction_count": 12,
      "created_at": "2026-02-11T10:00:00.000Z"
    }
  ],
  "pagination": {
    "total": 1250,
    "limit": 50,
    "offset": 0
  }
}
```

#### GET /v1/posts/:id
Get post details.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "source_account_id": "primary",
    "author_id": "user123",
    "content": "Hello world! #intro #welcome",
    "content_type": "text",
    "attachments": [...],
    "visibility": "public",
    "hashtags": ["intro", "welcome"],
    "mentions": [],
    "location": {...},
    "comment_count": 5,
    "reaction_count": 12,
    "share_count": 3,
    "bookmark_count": 8,
    "is_pinned": false,
    "edited_at": null,
    "deleted_at": null,
    "metadata": {},
    "created_at": "2026-02-11T10:00:00.000Z",
    "updated_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### PUT /v1/posts/:id
Update post.

**Request Body:**
```json
{
  "content": "Updated content",
  "visibility": "followers"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "content": "Updated content",
    "visibility": "followers",
    "edited_at": "2026-02-11T10:15:00.000Z"
  }
}
```

#### DELETE /v1/posts/:id
Delete post (soft delete).

**Response:**
```json
{
  "success": true
}
```

#### POST /v1/posts/:id/pin
Pin post.

**Response:**
```json
{
  "success": true
}
```

#### POST /v1/posts/:id/unpin
Unpin post.

**Response:**
```json
{
  "success": true
}
```

### Comments

#### POST /v1/comments
Create comment.

**Request Body:**
```json
{
  "target_type": "post",
  "target_id": "uuid",
  "parent_id": null,
  "author_id": "user456",
  "content": "Great post! @user123",
  "mentions": ["user123"]
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "target_type": "post",
    "target_id": "uuid",
    "parent_id": null,
    "author_id": "user456",
    "content": "Great post! @user123",
    "mentions": ["user123"],
    "reaction_count": 0,
    "reply_count": 0,
    "depth": 0,
    "created_at": "2026-02-11T10:05:00.000Z"
  }
}
```

#### GET /v1/comments
List comments.

**Query Parameters:**
- `target_type` (optional) - Filter by target type
- `target_id` (optional) - Filter by target ID
- `parent_id` (optional) - Filter by parent (null for top-level)
- `author_id` (optional) - Filter by author
- `limit` (optional) - Max results (default: 100)
- `offset` (optional) - Pagination offset

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "target_type": "post",
      "target_id": "uuid",
      "parent_id": null,
      "author_id": "user456",
      "content": "Great post!",
      "reaction_count": 3,
      "reply_count": 2,
      "depth": 0,
      "created_at": "2026-02-11T10:05:00.000Z"
    }
  ]
}
```

#### GET /v1/comments/:id
Get comment details.

#### PUT /v1/comments/:id
Update comment.

**Request Body:**
```json
{
  "content": "Updated comment"
}
```

#### DELETE /v1/comments/:id
Delete comment (soft delete).

**Response:**
```json
{
  "success": true
}
```

#### GET /v1/comments/:id/replies
Get comment replies (nested comments).

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "parent_id": "parent-uuid",
      "author_id": "user789",
      "content": "Thanks!",
      "depth": 1,
      "created_at": "2026-02-11T10:10:00.000Z"
    }
  ]
}
```

### Reactions

#### POST /v1/reactions
Add reaction.

**Request Body:**
```json
{
  "target_type": "post",
  "target_id": "uuid",
  "user_id": "user123",
  "reaction": "❤️"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "target_type": "post",
    "target_id": "uuid",
    "user_id": "user123",
    "reaction": "❤️",
    "created_at": "2026-02-11T10:06:00.000Z"
  }
}
```

#### GET /v1/reactions
List reactions.

**Query Parameters:**
- `target_type` (optional) - Filter by target type
- `target_id` (optional) - Filter by target ID
- `user_id` (optional) - Filter by user
- `reaction` (optional) - Filter by reaction emoji

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "target_type": "post",
      "target_id": "uuid",
      "user_id": "user123",
      "reaction": "❤️",
      "created_at": "2026-02-11T10:06:00.000Z"
    }
  ]
}
```

#### DELETE /v1/reactions/:id
Remove reaction.

**Response:**
```json
{
  "success": true
}
```

#### GET /v1/reactions/summary
Get reaction summary for target.

**Query Parameters:**
- `target_type` (required) - Target type
- `target_id` (required) - Target ID

**Response:**
```json
{
  "success": true,
  "data": {
    "target_type": "post",
    "target_id": "uuid",
    "total": 15,
    "reactions": [
      { "reaction": "❤️", "count": 8 },
      { "reaction": "👍", "count": 5 },
      { "reaction": "😂", "count": 2 }
    ]
  }
}
```

### Follows

#### POST /v1/follows
Follow user.

**Request Body:**
```json
{
  "follower_id": "user123",
  "following_id": "user456"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "follower_id": "user123",
    "following_id": "user456",
    "created_at": "2026-02-11T10:07:00.000Z"
  }
}
```

#### GET /v1/follows
List follows.

**Query Parameters:**
- `follower_id` (optional) - Get users this user follows
- `following_id` (optional) - Get followers of this user
- `limit` (optional) - Max results (default: 100)

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "follower_id": "user123",
      "following_id": "user456",
      "created_at": "2026-02-11T10:07:00.000Z"
    }
  ]
}
```

#### DELETE /v1/follows/:id
Unfollow user.

**Response:**
```json
{
  "success": true
}
```

### Bookmarks

#### POST /v1/bookmarks
Bookmark post.

**Request Body:**
```json
{
  "user_id": "user123",
  "target_type": "post",
  "target_id": "uuid"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_id": "user123",
    "target_type": "post",
    "target_id": "uuid",
    "created_at": "2026-02-11T10:08:00.000Z"
  }
}
```

#### GET /v1/bookmarks
List bookmarks.

**Query Parameters:**
- `user_id` (optional) - Filter by user
- `target_type` (optional) - Filter by target type
- `limit` (optional) - Max results (default: 100)

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "user_id": "user123",
      "target_type": "post",
      "target_id": "uuid",
      "created_at": "2026-02-11T10:08:00.000Z"
    }
  ]
}
```

#### DELETE /v1/bookmarks/:id
Remove bookmark.

**Response:**
```json
{
  "success": true
}
```

### Shares

#### POST /v1/shares
Share/repost content.

**Request Body:**
```json
{
  "user_id": "user123",
  "source_type": "post",
  "source_id": "uuid",
  "comment": "Check this out!"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_id": "user123",
    "source_type": "post",
    "source_id": "uuid",
    "comment": "Check this out!",
    "created_at": "2026-02-11T10:09:00.000Z"
  }
}
```

#### GET /v1/shares
List shares.

**Query Parameters:**
- `user_id` (optional) - Filter by user
- `source_type` (optional) - Filter by source type
- `source_id` (optional) - Filter by source ID

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "user_id": "user123",
      "source_type": "post",
      "source_id": "uuid",
      "comment": "Check this out!",
      "created_at": "2026-02-11T10:09:00.000Z"
    }
  ]
}
```

### Trending & Discovery

#### GET /v1/trending/hashtags
Get trending hashtags.

**Query Parameters:**
- `limit` (optional) - Max results (default: 20)
- `period` (optional) - Time period (`day`, `week`, `month`, default: `day`)

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "hashtag": "sports",
      "count": 150,
      "trend": "up"
    },
    {
      "hashtag": "news",
      "count": 120,
      "trend": "stable"
    }
  ]
}
```

#### GET /v1/feed/:userId
Get personalized feed for user.

**Query Parameters:**
- `limit` (optional) - Max results (default: 50)
- `offset` (optional) - Pagination offset

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "author_id": "user456",
      "content": "...",
      "created_at": "2026-02-11T10:00:00.000Z"
    }
  ]
}
```

### Statistics

#### GET /v1/stats
Get platform statistics.

**Response:**
```json
{
  "success": true,
  "data": {
    "posts": 1250,
    "comments": 3500,
    "reactions": 8200,
    "follows": 450,
    "bookmarks": 620,
    "shares": 280,
    "activeUsers": 350,
    "topHashtags": ["sports", "news", "tech"]
  }
}
```

#### GET /v1/users/:userId/stats
Get user statistics.

**Response:**
```json
{
  "success": true,
  "data": {
    "user_id": "user123",
    "posts": 50,
    "comments": 120,
    "reactions_given": 300,
    "reactions_received": 450,
    "followers": 75,
    "following": 42,
    "bookmarks": 28
  }
}
```

---

## Webhook Events

| Event | Description | Payload |
|-------|-------------|---------|
| `post.created` | New post created | `{ post_id, author_id, hashtags }` |
| `post.updated` | Post updated | `{ post_id, changes }` |
| `post.deleted` | Post deleted | `{ post_id }` |
| `comment.created` | New comment | `{ comment_id, post_id, author_id }` |
| `comment.updated` | Comment updated | `{ comment_id, changes }` |
| `comment.deleted` | Comment deleted | `{ comment_id }` |
| `reaction.added` | Reaction added | `{ reaction_id, target_type, target_id, reaction }` |
| `reaction.removed` | Reaction removed | `{ reaction_id }` |
| `follow.created` | User followed | `{ follower_id, following_id }` |
| `follow.deleted` | User unfollowed | `{ follower_id, following_id }` |
| `bookmark.created` | Content bookmarked | `{ bookmark_id, user_id, target_type, target_id }` |
| `bookmark.deleted` | Bookmark removed | `{ bookmark_id }` |
| `share.created` | Content shared | `{ share_id, user_id, source_type, source_id }` |

### Webhook Payload Example

```json
{
  "id": "evt_abc123",
  "type": "post.created",
  "created": 1707649200,
  "data": {
    "post_id": "uuid",
    "author_id": "user123",
    "content": "Hello world!",
    "hashtags": ["intro", "welcome"],
    "visibility": "public"
  }
}
```

---

## Database Schema

### social_posts

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `author_id` | VARCHAR(255) | Post author |
| `content` | TEXT | Post content |
| `content_type` | VARCHAR(32) | Content type (text, html, markdown) |
| `attachments` | JSONB | Media attachments |
| `visibility` | VARCHAR(16) | Visibility (public, private, followers) |
| `hashtags` | TEXT[] | Hashtag array |
| `mentions` | TEXT[] | Mentioned users |
| `location` | JSONB | Location data |
| `comment_count` | INTEGER | Comment count |
| `reaction_count` | INTEGER | Reaction count |
| `share_count` | INTEGER | Share count |
| `bookmark_count` | INTEGER | Bookmark count |
| `is_pinned` | BOOLEAN | Pinned flag |
| `edited_at` | TIMESTAMPTZ | Last edit timestamp |
| `deleted_at` | TIMESTAMPTZ | Soft delete timestamp |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |

**Indexes:**
- `idx_social_posts_source_account` - source_account_id
- `idx_social_posts_author` - author_id
- `idx_social_posts_created` - created_at DESC
- `idx_social_posts_visibility` - visibility
- `idx_social_posts_hashtags` - hashtags (GIN)
- `idx_social_posts_deleted` - deleted_at (partial)

### social_comments

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `target_type` | VARCHAR(64) | Target type |
| `target_id` | VARCHAR(255) | Target ID |
| `parent_id` | UUID | Parent comment reference |
| `author_id` | VARCHAR(255) | Comment author |
| `content` | TEXT | Comment content |
| `mentions` | TEXT[] | Mentioned users |
| `reaction_count` | INTEGER | Reaction count |
| `reply_count` | INTEGER | Reply count |
| `depth` | INTEGER | Nesting depth |
| `edited_at` | TIMESTAMPTZ | Last edit timestamp |
| `deleted_at` | TIMESTAMPTZ | Soft delete timestamp |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |

**Indexes:**
- `idx_social_comments_source_account` - source_account_id
- `idx_social_comments_target` - (target_type, target_id)
- `idx_social_comments_parent` - parent_id
- `idx_social_comments_author` - author_id
- `idx_social_comments_created` - created_at DESC
- `idx_social_comments_deleted` - deleted_at (partial)

### social_reactions

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `target_type` | VARCHAR(64) | Target type |
| `target_id` | VARCHAR(255) | Target ID |
| `user_id` | VARCHAR(255) | User ID |
| `reaction` | VARCHAR(8) | Emoji reaction |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_social_reactions_source_account` - source_account_id
- `idx_social_reactions_target` - (target_type, target_id)
- `idx_social_reactions_user` - user_id
- `idx_social_reactions_reaction` - reaction

**Unique Constraint:**
- `(source_account_id, target_type, target_id, user_id)` - One reaction per user per target

### social_follows

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `follower_id` | VARCHAR(255) | Follower user ID |
| `following_id` | VARCHAR(255) | Following user ID |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_social_follows_source_account` - source_account_id
- `idx_social_follows_follower` - follower_id
- `idx_social_follows_following` - following_id

**Unique Constraint:**
- `(source_account_id, follower_id, following_id)` - One follow per user pair

### social_bookmarks

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `user_id` | VARCHAR(255) | User ID |
| `target_type` | VARCHAR(64) | Target type |
| `target_id` | VARCHAR(255) | Target ID |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_social_bookmarks_source_account` - source_account_id
- `idx_social_bookmarks_user` - user_id
- `idx_social_bookmarks_target` - (target_type, target_id)

**Unique Constraint:**
- `(source_account_id, user_id, target_type, target_id)` - One bookmark per user per target

### social_shares

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `user_id` | VARCHAR(255) | User ID |
| `source_type` | VARCHAR(64) | Source type |
| `source_id` | VARCHAR(255) | Source ID |
| `comment` | TEXT | Optional comment |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_social_shares_source_account` - source_account_id
- `idx_social_shares_user` - user_id
- `idx_social_shares_source` - (source_type, source_id)

### social_webhook_events

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (event ID) |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `event_type` | VARCHAR(128) | Event type |
| `payload` | JSONB | Event payload |
| `processed` | BOOLEAN | Processing status |
| `processed_at` | TIMESTAMPTZ | Processing timestamp |
| `error` | TEXT | Error message if failed |
| `created_at` | TIMESTAMPTZ | Event creation time |

**Indexes:**
- `idx_social_webhook_account` - source_account_id
- `idx_social_webhook_processed` - processed
- `idx_social_webhook_created` - created_at DESC

---

## TypeScript Implementation

### File Structure

```
plugins/social/ts/src/
├── types.ts          # TypeScript interfaces
├── config.ts         # Configuration loading
├── database.ts       # Database operations
├── server.ts         # HTTP server
├── webhooks.ts       # Webhook handlers
├── cli.ts            # CLI commands
└── index.ts          # Module exports
```

### Key Components

#### SocialDatabase (database.ts)
- Schema initialization
- Post/comment CRUD
- Reaction management
- Follow relationships
- Statistics

---

## Examples

### Example 1: Create Post with Media

```typescript
const response = await fetch('http://localhost:3502/v1/posts', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    author_id: 'user123',
    content: 'Check out this amazing sunset! #photography #nature',
    attachments: [
      {
        type: 'image',
        url: 'https://example.com/sunset.jpg',
        width: 1920,
        height: 1080
      }
    ],
    visibility: 'public',
    location: {
      name: 'Golden Gate Bridge',
      lat: 37.8199,
      lng: -122.4783
    }
  })
});

const { data: post } = await response.json();
console.log(`Post created: ${post.id}`);
```

### Example 2: Nested Comments Thread

```typescript
// Create parent comment
const comment1 = await fetch('http://localhost:3502/v1/comments', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    target_type: 'post',
    target_id: 'post-uuid',
    author_id: 'user456',
    content: 'Great post!'
  })
}).then(r => r.json());

// Reply to comment
const comment2 = await fetch('http://localhost:3502/v1/comments', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    target_type: 'post',
    target_id: 'post-uuid',
    parent_id: comment1.data.id,
    author_id: 'user123',
    content: 'Thanks! @user456'
  })
}).then(r => r.json());
```

### Example 3: Query Popular Content

```sql
-- Most reacted posts
SELECT
  p.id,
  p.author_id,
  p.content,
  p.reaction_count,
  p.comment_count,
  p.share_count
FROM social_posts p
WHERE p.deleted_at IS NULL
  AND p.created_at >= NOW() - INTERVAL '7 days'
ORDER BY p.reaction_count DESC
LIMIT 20;

-- Trending hashtags
SELECT
  hashtag,
  COUNT(*) as post_count
FROM social_posts,
  UNNEST(hashtags) as hashtag
WHERE created_at >= NOW() - INTERVAL '24 hours'
  AND deleted_at IS NULL
GROUP BY hashtag
ORDER BY post_count DESC
LIMIT 10;

-- Active users
SELECT
  author_id,
  COUNT(*) as post_count,
  SUM(reaction_count) as total_reactions
FROM social_posts
WHERE created_at >= NOW() - INTERVAL '30 days'
  AND deleted_at IS NULL
GROUP BY author_id
ORDER BY post_count DESC
LIMIT 20;
```

---

## Troubleshooting

### Common Issues

#### Edit Window Expired

**Error:**
```
Error: Edit window has expired
```

**Solution:**
Posts/comments can only be edited within `SOCIAL_EDIT_WINDOW_MINUTES` (default: 30 minutes).

#### Comment Depth Exceeded

**Error:**
```
Error: Maximum comment depth exceeded
```

**Solution:**
Increase `SOCIAL_MAX_COMMENT_DEPTH` or stop nesting comments so deeply.

#### Content Too Long

**Error:**
```
Error: Content exceeds maximum length
```

**Solution:**
Reduce content length or increase limits:
- `SOCIAL_MAX_POST_LENGTH`
- `SOCIAL_MAX_COMMENT_LENGTH`

#### Invalid Reaction

**Error:**
```
Error: Reaction not allowed
```

**Solution:**
Reaction emoji must be in `SOCIAL_REACTIONS_ALLOWED` list.

### Debug Mode

Enable debug logging:
```bash
LOG_LEVEL=debug nself plugin social server
```

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Social
- **Issues**: https://github.com/acamarata/nself-plugins/issues
