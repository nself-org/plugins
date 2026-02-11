# CMS Plugin

Headless CMS plugin for nself with content types, posts, categories, tags, and version history.

## Features

- **Content Types**: Define custom content types with custom fields
- **Posts**: Full-featured post management with rich metadata
- **Categories**: Hierarchical category system
- **Tags**: Tag-based organization
- **Versioning**: Automatic version history for all post edits
- **Scheduling**: Schedule posts for future publication
- **SEO**: Built-in SEO fields (title, description, keywords, canonical URL)
- **Multi-format**: Support for markdown, HTML, and plaintext
- **Multi-account**: Isolated data per source_account_id
- **REST API**: Complete REST API for all operations
- **RSS/Atom Feeds**: Automatic feed generation

## Installation

```bash
cd plugins/cms/ts
npm install
npm run build
```

## Configuration

Create a `.env` file based on `.env.example`:

```bash
cp .env.example .env
```

Required environment variables:
- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`

Optional:
- `CMS_PLUGIN_PORT` (default: 3501)
- `CMS_API_KEY` (enables API authentication)
- `CMS_MAX_BODY_LENGTH` (default: 500000 bytes)
- `CMS_MAX_VERSIONS` (default: 50)

## Usage

### Initialize Schema

```bash
npm run cli init
```

### Start Server

```bash
npm start
# or for development
npm run dev
```

### CLI Commands

```bash
# Show status
npm run cli status

# List posts
npm run cli posts --list
npm run cli posts --status published

# List categories
npm run cli categories --list
npm run cli categories --tree

# List tags
npm run cli tags --list

# Publish a post
npm run cli publish <post-id>
```

## API Endpoints

### Health & Status
- `GET /health` - Basic health check
- `GET /ready` - Readiness check (verifies DB)
- `GET /live` - Liveness check with stats
- `GET /v1/status` - Full status
- `GET /v1/stats` - Content statistics

### Content Types
- `POST /v1/content-types` - Create content type
- `GET /v1/content-types` - List content types
- `GET /v1/content-types/:id` - Get content type
- `PUT /v1/content-types/:id` - Update content type
- `DELETE /v1/content-types/:id` - Delete content type

### Posts
- `POST /v1/posts` - Create post
- `GET /v1/posts` - List posts (with filters)
- `GET /v1/posts/:id` - Get post
- `GET /v1/posts/slug/:slug` - Get post by slug
- `PUT /v1/posts/:id` - Update post
- `DELETE /v1/posts/:id` - Delete post
- `POST /v1/posts/:id/publish` - Publish post
- `POST /v1/posts/:id/unpublish` - Unpublish post
- `POST /v1/posts/:id/schedule` - Schedule post
- `POST /v1/posts/:id/duplicate` - Duplicate post

### Post Versions
- `GET /v1/posts/:id/versions` - List versions
- `GET /v1/posts/:id/versions/:version` - Get version
- `POST /v1/posts/:id/versions/:version/restore` - Restore version

### Post Relations
- `POST /v1/posts/:id/categories` - Set categories
- `POST /v1/posts/:id/tags` - Set tags

### Categories
- `POST /v1/categories` - Create category
- `GET /v1/categories` - List categories
- `GET /v1/categories?tree=true` - Get category tree
- `GET /v1/categories/:id` - Get category
- `PUT /v1/categories/:id` - Update category
- `DELETE /v1/categories/:id` - Delete category

### Tags
- `POST /v1/tags` - Create tag
- `GET /v1/tags` - List tags
- `GET /v1/tags/:id` - Get tag
- `DELETE /v1/tags/:id` - Delete tag

### Feed
- `GET /v1/feed` - RSS feed (default)
- `GET /v1/feed?format=atom` - Atom feed

## Database Schema

### Tables

1. **cms_content_types** - Custom content type definitions
2. **cms_posts** - All post content
3. **cms_post_versions** - Version history
4. **cms_categories** - Hierarchical categories
5. **cms_tags** - Tags
6. **cms_post_categories** - Post-category relations
7. **cms_post_tags** - Post-tag relations
8. **cms_webhook_events** - Webhook event log

## Example: Create a Post

```bash
curl -X POST http://localhost:3501/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "title": "My First Post",
    "body": "This is the content of my first post.",
    "author_id": "user-123",
    "status": "published",
    "content_type": "post"
  }'
```

## Example: List Published Posts

```bash
curl http://localhost:3501/v1/posts?status=published
```

## Multi-Account Support

The CMS plugin supports multi-account isolation via the `source_account_id` column. Use the `X-Source-Account-Id` header to specify the account:

```bash
curl -H "X-Source-Account-Id: account-1" http://localhost:3501/v1/posts
```

## Development

```bash
# Type check
npm run typecheck

# Watch mode
npm run watch

# Development server
npm run dev
```

## License

MIT
