# Knowledge Base

Knowledge base with documentation, FAQ, semantic search, versioning, translations, and analytics.

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The Knowledge Base plugin provides a comprehensive documentation and knowledge management system for nself applications. It supports rich content creation, versioning, translations, semantic search, categories, comments, review workflows, and detailed analytics.

This plugin is essential for applications requiring structured documentation, FAQs, help centers, wikis, or internal knowledge repositories.

### Key Features

- **Rich Content**: Markdown support with attachments and media embedding
- **Versioning**: Track document versions with rollback capability
- **Translations**: Multi-language support with translation management
- **Semantic Search**: Advanced search with relevance ranking
- **Categories & Collections**: Organize documents hierarchically
- **FAQ Management**: Dedicated FAQ system with Q&A format
- **Review Workflow**: Document review and approval process
- **Comments**: Reader comments and discussion threads
- **Analytics**: View tracking, helpful ratings, search analytics
- **Access Control**: Public/private documents with permission management
- **SEO Optimization**: Meta tags and structured data
- **Multi-Account Isolation**: Full support for multi-tenant applications

### Supported Features

- **Content Types**: documents, FAQs, how-tos, guides, API reference
- **Formats**: Markdown, HTML
- **Search**: Full-text search, semantic search (optional)
- **Attachments**: Images, PDFs, videos
- **Export**: PDF, HTML, Markdown
- **Integrations**: External search engines, analytics platforms

### Use Cases

1. **Help Center**: Customer-facing documentation and FAQs
2. **Internal Wiki**: Employee knowledge base and procedures
3. **API Documentation**: Developer documentation and guides
4. **Training Materials**: Educational content and tutorials
5. **Product Documentation**: User manuals and specifications

## Quick Start

```bash
# Install the plugin
nself plugin install knowledge-base

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export KB_PLUGIN_PORT=3713

# Initialize database schema
nself plugin knowledge-base init

# Start the knowledge base plugin server
nself plugin knowledge-base server

# Check status
nself plugin knowledge-base status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `KB_PLUGIN_PORT` | No | `3713` | HTTP server port |
| `KB_SEMANTIC_SEARCH_ENABLED` | No | `false` | Enable semantic search |
| `KB_DEFAULT_LANGUAGE` | No | `en` | Default language code |
| `KB_MAX_DOCUMENT_SIZE` | No | `10485760` | Max document size (10MB) |
| `KB_CACHE_ENABLED` | No | `true` | Enable response caching |
| `KB_CACHE_TTL` | No | `3600` | Cache TTL in seconds |
| `KB_API_KEY` | No | - | API key for authenticated requests |
| `KB_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `KB_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server Configuration
KB_PLUGIN_PORT=3713

# Search Configuration
KB_SEMANTIC_SEARCH_ENABLED=true
KB_DEFAULT_LANGUAGE=en

# Content Configuration
KB_MAX_DOCUMENT_SIZE=10485760

# Cache Configuration
KB_CACHE_ENABLED=true
KB_CACHE_TTL=3600

# Security
KB_API_KEY=your-secret-api-key-here
KB_RATE_LIMIT_MAX=100
KB_RATE_LIMIT_WINDOW_MS=60000
```

## CLI Commands

### Global Commands

#### `init`
Initialize the knowledge base plugin database schema.

```bash
nself plugin knowledge-base init
```

#### `server`
Start the knowledge base plugin HTTP server.

```bash
nself plugin knowledge-base server
```

#### `status`
Display current knowledge base plugin status.

```bash
nself plugin knowledge-base status
```

### Document Management

#### `documents`
Manage knowledge base documents.

```bash
nself plugin knowledge-base documents list
nself plugin knowledge-base documents create "Getting Started" --content "# Getting Started..."
nself plugin knowledge-base documents info DOC_ID
nself plugin knowledge-base documents publish DOC_ID
nself plugin knowledge-base documents archive DOC_ID
```

### Collection Management

#### `collections`
Manage document collections.

```bash
nself plugin knowledge-base collections list
nself plugin knowledge-base collections create "User Guides" --description "End-user documentation"
nself plugin knowledge-base collections add-doc COLLECTION_ID DOC_ID
```

### FAQ Management

#### `faqs`
Manage FAQs.

```bash
nself plugin knowledge-base faqs list
nself plugin knowledge-base faqs create "How do I reset my password?" \
  --answer "Click 'Forgot Password' on the login page..."
nself plugin knowledge-base faqs publish FAQ_ID
```

### Search

#### `search`
Search knowledge base.

```bash
nself plugin knowledge-base search "reset password"
nself plugin knowledge-base search "user authentication" --collection "User Guides"
```

### Analytics

#### `analytics`
View knowledge base analytics.

```bash
nself plugin knowledge-base analytics
nself plugin knowledge-base analytics documents TOP_DOC_ID
nself plugin knowledge-base analytics search --top-queries
```

### Review Management

#### `reviews`
Manage review requests.

```bash
nself plugin knowledge-base reviews list
nself plugin knowledge-base reviews request DOC_ID REVIEWER_ID
nself plugin knowledge-base reviews approve REVIEW_ID
```

## REST API

### Document Management

#### `POST /api/kb/documents`
Create a document.

**Request:**
```json
{
  "title": "Getting Started Guide",
  "slug": "getting-started",
  "content": "# Getting Started\n\nWelcome to our platform...",
  "summary": "Quick introduction to the platform",
  "collectionId": "550e8400-e29b-41d4-a716-446655440000",
  "authorId": "550e8400-e29b-41d4-a716-446655440001",
  "tags": ["tutorial", "beginner"],
  "language": "en",
  "isPublic": true,
  "metaTitle": "Getting Started - Documentation",
  "metaDescription": "Learn how to get started with our platform"
}
```

**Response:**
```json
{
  "success": true,
  "document": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "title": "Getting Started Guide",
    "slug": "getting-started",
    "status": "draft",
    "version": 1
  }
}
```

#### `GET /api/kb/documents/:documentId`
Get document details.

#### `PATCH /api/kb/documents/:documentId`
Update document.

#### `POST /api/kb/documents/:documentId/publish`
Publish document.

#### `POST /api/kb/documents/:documentId/archive`
Archive document.

#### `GET /api/kb/documents`
List documents.

**Query Parameters:**
- `collectionId` - Filter by collection
- `language` - Filter by language
- `tags` - Comma-separated tags
- `status` - Filter by status (draft, published, archived)
- `search` - Search query
- `limit` - Result limit
- `offset` - Result offset

### Collection Management

#### `POST /api/kb/collections`
Create collection.

**Request:**
```json
{
  "name": "User Guides",
  "description": "End-user documentation",
  "slug": "user-guides",
  "parentId": null,
  "icon": "book",
  "isPublic": true
}
```

#### `GET /api/kb/collections`
List collections.

#### `GET /api/kb/collections/:collectionId/documents`
List documents in collection.

### FAQ Management

#### `POST /api/kb/faqs`
Create FAQ.

**Request:**
```json
{
  "question": "How do I reset my password?",
  "answer": "Click 'Forgot Password' on the login page...",
  "collectionId": "550e8400-e29b-41d4-a716-446655440000",
  "tags": ["account", "password"],
  "order": 1
}
```

#### `GET /api/kb/faqs`
List FAQs.

#### `PATCH /api/kb/faqs/:faqId`
Update FAQ.

### Search

#### `GET /api/kb/search`
Search knowledge base.

**Query Parameters:**
- `q` - Search query (required)
- `collectionId` - Filter by collection
- `language` - Filter by language
- `tags` - Comma-separated tags
- `limit` - Result limit (default: 20)

**Response:**
```json
{
  "success": true,
  "results": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "title": "Getting Started Guide",
      "summary": "Quick introduction...",
      "relevance": 0.95,
      "highlights": ["...get <mark>started</mark>..."]
    }
  ],
  "total": 15,
  "query": "getting started"
}
```

### Attachments

#### `POST /api/kb/documents/:documentId/attachments`
Upload attachment.

#### `GET /api/kb/documents/:documentId/attachments`
List attachments.

#### `DELETE /api/kb/attachments/:attachmentId`
Delete attachment.

### Comments

#### `POST /api/kb/documents/:documentId/comments`
Add comment.

**Request:**
```json
{
  "userId": "550e8400-e29b-41d4-a716-446655440003",
  "content": "Very helpful guide!",
  "parentCommentId": null
}
```

#### `GET /api/kb/documents/:documentId/comments`
List comments.

#### `DELETE /api/kb/comments/:commentId`
Delete comment.

### Analytics

#### `POST /api/kb/documents/:documentId/view`
Track document view.

#### `POST /api/kb/documents/:documentId/helpful`
Mark document as helpful.

**Request:**
```json
{
  "helpful": true
}
```

#### `GET /api/kb/analytics/documents`
Get document analytics.

**Response:**
```json
{
  "success": true,
  "analytics": {
    "topDocuments": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440002",
        "title": "Getting Started Guide",
        "views": 12345,
        "helpfulVotes": 987,
        "avgRating": 4.8
      }
    ],
    "totalViews": 45678,
    "totalDocuments": 234
  }
}
```

#### `GET /api/kb/analytics/search`
Get search analytics.

**Response:**
```json
{
  "success": true,
  "analytics": {
    "topQueries": [
      {"query": "reset password", "count": 456},
      {"query": "getting started", "count": 234}
    ],
    "totalSearches": 12345,
    "avgResultsPerSearch": 8.5
  }
}
```

### Translations

#### `POST /api/kb/documents/:documentId/translations`
Add translation.

**Request:**
```json
{
  "language": "es",
  "title": "Guía de Inicio",
  "content": "# Guía de Inicio\n\nBienvenido...",
  "translatedBy": "550e8400-e29b-41d4-a716-446655440004"
}
```

#### `GET /api/kb/documents/:documentId/translations`
List translations.

### Review Management

#### `POST /api/kb/reviews`
Request document review.

**Request:**
```json
{
  "documentId": "550e8400-e29b-41d4-a716-446655440002",
  "reviewerId": "550e8400-e29b-41d4-a716-446655440005",
  "notes": "Please review for technical accuracy",
  "dueDate": "2024-02-15T00:00:00Z"
}
```

#### `POST /api/kb/reviews/:reviewId/complete`
Complete review.

**Request:**
```json
{
  "approved": true,
  "feedback": "Looks good, minor typo fixed",
  "changes": ["Fixed typo in step 3"]
}
```

#### `GET /api/kb/reviews`
List review requests.

### Webhook Endpoint

#### `POST /webhook`
Receive webhook events.

## Webhook Events

### Document Events

#### `document.created`
New document created.

**Payload:**
```json
{
  "type": "document.created",
  "document": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "title": "Getting Started Guide",
    "slug": "getting-started",
    "authorId": "550e8400-e29b-41d4-a716-446655440001"
  },
  "timestamp": "2024-02-10T10:00:00Z"
}
```

#### `document.updated`
Document updated.

#### `document.published`
Document published.

#### `document.archived`
Document archived.

### FAQ Events

#### `faq.created`
FAQ created.

#### `faq.updated`
FAQ updated.

### Comment Events

#### `comment.created`
Comment added.

### Search Events

#### `search.performed`
Search performed (for analytics).

### Review Events

#### `review.requested`
Review requested.

#### `review.completed`
Review completed.

## Database Schema

### kb_documents

Knowledge base documents.

```sql
CREATE TABLE kb_documents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  collection_id UUID REFERENCES kb_collections(id) ON DELETE SET NULL,
  title VARCHAR(500) NOT NULL,
  slug VARCHAR(500) NOT NULL,
  content TEXT NOT NULL,
  summary TEXT,
  author_id UUID NOT NULL,
  status VARCHAR(50) NOT NULL DEFAULT 'draft',
  language VARCHAR(10) NOT NULL DEFAULT 'en',
  tags TEXT[] DEFAULT '{}',
  is_public BOOLEAN NOT NULL DEFAULT true,
  meta_title VARCHAR(200),
  meta_description VARCHAR(500),
  version INTEGER NOT NULL DEFAULT 1,
  previous_version_id UUID,
  view_count INTEGER DEFAULT 0,
  helpful_count INTEGER DEFAULT 0,
  not_helpful_count INTEGER DEFAULT 0,
  comment_count INTEGER DEFAULT 0,
  last_viewed_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  published_at TIMESTAMPTZ,
  archived_at TIMESTAMPTZ,
  UNIQUE(source_account_id, slug, language)
);

CREATE INDEX idx_kb_documents_account ON kb_documents(source_account_id);
CREATE INDEX idx_kb_documents_collection ON kb_documents(collection_id);
CREATE INDEX idx_kb_documents_slug ON kb_documents(slug);
CREATE INDEX idx_kb_documents_author ON kb_documents(author_id);
CREATE INDEX idx_kb_documents_status ON kb_documents(status);
CREATE INDEX idx_kb_documents_language ON kb_documents(language);
CREATE INDEX idx_kb_documents_tags ON kb_documents USING GIN(tags);
CREATE INDEX idx_kb_documents_public ON kb_documents(is_public) WHERE is_public = true;
CREATE INDEX idx_kb_documents_search ON kb_documents USING GIN(to_tsvector('english', title || ' ' || content));
```

### kb_collections

Document collections/categories.

```sql
CREATE TABLE kb_collections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  parent_id UUID REFERENCES kb_collections(id) ON DELETE CASCADE,
  name VARCHAR(200) NOT NULL,
  slug VARCHAR(200) NOT NULL,
  description TEXT,
  icon VARCHAR(50),
  color VARCHAR(20),
  order_index INTEGER DEFAULT 0,
  is_public BOOLEAN NOT NULL DEFAULT true,
  document_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, slug)
);

CREATE INDEX idx_kb_collections_account ON kb_collections(source_account_id);
CREATE INDEX idx_kb_collections_parent ON kb_collections(parent_id);
CREATE INDEX idx_kb_collections_slug ON kb_collections(slug);
CREATE INDEX idx_kb_collections_order ON kb_collections(order_index);
```

### kb_faqs

Frequently asked questions.

```sql
CREATE TABLE kb_faqs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  collection_id UUID REFERENCES kb_collections(id) ON DELETE SET NULL,
  question TEXT NOT NULL,
  answer TEXT NOT NULL,
  tags TEXT[] DEFAULT '{}',
  order_index INTEGER DEFAULT 0,
  is_published BOOLEAN NOT NULL DEFAULT false,
  view_count INTEGER DEFAULT 0,
  helpful_count INTEGER DEFAULT 0,
  not_helpful_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_by UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  published_at TIMESTAMPTZ
);

CREATE INDEX idx_kb_faqs_account ON kb_faqs(source_account_id);
CREATE INDEX idx_kb_faqs_collection ON kb_faqs(collection_id);
CREATE INDEX idx_kb_faqs_tags ON kb_faqs USING GIN(tags);
CREATE INDEX idx_kb_faqs_published ON kb_faqs(is_published) WHERE is_published = true;
CREATE INDEX idx_kb_faqs_search ON kb_faqs USING GIN(to_tsvector('english', question || ' ' || answer));
```

### kb_attachments

Document attachments.

```sql
CREATE TABLE kb_attachments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  document_id UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
  filename VARCHAR(255) NOT NULL,
  file_url TEXT NOT NULL,
  file_type VARCHAR(100),
  file_size INTEGER,
  uploaded_by UUID NOT NULL,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kb_attachments_account ON kb_attachments(source_account_id);
CREATE INDEX idx_kb_attachments_document ON kb_attachments(document_id);
```

### kb_comments

Document comments.

```sql
CREATE TABLE kb_comments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  document_id UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
  parent_comment_id UUID REFERENCES kb_comments(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  content TEXT NOT NULL,
  is_deleted BOOLEAN NOT NULL DEFAULT false,
  deleted_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kb_comments_account ON kb_comments(source_account_id);
CREATE INDEX idx_kb_comments_document ON kb_comments(document_id);
CREATE INDEX idx_kb_comments_parent ON kb_comments(parent_comment_id);
CREATE INDEX idx_kb_comments_user ON kb_comments(user_id);
CREATE INDEX idx_kb_comments_active ON kb_comments(is_deleted) WHERE is_deleted = false;
```

### kb_analytics

Analytics tracking.

```sql
CREATE TABLE kb_analytics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  document_id UUID REFERENCES kb_documents(id) ON DELETE CASCADE,
  event_type VARCHAR(50) NOT NULL,
  user_id UUID,
  session_id VARCHAR(255),
  ip_address VARCHAR(45),
  user_agent TEXT,
  referer TEXT,
  search_query TEXT,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kb_analytics_account ON kb_analytics(source_account_id);
CREATE INDEX idx_kb_analytics_document ON kb_analytics(document_id);
CREATE INDEX idx_kb_analytics_event ON kb_analytics(event_type);
CREATE INDEX idx_kb_analytics_created ON kb_analytics(created_at DESC);
```

**Event types:** `view`, `search`, `helpful`, `not_helpful`, `comment`

### kb_translations

Document translations.

```sql
CREATE TABLE kb_translations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  document_id UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
  language VARCHAR(10) NOT NULL,
  title VARCHAR(500) NOT NULL,
  content TEXT NOT NULL,
  summary TEXT,
  translated_by UUID NOT NULL,
  status VARCHAR(50) NOT NULL DEFAULT 'draft',
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  published_at TIMESTAMPTZ,
  UNIQUE(source_account_id, document_id, language)
);

CREATE INDEX idx_kb_translations_account ON kb_translations(source_account_id);
CREATE INDEX idx_kb_translations_document ON kb_translations(document_id);
CREATE INDEX idx_kb_translations_language ON kb_translations(language);
```

### kb_review_requests

Document review tracking.

```sql
CREATE TABLE kb_review_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  document_id UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
  requested_by UUID NOT NULL,
  reviewer_id UUID NOT NULL,
  status VARCHAR(50) NOT NULL DEFAULT 'pending',
  notes TEXT,
  feedback TEXT,
  approved BOOLEAN,
  changes_made TEXT[] DEFAULT '{}',
  due_date TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_kb_reviews_account ON kb_review_requests(source_account_id);
CREATE INDEX idx_kb_reviews_document ON kb_review_requests(document_id);
CREATE INDEX idx_kb_reviews_reviewer ON kb_review_requests(reviewer_id);
CREATE INDEX idx_kb_reviews_status ON kb_review_requests(status);
```

## Examples

### Example 1: Create and Publish Document

```bash
# Create document
curl -X POST http://localhost:3713/api/kb/documents \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Installation Guide",
    "slug": "installation",
    "content": "# Installation\n\n## Prerequisites...",
    "authorId": "USER_ID",
    "tags": ["setup", "installation"],
    "isPublic": true
  }'

# Publish document
curl -X POST http://localhost:3713/api/kb/documents/DOC_ID/publish
```

### Example 2: Create Collection Hierarchy

```bash
# Create parent collection
curl -X POST http://localhost:3713/api/kb/collections \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Documentation",
    "slug": "docs",
    "isPublic": true
  }'

# Create child collection
curl -X POST http://localhost:3713/api/kb/collections \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Getting Started",
    "slug": "getting-started",
    "parentId": "PARENT_COLLECTION_ID"
  }'
```

### Example 3: Search Knowledge Base

```bash
# Basic search
curl "http://localhost:3713/api/kb/search?q=installation"

# Advanced search with filters
curl "http://localhost:3713/api/kb/search?q=reset+password&collectionId=COLLECTION_ID&tags=account&limit=10"
```

### Example 4: Track Analytics

```bash
# Track document view
curl -X POST http://localhost:3713/api/kb/documents/DOC_ID/view

# Mark as helpful
curl -X POST http://localhost:3713/api/kb/documents/DOC_ID/helpful \
  -H "Content-Type: application/json" \
  -d '{"helpful": true}'

# View analytics
curl http://localhost:3713/api/kb/analytics/documents
```

### Example 5: Request Review

```bash
# Request document review
curl -X POST http://localhost:3713/api/kb/reviews \
  -H "Content-Type: application/json" \
  -d '{
    "documentId": "DOC_ID",
    "reviewerId": "REVIEWER_ID",
    "notes": "Please verify technical accuracy",
    "dueDate": "2024-02-15T00:00:00Z"
  }'

# Complete review
curl -X POST http://localhost:3713/api/kb/reviews/REVIEW_ID/complete \
  -H "Content-Type: application/json" \
  -d '{
    "approved": true,
    "feedback": "Looks good",
    "changes": ["Fixed typo in step 2"]
  }'
```

## Troubleshooting

### Search Issues

**Problem:** Search not returning expected results

**Solutions:**
1. Verify full-text search indexes are created
2. Check document status is "published"
3. Review language settings
4. Test with simpler queries
5. Check if semantic search is enabled when needed

### Performance Issues

**Problem:** Slow page loads

**Solutions:**
1. Enable caching with KB_CACHE_ENABLED=true
2. Add database indexes for common queries
3. Optimize large documents by splitting into sections
4. Use CDN for media attachments
5. Review analytics queries for performance

### Content Issues

**Problem:** Documents not appearing

**Solutions:**
1. Check document status is "published"
2. Verify isPublic flag for public content
3. Check collection permissions
4. Review access control settings

---

**Version:** 1.0.0
**Last Updated:** February 2024
**Support:** https://github.com/acamarata/nself-plugins/issues
