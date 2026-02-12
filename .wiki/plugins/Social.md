# Social Plugin

Universal social features plugin with posts, threaded comments, reactions, follows, bookmarks, shares, and trending hashtags for any application.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Webhooks](#webhooks)
- [Features](#features)
- [Troubleshooting](#troubleshooting)

---

## Overview

| Field | Value |
|-------|-------|
| **Version** | 1.0.0 |
| **Category** | content |
| **Port** | 3502 |
| **License** | Source-Available |
| **Min nself Version** | 0.4.8 |
| **Multi-App** | Yes (`source_account_id`) |

The Social plugin provides a complete social interaction layer that can be embedded into any application. It supports posts with hashtags and mentions, threaded comments with configurable depth limits, emoji reactions with per-target summaries, user/tag/category follows, bookmarks with collections, shares (repost/quote), user profile aggregation, and trending hashtag discovery. Content editing is governed by a configurable edit window.

---

## Quick Start

```bash
nself plugin install social
nself plugin social init
nself plugin social server
nself plugin social status
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
| `SOCIAL_PLUGIN_PORT` | `3502` | Server port |
| `SOCIAL_MAX_POST_LENGTH` | `5000` | Maximum post content length (characters) |
| `SOCIAL_MAX_COMMENT_LENGTH` | `2000` | Maximum comment content length (characters) |
| `SOCIAL_MAX_COMMENT_DEPTH` | `5` | Maximum nesting depth for threaded comments |
| `SOCIAL_EDIT_WINDOW_MINUTES` | `30` | Minutes after creation during which content can be edited |
| `SOCIAL_REACTIONS_ALLOWED` | See below | Comma-separated list of allowed reaction types |
| `SOCIAL_API_KEY` | - | API key for authentication |
| `SOCIAL_RATE_LIMIT_MAX` | `200` | Rate limit max requests |
| `SOCIAL_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window |

**Default allowed reactions:** thumbs_up, heart, laughing, surprised, crying, fire

---

## CLI Commands

| Command | Description | Options |
|---------|-------------|---------|
| `init` | Initialize database schema | - |
| `server` | Start the API server | `-p, --port <port>`, `-h, --host <host>` |
| `status` | Show plugin status and record counts | - |
| `posts` | List recent posts | `-a, --author <author_id>`, `-t, --hashtag <hashtag>`, `-l, --limit <limit>` |
| `comments` | List recent comments | `-t, --target-type <type>`, `-i, --target-id <id>`, `-a, --author <author_id>`, `-l, --limit <limit>` |
| `reactions` | Show reactions for a target | `-t, --target-type <type>` (required), `-i, --target-id <id>` (required) |
| `follows` | Show follow relationships | `--followers <user_id>`, `--following <user_id>` |
| `stats` | Show detailed statistics and trending hashtags | `-u, --user <user_id>` (for user profile) |

---

## REST API

### Posts

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/posts` | Create a post (enforces max length) |
| `GET` | `/v1/posts` | List posts with filters (author, hashtag, visibility) |
| `GET` | `/v1/posts/:id` | Get a single post |
| `PUT` | `/v1/posts/:id` | Update a post (within edit window) |
| `DELETE` | `/v1/posts/:id` | Soft-delete a post |
| `GET` | `/v1/posts/:id/comments` | List comments on a post |

### Comments

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/comments` | Create a comment (enforces max length and depth) |
| `GET` | `/v1/comments/:id` | Get comment with replies |
| `PUT` | `/v1/comments/:id` | Update a comment (within edit window) |
| `DELETE` | `/v1/comments/:id` | Soft-delete a comment |

### Reactions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/reactions` | Add a reaction (validates allowed types) |
| `DELETE` | `/v1/reactions` | Remove a reaction (query params: target_type, target_id, user_id) |
| `GET` | `/v1/reactions` | Get reaction summaries with user lists |

### Follows

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/follows` | Create a follow (user, tag, or category) |
| `DELETE` | `/v1/follows` | Unfollow (query params: follower_id, following_type, following_id) |
| `GET` | `/v1/follows/followers/:userId` | List followers of a user |
| `GET` | `/v1/follows/following/:userId` | List who a user is following |
| `GET` | `/v1/follows/check` | Check if a follow relationship exists |

### Bookmarks

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/bookmarks` | Create a bookmark (with optional collection and note) |
| `DELETE` | `/v1/bookmarks` | Remove a bookmark (query params: user_id, target_type, target_id) |
| `GET` | `/v1/bookmarks` | List bookmarks with filters (user_id, target_type, collection) |

### Shares

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/shares` | Create a share (repost or quote) |

### Profiles and Trending

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/users/:userId/profile` | Get user profile (post/follower/following/bookmark counts) |
| `GET` | `/v1/trending` | Get trending hashtags (last 7 days) |

### Health and Status

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/webhooks/social` | Receive webhook events |
| `GET` | `/status` | Plugin status with statistics |
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (verifies database) |
| `GET` | `/live` | Liveness check with stats |

---

## Database Schema

### `social_posts`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `author_id` | VARCHAR(255) | Post author ID |
| `content` | TEXT | Post content |
| `content_type` | VARCHAR(32) | Content type (text, image, video, link) |
| `attachments` | JSONB | Attachment objects array |
| `visibility` | VARCHAR(16) | Visibility (public, private, followers, unlisted) |
| `hashtags` | TEXT[] | Hashtag array |
| `mentions` | TEXT[] | Mentioned user IDs |
| `location` | JSONB | Location data |
| `comment_count` | INTEGER | Denormalized comment count |
| `reaction_count` | INTEGER | Denormalized reaction count |
| `share_count` | INTEGER | Denormalized share count |
| `bookmark_count` | INTEGER | Denormalized bookmark count |
| `is_pinned` | BOOLEAN | Whether post is pinned |
| `edited_at` | TIMESTAMPTZ | Last edit timestamp |
| `deleted_at` | TIMESTAMPTZ | Soft-delete timestamp |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

**Indexes:** account, author_id, created_at DESC, visibility, GIN on hashtags, partial index on deleted_at IS NULL.

### `social_comments`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `target_type` | VARCHAR(64) | Target type (post, comment, etc.) |
| `target_id` | VARCHAR(255) | Target object ID |
| `parent_id` | UUID | Self-referencing FK for threading (CASCADE delete) |
| `author_id` | VARCHAR(255) | Comment author ID |
| `content` | TEXT | Comment content |
| `mentions` | TEXT[] | Mentioned user IDs |
| `reaction_count` | INTEGER | Denormalized reaction count |
| `reply_count` | INTEGER | Denormalized reply count |
| `depth` | INTEGER | Nesting depth (0 = top-level) |
| `edited_at` | TIMESTAMPTZ | Last edit timestamp |
| `deleted_at` | TIMESTAMPTZ | Soft-delete timestamp |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

**Indexes:** account, (target_type, target_id), parent_id, author_id, created_at DESC, partial index on deleted_at IS NULL.

### `social_reactions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `target_type` | VARCHAR(64) | Target type |
| `target_id` | VARCHAR(255) | Target object ID |
| `user_id` | VARCHAR(255) | Reacting user |
| `reaction_type` | VARCHAR(32) | Reaction emoji/type |
| `created_at` | TIMESTAMPTZ | Reaction timestamp |

**Unique constraint:** `(source_account_id, target_type, target_id, user_id, reaction_type)`. **Indexes:** account, (target_type, target_id), user_id, reaction_type.

### `social_follows`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `follower_id` | VARCHAR(255) | Following user ID |
| `following_type` | VARCHAR(32) | Type (user, tag, category) |
| `following_id` | VARCHAR(255) | Followed entity ID |
| `created_at` | TIMESTAMPTZ | Follow timestamp |

**Unique constraint:** `(source_account_id, follower_id, following_type, following_id)`. **Indexes:** account, follower_id, (following_type, following_id).

### `social_bookmarks`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | Bookmarking user |
| `target_type` | VARCHAR(64) | Target type |
| `target_id` | VARCHAR(255) | Target object ID |
| `collection` | VARCHAR(128) | Bookmark collection name (default: `default`) |
| `note` | TEXT | User note on bookmark |
| `created_at` | TIMESTAMPTZ | Bookmark timestamp |

**Unique constraint:** `(source_account_id, user_id, target_type, target_id)`. **Indexes:** account, user_id, (target_type, target_id), collection.

### `social_shares`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | Sharing user |
| `target_type` | VARCHAR(64) | Target type |
| `target_id` | VARCHAR(255) | Target object ID |
| `share_type` | VARCHAR(16) | Share type (repost, quote) |
| `message` | TEXT | Quote message (for quote shares) |
| `created_at` | TIMESTAMPTZ | Share timestamp |

**Indexes:** account, user_id, (target_type, target_id), share_type.

### `social_webhook_events`

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `event_type` | VARCHAR(128) | Event type |
| `payload` | JSONB | Event payload |
| `processed` | BOOLEAN | Processing status |
| `processed_at` | TIMESTAMPTZ | Processing timestamp |
| `error` | TEXT | Error message |
| `created_at` | TIMESTAMPTZ | Event timestamp |

---

## Webhooks

### Supported Events

| Event | Description |
|-------|-------------|
| `post.created` | New post created |
| `post.updated` | Post updated |
| `post.deleted` | Post deleted |
| `comment.created` | New comment added |
| `comment.updated` | Comment updated |
| `comment.deleted` | Comment deleted |
| `reaction.added` | Reaction added to target |
| `reaction.removed` | Reaction removed from target |
| `follow.created` | New follow relationship |
| `follow.deleted` | Unfollow |
| `bookmark.created` | New bookmark |
| `bookmark.deleted` | Bookmark removed |
| `share.created` | New share/repost |

---

## Features

- **Posts** with content types, hashtags, mentions, attachments, visibility controls, and location
- **Threaded comments** with self-referencing parent_id, configurable max depth (default 5), and reply counting
- **Emoji reactions** with configurable allowed types, unique constraints, and per-target aggregation via ARRAY_AGG
- **Follows** supporting user, tag, and category types with duplicate-safe upserts
- **Bookmarks** organized into collections with optional user notes
- **Shares** supporting repost and quote types with optional messages
- **Edit window enforcement** preventing edits after configurable timeout (default 30 minutes)
- **Content length limits** for posts (5000 chars) and comments (2000 chars)
- **Denormalized counters** on posts (comment/reaction/share/bookmark counts) auto-updated on CRUD
- **Soft-delete** for posts and comments (sets `deleted_at`, preserves data)
- **User profiles** with aggregated post, follower, following, and bookmark counts
- **Trending hashtags** from last 7 days using PostgreSQL `unnest` and aggregation
- **GIN index on hashtags** for fast hashtag-based post filtering
- **Multi-app isolation** via `source_account_id` on all tables

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Edit returns 403 | Edit window has expired; check `SOCIAL_EDIT_WINDOW_MINUTES` setting |
| Reaction rejected | Reaction type not in allowed list; check `SOCIAL_REACTIONS_ALLOWED` |
| Comment depth limit reached | Max depth is configurable via `SOCIAL_MAX_COMMENT_DEPTH` (default 5) |
| Post content too long | Increase `SOCIAL_MAX_POST_LENGTH` or truncate content |
| Trending empty | Trending looks at last 7 days; ensure posts have hashtags |
| Deleted posts still visible | Posts use soft-delete; queries filter `deleted_at IS NULL` automatically |
| Duplicate reaction error | Reactions have a unique constraint; same user/target/type is upserted |
