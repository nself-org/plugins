# CMS Plugin

Headless CMS plugin with content types, posts, categories, tags, versioning, and scheduled publishing for nself applications.

## Overview

The CMS plugin provides a complete headless content management system for building blogs, documentation sites, knowledge bases, and any content-driven application. It supports flexible content types, hierarchical categories, tagging, version control, and scheduled publishing.

### Key Features

- **Flexible Content Types**: Define custom content types with configurable fields
- **Post Management**: Create, update, publish, and schedule content
- **Version Control**: Track content changes with full version history
- **Categories & Tags**: Organize content with hierarchical categories and tags
- **SEO Optimization**: Built-in SEO fields for titles, descriptions, and keywords
- **Featured Content**: Pin and feature important posts
- **Scheduled Publishing**: Schedule posts for future publication
- **Draft System**: Save drafts before publishing
- **Rich Content**: Support for markdown, HTML, and custom formats
- **Custom Fields**: Extend posts with custom JSON fields
- **Multi-Author**: Track content authors and contributors
- **Analytics**: View counts, reading time, word count
- **Search & Filtering**: Query content by status, author, category, tags
- **Multi-App Support**: Isolated CMS instances per source account

### Use Cases

- **Blogs**: Personal or corporate blogs
- **Documentation**: Technical documentation sites
- **Knowledge Bases**: Help centers and FAQs
- **News Sites**: News and magazine publications
- **Marketing**: Landing pages and marketing content
- **E-commerce**: Product descriptions and guides
- **Education**: Course content and learning materials
- **Portfolios**: Creative portfolios and case studies

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install cms

# Initialize database schema
nself cms init

# Start the server
nself cms server
```

### Basic Usage

```bash
# Create a post
curl -X POST http://localhost:3501/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "title": "My First Post",
    "slug": "my-first-post",
    "body": "# Hello World\n\nThis is my first post!",
    "author_id": "user123",
    "status": "published"
  }'

# List posts
curl http://localhost:3501/v1/posts

# Check status
nself cms status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `CMS_PLUGIN_PORT` | No | `3501` | HTTP server port |
| `CMS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `CMS_MAX_BODY_LENGTH` | No | `500000` | Maximum post body length (bytes) |
| `CMS_MAX_TITLE_LENGTH` | No | `500` | Maximum title length (characters) |
| `CMS_SLUG_MAX_LENGTH` | No | `200` | Maximum slug length (characters) |
| `CMS_MAX_VERSIONS` | No | `50` | Maximum versions to keep per post |
| `CMS_SCHEDULED_CHECK_INTERVAL_MS` | No | `60000` | Interval to check for scheduled posts (ms) |
| `CMS_DEFAULT_CONTENT_TYPES` | No | `post,page,recipe` | Default content types (CSV) |
| `CMS_API_KEY` | No | - | API key for authentication |
| `CMS_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `CMS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level |

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
CMS_PLUGIN_PORT=3501
CMS_MAX_BODY_LENGTH=1000000
CMS_MAX_VERSIONS=100
CMS_DEFAULT_CONTENT_TYPES=post,page,article,tutorial
CMS_API_KEY=your-secret-key
```

---

## CLI Commands

### `init`
Initialize the database schema.

```bash
nself cms init
```

### `server`
Start the HTTP API server.

```bash
nself cms server [options]

Options:
  -p, --port <port>    Server port (default: 3501)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

### `status`
Show plugin status and content statistics.

```bash
nself cms status
```

**Output:**
```
CMS Plugin Status
=================
Version:              1.0.0
Port:                 3501
Max Body Length:      500000 bytes
Max Title Length:     500 characters
Max Versions:         50

Content Statistics
==================
Total Posts:          523
Published Posts:      412
Draft Posts:          89
Scheduled Posts:      22
Total Categories:     45
Total Tags:           156
Total Versions:       2341
```

### `posts`
Manage posts (list, create, update, delete).

```bash
nself cms posts [options]

Options:
  -l, --limit <limit>      Number to show (default: 20)
  -s, --status <status>    Filter by status (draft, published, scheduled)
  -a, --author <authorId>  Filter by author
```

**Examples:**
```bash
# List all posts
nself cms posts

# List published posts
nself cms posts --status published

# List by author
nself cms posts --author user123
```

### `categories`
Manage categories.

```bash
nself cms categories
```

### `tags`
Manage tags.

```bash
nself cms tags
```

### `content-types`
Manage content types.

```bash
nself cms content-types
```

### `publish`
Publish a post.

```bash
nself cms publish <postId>
```

### `stats`
Show detailed content statistics.

```bash
nself cms stats
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
  "plugin": "cms",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/status`
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "cms",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "totalPosts": 523,
    "publishedPosts": 412,
    "draftPosts": 89,
    "scheduledPosts": 22,
    "totalCategories": 45,
    "totalTags": 156,
    "totalVersions": 2341,
    "postsByContentType": {
      "post": 389,
      "page": 98,
      "recipe": 36
    },
    "postsByStatus": {
      "published": 412,
      "draft": 89,
      "scheduled": 22
    }
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

### Content Types

#### `POST /v1/content-types`
Create a new content type.

**Request:**
```json
{
  "name": "recipe",
  "display_name": "Recipe",
  "description": "Cooking recipes",
  "icon": "🍳",
  "fields": [
    {
      "name": "prep_time",
      "type": "number",
      "label": "Prep Time (minutes)",
      "required": true
    },
    {
      "name": "servings",
      "type": "number",
      "label": "Servings",
      "required": true
    },
    {
      "name": "ingredients",
      "type": "array",
      "label": "Ingredients",
      "required": true
    }
  ],
  "settings": {
    "enableComments": true,
    "enableRatings": true
  }
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "recipe",
  "display_name": "Recipe",
  "description": "Cooking recipes",
  "icon": "🍳",
  "fields": [...],
  "settings": {...},
  "enabled": true,
  "created_at": "2026-02-11T10:30:00Z",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/content-types`
List all content types.

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "name": "post",
      "display_name": "Blog Post",
      "enabled": true
    }
  ],
  "total": 5
}
```

### Posts

#### `POST /v1/posts`
Create a new post.

**Request:**
```json
{
  "content_type": "post",
  "title": "10 Tips for Better Content",
  "slug": "10-tips-for-better-content",
  "excerpt": "Learn how to create engaging content...",
  "body": "# Introduction\n\nContent is king...",
  "body_format": "markdown",
  "author_id": "user123",
  "status": "draft",
  "visibility": "public",
  "featured_image_url": "https://example.com/image.jpg",
  "featured_image_alt": "Content creation",
  "is_featured": false,
  "custom_fields": {
    "difficulty": "beginner",
    "estimated_time": 15
  },
  "seo_title": "10 Content Tips | My Blog",
  "seo_description": "Discover 10 proven tips...",
  "seo_keywords": ["content", "writing", "tips"],
  "canonical_url": "https://myblog.com/10-tips"
}
```

**Response:**
```json
{
  "id": "post-uuid",
  "content_type": "post",
  "title": "10 Tips for Better Content",
  "slug": "10-tips-for-better-content",
  "excerpt": "Learn how to create engaging content...",
  "body": "# Introduction\n\nContent is king...",
  "author_id": "user123",
  "status": "draft",
  "visibility": "public",
  "is_featured": false,
  "reading_time_minutes": 5,
  "word_count": 1234,
  "view_count": 0,
  "created_at": "2026-02-11T10:30:00Z",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/posts`
List posts with filtering.

**Query Parameters:**
- `status`: Filter by status (draft, published, scheduled)
- `content_type`: Filter by content type
- `author_id`: Filter by author
- `category`: Filter by category slug
- `tag`: Filter by tag slug
- `featured`: Filter featured posts (true/false)
- `limit`: Results per page (default: 50)
- `offset`: Pagination offset (default: 0)
- `search`: Search in title and body

**Response:**
```json
{
  "data": [
    {
      "id": "post-uuid",
      "title": "10 Tips for Better Content",
      "slug": "10-tips-for-better-content",
      "excerpt": "Learn how to create...",
      "author_id": "user123",
      "status": "published",
      "published_at": "2026-02-11T10:00:00Z",
      "reading_time_minutes": 5,
      "view_count": 123,
      "categories": ["writing", "tips"],
      "tags": ["content", "blogging"]
    }
  ],
  "total": 412,
  "limit": 50,
  "offset": 0,
  "hasMore": true
}
```

#### `GET /v1/posts/:id`
Get a single post with full details.

**Response:**
```json
{
  "id": "post-uuid",
  "content_type": "post",
  "title": "10 Tips for Better Content",
  "slug": "10-tips-for-better-content",
  "excerpt": "Learn how to create engaging content...",
  "body": "# Introduction\n\nContent is king...",
  "body_format": "markdown",
  "author_id": "user123",
  "status": "published",
  "published_at": "2026-02-11T10:00:00Z",
  "reading_time_minutes": 5,
  "word_count": 1234,
  "view_count": 123,
  "categories": [
    {
      "id": "cat-uuid",
      "name": "Writing Tips",
      "slug": "writing-tips"
    }
  ],
  "tags": [
    {
      "id": "tag-uuid",
      "name": "Content",
      "slug": "content"
    }
  ],
  "versions": [
    {
      "version": 1,
      "changed_by": "user123",
      "created_at": "2026-02-11T10:30:00Z"
    }
  ],
  "seo_title": "10 Content Tips | My Blog",
  "seo_description": "Discover 10 proven tips...",
  "created_at": "2026-02-11T10:30:00Z",
  "updated_at": "2026-02-11T10:32:00Z"
}
```

#### `GET /v1/posts/slug/:slug`
Get post by slug.

**Response:**
```json
{
  "id": "post-uuid",
  "title": "10 Tips for Better Content",
  "slug": "10-tips-for-better-content",
  ...
}
```

#### `PUT /v1/posts/:id`
Update a post (creates new version).

**Request:**
```json
{
  "title": "Updated Title",
  "body": "Updated content...",
  "status": "published"
}
```

**Response:**
```json
{
  "id": "post-uuid",
  "title": "Updated Title",
  "body": "Updated content...",
  "status": "published",
  "updated_at": "2026-02-11T10:35:00Z"
}
```

#### `DELETE /v1/posts/:id`
Soft-delete a post.

**Response:**
```json
{
  "success": true,
  "post": {
    "id": "post-uuid",
    "deleted_at": "2026-02-11T10:35:00Z"
  }
}
```

### Publishing

#### `POST /v1/posts/:id/publish`
Publish a post immediately.

**Response:**
```json
{
  "success": true,
  "post": {
    "id": "post-uuid",
    "status": "published",
    "published_at": "2026-02-11T10:35:00Z"
  }
}
```

#### `POST /v1/posts/:id/unpublish`
Unpublish a post (set to draft).

**Response:**
```json
{
  "success": true,
  "post": {
    "id": "post-uuid",
    "status": "draft",
    "published_at": null
  }
}
```

#### `POST /v1/posts/:id/schedule`
Schedule a post for future publication.

**Request:**
```json
{
  "scheduled_at": "2026-02-15T09:00:00Z"
}
```

**Response:**
```json
{
  "success": true,
  "post": {
    "id": "post-uuid",
    "status": "scheduled",
    "scheduled_at": "2026-02-15T09:00:00Z"
  }
}
```

### Categories

#### `POST /v1/categories`
Create a new category.

**Request:**
```json
{
  "name": "Writing Tips",
  "slug": "writing-tips",
  "description": "Tips for better writing",
  "parent_id": "parent-category-uuid",
  "sort_order": 0
}
```

**Response:**
```json
{
  "id": "category-uuid",
  "name": "Writing Tips",
  "slug": "writing-tips",
  "description": "Tips for better writing",
  "parent_id": "parent-uuid",
  "sort_order": 0,
  "post_count": 0,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/categories`
List all categories.

**Query Parameters:**
- `parent_id`: Filter by parent (use 'null' for top-level)

**Response:**
```json
{
  "data": [
    {
      "id": "category-uuid",
      "name": "Writing Tips",
      "slug": "writing-tips",
      "post_count": 23,
      "children": [
        {
          "id": "sub-cat-uuid",
          "name": "Grammar",
          "slug": "grammar",
          "post_count": 12
        }
      ]
    }
  ],
  "total": 45
}
```

#### `GET /v1/categories/:id/posts`
Get posts in a category.

**Response:**
```json
{
  "category": {
    "id": "category-uuid",
    "name": "Writing Tips",
    "slug": "writing-tips"
  },
  "posts": [
    {
      "id": "post-uuid",
      "title": "Grammar Basics",
      "slug": "grammar-basics",
      "published_at": "2026-02-11T10:00:00Z"
    }
  ],
  "total": 23
}
```

### Tags

#### `POST /v1/tags`
Create a new tag.

**Request:**
```json
{
  "name": "Content Marketing",
  "slug": "content-marketing"
}
```

**Response:**
```json
{
  "id": "tag-uuid",
  "name": "Content Marketing",
  "slug": "content-marketing",
  "post_count": 0,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/tags`
List all tags.

**Response:**
```json
{
  "data": [
    {
      "id": "tag-uuid",
      "name": "Content Marketing",
      "slug": "content-marketing",
      "post_count": 34
    }
  ],
  "total": 156
}
```

### Post Relations

#### `POST /v1/posts/:id/categories`
Assign categories to a post.

**Request:**
```json
{
  "category_ids": ["cat-uuid-1", "cat-uuid-2"]
}
```

**Response:**
```json
{
  "success": true,
  "categories": [...]
}
```

#### `POST /v1/posts/:id/tags`
Assign tags to a post.

**Request:**
```json
{
  "tag_ids": ["tag-uuid-1", "tag-uuid-2"]
}
```

**Response:**
```json
{
  "success": true,
  "tags": [...]
}
```

### Versions

#### `GET /v1/posts/:id/versions`
List all versions of a post.

**Response:**
```json
{
  "data": [
    {
      "id": "version-uuid",
      "version": 3,
      "title": "Previous Title",
      "change_summary": "Updated introduction",
      "changed_by": "user123",
      "created_at": "2026-02-11T09:00:00Z"
    }
  ],
  "total": 3
}
```

#### `GET /v1/posts/:id/versions/:version`
Get a specific version.

**Response:**
```json
{
  "id": "version-uuid",
  "post_id": "post-uuid",
  "version": 2,
  "title": "Old Title",
  "body": "Old content...",
  "custom_fields": {...},
  "changed_by": "user123",
  "created_at": "2026-02-11T08:00:00Z"
}
```

#### `POST /v1/posts/:id/versions/:version/restore`
Restore a previous version.

**Response:**
```json
{
  "success": true,
  "post": {...},
  "new_version": 4
}
```

---

## Webhook Events

### `post.created`
Triggered when a new post is created.

### `post.updated`
Triggered when a post is updated.

### `post.deleted`
Triggered when a post is soft-deleted.

### `post.published`
Triggered when a post is published.

### `post.unpublished`
Triggered when a post is unpublished.

### `category.created`
Triggered when a category is created.

### `category.updated`
Triggered when a category is updated.

### `category.deleted`
Triggered when a category is removed.

### `tag.created`
Triggered when a tag is created.

### `tag.updated`
Triggered when a tag is updated.

### `tag.deleted`
Triggered when a tag is removed.

---

## Database Schema

### `cms_content_types`
```sql
CREATE TABLE cms_content_types (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(64) NOT NULL,
  display_name VARCHAR(255),
  description TEXT,
  icon VARCHAR(32),
  fields JSONB DEFAULT '[]',
  settings JSONB DEFAULT '{}',
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);
```

### `cms_posts`
```sql
CREATE TABLE cms_posts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  content_type VARCHAR(64) NOT NULL DEFAULT 'post',
  title VARCHAR(500) NOT NULL,
  slug VARCHAR(500) NOT NULL,
  excerpt TEXT,
  body TEXT,
  body_format VARCHAR(16) DEFAULT 'markdown',
  author_id VARCHAR(255) NOT NULL,
  status VARCHAR(16) DEFAULT 'draft',
  visibility VARCHAR(16) DEFAULT 'public',
  featured_image_url TEXT,
  featured_image_alt TEXT,
  cover_image_url TEXT,
  is_featured BOOLEAN DEFAULT false,
  is_pinned BOOLEAN DEFAULT false,
  pinned_at TIMESTAMP WITH TIME ZONE,
  published_at TIMESTAMP WITH TIME ZONE,
  scheduled_at TIMESTAMP WITH TIME ZONE,
  reading_time_minutes INTEGER,
  word_count INTEGER,
  view_count INTEGER DEFAULT 0,
  comment_count INTEGER DEFAULT 0,
  custom_fields JSONB DEFAULT '{}',
  seo_title VARCHAR(255),
  seo_description TEXT,
  seo_keywords TEXT[],
  canonical_url TEXT,
  metadata JSONB DEFAULT '{}',
  deleted_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, slug)
);
```

### `cms_post_versions`
```sql
CREATE TABLE cms_post_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  post_id UUID NOT NULL REFERENCES cms_posts(id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  title VARCHAR(500),
  body TEXT,
  body_format VARCHAR(16),
  custom_fields JSONB,
  change_summary TEXT,
  changed_by VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### `cms_categories`
```sql
CREATE TABLE cms_categories (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  slug VARCHAR(255) NOT NULL,
  description TEXT,
  parent_id UUID REFERENCES cms_categories(id) ON DELETE SET NULL,
  sort_order INTEGER DEFAULT 0,
  post_count INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, slug)
);
```

### `cms_tags`
```sql
CREATE TABLE cms_tags (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(128) NOT NULL,
  slug VARCHAR(128) NOT NULL,
  post_count INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, slug)
);
```

### `cms_post_categories`
```sql
CREATE TABLE cms_post_categories (
  post_id UUID NOT NULL REFERENCES cms_posts(id) ON DELETE CASCADE,
  category_id UUID NOT NULL REFERENCES cms_categories(id) ON DELETE CASCADE,
  PRIMARY KEY(post_id, category_id)
);
```

### `cms_post_tags`
```sql
CREATE TABLE cms_post_tags (
  post_id UUID NOT NULL REFERENCES cms_posts(id) ON DELETE CASCADE,
  tag_id UUID NOT NULL REFERENCES cms_tags(id) ON DELETE CASCADE,
  PRIMARY KEY(post_id, tag_id)
);
```

### `cms_webhook_events`
```sql
CREATE TABLE cms_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Examples

### Example 1: Blog Post Workflow

```bash
# Create draft post
curl -X POST http://localhost:3501/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Getting Started with nself",
    "slug": "getting-started-nself",
    "body": "# Introduction\n\nnself is a powerful platform...",
    "author_id": "user123",
    "status": "draft"
  }'

# Add categories
curl -X POST http://localhost:3501/v1/posts/post-id/categories \
  -H "Content-Type: application/json" \
  -d '{"category_ids": ["tutorials-cat-id"]}'

# Add tags
curl -X POST http://localhost:3501/v1/posts/post-id/tags \
  -H "Content-Type: application/json" \
  -d '{"tag_ids": ["beginner-tag-id", "tutorial-tag-id"]}'

# Publish
curl -X POST http://localhost:3501/v1/posts/post-id/publish
```

### Example 2: Scheduled Publishing

```bash
# Schedule post for next week
curl -X POST http://localhost:3501/v1/posts/post-id/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "scheduled_at": "2026-02-18T09:00:00Z"
  }'

# The CMS will automatically publish at the scheduled time
```

### Example 3: Custom Content Type (Recipe)

```bash
# Create recipe content type
curl -X POST http://localhost:3501/v1/content-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "recipe",
    "display_name": "Recipe",
    "fields": [
      {"name": "prep_time", "type": "number", "required": true},
      {"name": "cook_time", "type": "number", "required": true},
      {"name": "servings", "type": "number", "required": true},
      {"name": "ingredients", "type": "array", "required": true},
      {"name": "instructions", "type": "array", "required": true}
    ]
  }'

# Create recipe post
curl -X POST http://localhost:3501/v1/posts \
  -H "Content-Type: application/json" \
  -d '{
    "content_type": "recipe",
    "title": "Chocolate Chip Cookies",
    "slug": "chocolate-chip-cookies",
    "body": "Delicious homemade cookies...",
    "author_id": "user123",
    "custom_fields": {
      "prep_time": 15,
      "cook_time": 12,
      "servings": 24,
      "ingredients": ["flour", "sugar", "chocolate chips"],
      "instructions": ["Mix dry ingredients", "Bake at 350F"]
    },
    "status": "published"
  }'
```

### Example 4: Version Control

```bash
# Make changes to post (creates version 2)
curl -X PUT http://localhost:3501/v1/posts/post-id \
  -H "Content-Type: application/json" \
  -d '{
    "body": "Updated content with new information..."
  }'

# List versions
curl http://localhost:3501/v1/posts/post-id/versions

# Restore previous version
curl -X POST http://localhost:3501/v1/posts/post-id/versions/1/restore
```

### Example 5: Featured and Pinned Content

```bash
# Feature a post (show on homepage)
curl -X PUT http://localhost:3501/v1/posts/post-id \
  -H "Content-Type: application/json" \
  -d '{"is_featured": true}'

# Pin a post (keep at top)
curl -X PUT http://localhost:3501/v1/posts/post-id \
  -H "Content-Type: application/json" \
  -d '{"is_pinned": true}'

# Get featured posts
curl "http://localhost:3501/v1/posts?featured=true"
```

---

## Troubleshooting

### Slug Conflicts

**Problem:** Duplicate slug errors when creating posts.

**Solution:**
- Ensure slugs are unique per source account
- Use automatic slug generation from titles
- Add timestamps to slugs: `my-post-2026-02-11`
- Check existing slugs before creating

### Scheduled Posts Not Publishing

**Problem:** Posts remain in scheduled status past scheduled time.

**Solution:**
- Check `CMS_SCHEDULED_CHECK_INTERVAL_MS` setting
- Ensure server is running continuously
- Verify scheduled_at timestamp is correct
- Check server logs for errors
- Manually trigger: `POST /v1/posts/:id/publish`

### Version Limit Reached

**Problem:** Cannot create more versions.

**Solution:**
- Increase `CMS_MAX_VERSIONS` limit
- Clean up old versions manually
- Archive posts that don't need version history

### Slow Post Queries

**Solution:**
- Ensure database indexes exist: `nself cms init`
- Use pagination with reasonable limits
- Filter by status, category, or date range
- Consider full-text search indexing
- Add database query caching

### Body Length Exceeded

**Solution:**
```bash
# Increase limit
export CMS_MAX_BODY_LENGTH=1000000

# Or split into multiple posts
```

---

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/cms
