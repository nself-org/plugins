# Social Plugin for nself

Universal social features plugin providing posts, comments, reactions, follows, bookmarks, and shares with multi-app support.

## Features

- **Posts**: Create, update, delete posts with rich content (text, images, videos, links, polls)
- **Comments**: Threaded comments on any resource with configurable depth
- **Reactions**: Add emoji reactions to posts and comments
- **Follows**: Follow users, tags, or categories
- **Bookmarks**: Save posts or any content to collections
- **Shares**: Repost or quote content
- **Multi-App Support**: Isolate data by `source_account_id`
- **Generic Targets**: Comments and reactions work on any resource type

## Quick Start

### Installation

```bash
cd plugins/social/ts
npm install
npm run build
```

### Configuration

```bash
cp .env.example .env
# Edit .env with your database credentials
```

Required environment variables:
- `DATABASE_URL` or `POSTGRES_*` settings

### Initialize Database

```bash
npm run build
node dist/cli.js init
```

### Start Server

```bash
npm run dev
# or for production:
npm start
```

Server will listen on port 3502 by default.

## API Endpoints

### Health Checks
- `GET /health` - Basic health check
- `GET /ready` - Readiness check with database connectivity
- `GET /live` - Liveness check with statistics
- `GET /status` - Full status with statistics

### Posts
- `POST /v1/posts` - Create a post
- `GET /v1/posts` - List posts (filter by author, hashtag, visibility)
- `GET /v1/posts/:id` - Get a post
- `PUT /v1/posts/:id` - Update a post
- `DELETE /v1/posts/:id` - Delete a post
- `GET /v1/posts/:id/comments` - Get post comments

### Comments
- `POST /v1/comments` - Create a comment
- `GET /v1/comments/:id` - Get a comment with replies
- `PUT /v1/comments/:id` - Update a comment
- `DELETE /v1/comments/:id` - Delete a comment

### Reactions
- `POST /v1/reactions` - Add a reaction
- `DELETE /v1/reactions` - Remove a reaction
- `GET /v1/reactions` - Get reactions (grouped by type)

### Follows
- `POST /v1/follows` - Follow a user/tag/category
- `DELETE /v1/follows` - Unfollow
- `GET /v1/follows/followers/:userId` - Get followers
- `GET /v1/follows/following/:userId` - Get following
- `GET /v1/follows/check` - Check if following

### Bookmarks
- `POST /v1/bookmarks` - Bookmark content
- `DELETE /v1/bookmarks` - Remove bookmark
- `GET /v1/bookmarks` - List bookmarks

### Shares
- `POST /v1/shares` - Share/repost content

### Analytics
- `GET /v1/users/:userId/profile` - Get user profile with counts
- `GET /v1/trending` - Get trending hashtags

### Webhooks
- `POST /webhooks/social` - Receive webhook events

## CLI Commands

```bash
# Initialize database schema
nself-social init

# Start server
nself-social server [--port 3502] [--host 0.0.0.0]

# Show status and statistics
nself-social status

# List posts
nself-social posts [--author <id>] [--hashtag <tag>] [--limit 10]

# List comments
nself-social comments [--target-type <type>] [--target-id <id>] [--author <id>]

# Show reactions
nself-social reactions --target-type <type> --target-id <id>

# Show follows
nself-social follows [--followers <user_id>] [--following <user_id>]

# Show statistics
nself-social stats [--user <user_id>]
```

## Database Schema

### Tables

1. **social_posts** - Posts with content, visibility, engagement counts
2. **social_comments** - Threaded comments on any resource
3. **social_reactions** - Emoji reactions on posts/comments
4. **social_follows** - Follow relationships (user/tag/category)
5. **social_bookmarks** - Saved content with collections
6. **social_shares** - Reposts and quotes
7. **social_webhook_events** - Webhook event log

### Multi-App Support

All tables include `source_account_id` for data isolation:
- Default value: `'primary'`
- Set via `X-App-Context` header or query parameter
- Queries automatically filtered by context

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection URL |
| `SOCIAL_PLUGIN_PORT` | No | 3502 | Server port |
| `SOCIAL_MAX_POST_LENGTH` | No | 5000 | Maximum post content length |
| `SOCIAL_MAX_COMMENT_LENGTH` | No | 2000 | Maximum comment content length |
| `SOCIAL_MAX_COMMENT_DEPTH` | No | 5 | Maximum comment nesting depth |
| `SOCIAL_EDIT_WINDOW_MINUTES` | No | 30 | Time window for editing posts/comments |
| `SOCIAL_REACTIONS_ALLOWED` | No | 👍,❤️,😂,😮,😢,🔥 | Allowed reaction emojis |
| `SOCIAL_API_KEY` | No | - | API key for authentication |
| `SOCIAL_RATE_LIMIT_MAX` | No | 200 | Rate limit max requests |
| `SOCIAL_RATE_LIMIT_WINDOW_MS` | No | 60000 | Rate limit window (1 minute) |

## Usage Examples

### Create a Post

```bash
curl -X POST http://localhost:3502/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "author_id": "user123",
    "content": "Hello world! #greeting",
    "content_type": "text",
    "visibility": "public",
    "hashtags": ["greeting"]
  }'
```

### Add a Comment

```bash
curl -X POST http://localhost:3502/v1/comments \
  -H "Content-Type: application/json" \
  -d '{
    "target_type": "post",
    "target_id": "<post-uuid>",
    "author_id": "user456",
    "content": "Great post!"
  }'
```

### Add a Reaction

```bash
curl -X POST http://localhost:3502/v1/reactions \
  -H "Content-Type: application/json" \
  -d '{
    "target_type": "post",
    "target_id": "<post-uuid>",
    "user_id": "user789",
    "reaction_type": "❤️"
  }'
```

### Follow a User

```bash
curl -X POST http://localhost:3502/v1/follows \
  -H "Content-Type: application/json" \
  -d '{
    "follower_id": "user123",
    "following_type": "user",
    "following_id": "user456"
  }'
```

### Bookmark a Post

```bash
curl -X POST http://localhost:3502/v1/bookmarks \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "target_type": "post",
    "target_id": "<post-uuid>",
    "collection": "favorites",
    "note": "Important post"
  }'
```

## Key Features

### Generic Comments

Comments can be attached to ANY resource type, not just posts:

```json
{
  "target_type": "video",
  "target_id": "vid123",
  "content": "Amazing video!"
}
```

### Threaded Comments

Comments support nesting with configurable depth:

```json
{
  "target_type": "post",
  "target_id": "post123",
  "parent_id": "comment-uuid",
  "content": "Reply to comment"
}
```

### Visibility Control

Posts support three visibility levels:
- `public` - Everyone can see
- `followers` - Only followers can see
- `private` - Only author can see

### Trending Hashtags

Automatically tracks hashtag usage and provides trending data:

```bash
curl http://localhost:3502/v1/trending?limit=10
```

## Development

```bash
# Install dependencies
npm install

# Type check
npm run typecheck

# Build
npm run build

# Watch mode
npm run watch

# Development server with auto-reload
npm run dev
```

## License

MIT
