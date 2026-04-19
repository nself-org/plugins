# Documents

Document management and generation service with templates, versioning, and sharing

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

The Documents plugin provides comprehensive document management capabilities including template-based document generation, versioning, sharing with expiration, and PDF rendering. It supports multiple template engines and storage providers for flexible deployment scenarios.

### Key Features

- **Document Generation** - Generate PDFs from templates with dynamic data injection
- **Template Management** - Create and manage reusable document templates
- **Version Control** - Track document versions with configurable retention
- **Secure Sharing** - Generate time-limited share links with token authentication
- **PDF Rendering** - Multiple PDF engines (Puppeteer, wkhtmltopdf, PDFKit)
- **Template Engines** - Support for Handlebars, EJS, Pug, and Markdown
- **Storage Flexibility** - Local filesystem, S3, or cloud storage backends
- **Multi-Format** - Generate PDF, HTML, Markdown, and plain text documents

## Quick Start

```bash
# Install the plugin
nself plugin install documents

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export DOCS_PLUGIN_PORT=3029
export DOCS_STORAGE_PATH="/data/documents"

# Initialize the database schema
nself plugin documents init

# Start the server
nself plugin documents server
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `""` | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `DOCS_PLUGIN_PORT` | No | `3029` | HTTP server port |
| `DOCS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `DOCS_PDF_ENGINE` | No | `puppeteer` | PDF rendering engine (puppeteer, wkhtmltopdf, pdfkit) |
| `DOCS_DEFAULT_TEMPLATE_ENGINE` | No | `handlebars` | Default template engine (handlebars, ejs, pug, markdown) |
| `DOCS_STORAGE_PROVIDER` | No | `local` | Storage backend (local, s3, gcs) |
| `DOCS_STORAGE_PATH` | No | `/data/documents` | Local storage path |
| `DOCS_MAX_DOCUMENT_SIZE_MB` | No | `50` | Maximum document size in MB |
| `DOCS_SHARE_TOKEN_LENGTH` | No | `32` | Length of share tokens |
| `DOCS_SHARE_DEFAULT_EXPIRY_DAYS` | No | `30` | Default expiration for shares in days |
| `DOCS_VERSION_RETENTION` | No | `10` | Number of versions to retain per document |
| `DOCS_APP_IDS` | No | `primary` | Comma-separated application IDs |

### Example .env

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server Configuration
DOCS_PLUGIN_PORT=3029
DOCS_PLUGIN_HOST=0.0.0.0

# PDF Generation
DOCS_PDF_ENGINE=puppeteer
DOCS_DEFAULT_TEMPLATE_ENGINE=handlebars

# Storage
DOCS_STORAGE_PROVIDER=local
DOCS_STORAGE_PATH=/data/documents
DOCS_MAX_DOCUMENT_SIZE_MB=50

# Sharing
DOCS_SHARE_DEFAULT_EXPIRY_DAYS=30
DOCS_SHARE_TOKEN_LENGTH=32

# Versioning
DOCS_VERSION_RETENTION=10

# Multi-App Support
DOCS_APP_IDS=primary,app1,app2
```

## CLI Commands

### `init`
Initialize the documents database schema.

```bash
nself plugin documents init
```

### `server`
Start the documents HTTP server.

```bash
nself plugin documents server
```

### `list`
List documents.

```bash
# List all documents
nself plugin documents list

# Filter by creator
nself plugin documents list --creator user123

# Pagination
nself plugin documents list --limit 50 --offset 100
```

### `generate`
Generate a document from a template.

```bash
# Generate invoice
nself plugin documents generate \
  --template invoice \
  --data '{"invoiceNumber":"INV-001","amount":1000}' \
  --format pdf

# Generate contract
nself plugin documents generate \
  --template contract \
  --data-file contract-data.json \
  --output contract-final.pdf
```

### `templates`
Manage templates.

```bash
# List templates
nself plugin documents templates list

# Create template
nself plugin documents templates create \
  --name "Invoice Template" \
  --engine handlebars \
  --content-file invoice-template.hbs

# Update template
nself plugin documents templates update \
  --id template-uuid \
  --content-file updated-template.hbs

# Delete template
nself plugin documents templates delete --id template-uuid
```

### `search`
Search documents.

```bash
# Search by title
nself plugin documents search --query "invoice"

# Search with filters
nself plugin documents search \
  --query "contract" \
  --creator user123 \
  --from "2025-01-01" \
  --to "2025-12-31"
```

### `stats`
Show document statistics.

```bash
nself plugin documents stats

# Example output:
# {
#   "totalDocuments": 1520,
#   "totalTemplates": 25,
#   "totalVersions": 3400,
#   "totalShares": 450,
#   "totalStorageMb": 1250
# }
```

## REST API

### Documents

#### `POST /api/documents`
Create a new document.

**Request Body:**
```json
{
  "title": "Contract Agreement",
  "content": "<h1>Contract</h1><p>This is a contract...</p>",
  "format": "html",
  "creatorId": "user123",
  "metadata": {
    "category": "legal",
    "client": "ACME Corp"
  }
}
```

**Response:** `201 Created`

#### `POST /api/documents/generate`
Generate a document from a template.

**Request Body:**
```json
{
  "templateId": "template-uuid",
  "data": {
    "customerName": "John Doe",
    "invoiceNumber": "INV-2025-001",
    "amount": 1500.00,
    "items": [
      {"description": "Service A", "price": 1000},
      {"description": "Service B", "price": 500}
    ]
  },
  "format": "pdf",
  "title": "Invoice INV-2025-001",
  "creatorId": "user123"
}
```

**Response:**
```json
{
  "id": "document-uuid",
  "title": "Invoice INV-2025-001",
  "format": "pdf",
  "url": "/api/documents/document-uuid/download",
  "version": 1
}
```

#### `GET /api/documents`
List documents with filtering.

**Query Parameters:**
- `creatorId` (optional): Filter by creator
- `search` (optional): Search query
- `from` (optional): Start date (ISO 8601)
- `to` (optional): End date (ISO 8601)
- `limit` (optional, default: 50)
- `offset` (optional, default: 0)

#### `GET /api/documents/:id`
Get document details.

**Response:**
```json
{
  "id": "document-uuid",
  "title": "Contract Agreement",
  "format": "pdf",
  "creatorId": "user123",
  "currentVersion": 2,
  "totalVersions": 2,
  "fileSize": 245760,
  "createdAt": "2025-02-11T10:30:00Z",
  "updatedAt": "2025-02-11T12:00:00Z",
  "metadata": {...}
}
```

#### `GET /api/documents/:id/download`
Download document file.

**Response:** Binary file with appropriate content-type header

#### `PUT /api/documents/:id`
Update document (creates new version).

**Request Body:**
```json
{
  "content": "<updated content>",
  "comment": "Updated terms section"
}
```

#### `DELETE /api/documents/:id`
Delete document and all versions.

**Response:** `204 No Content`

### Templates

#### `POST /api/templates`
Create a new template.

**Request Body:**
```json
{
  "name": "Invoice Template",
  "description": "Standard invoice template",
  "engine": "handlebars",
  "content": "<html>{{#each items}}...{{/each}}</html>",
  "sampleData": {
    "customerName": "Example Customer",
    "items": [...]
  }
}
```

#### `GET /api/templates`
List templates.

#### `GET /api/templates/:id`
Get template details.

#### `PUT /api/templates/:id`
Update template.

#### `DELETE /api/templates/:id`
Delete template.

### Sharing

#### `POST /api/documents/:id/share`
Create a share link.

**Request Body:**
```json
{
  "expiresIn": 7,
  "allowDownload": true,
  "password": "optional-password"
}
```

**Response:**
```json
{
  "shareToken": "abc123def456...",
  "shareUrl": "https://example.com/share/abc123def456",
  "expiresAt": "2025-02-18T10:30:00Z"
}
```

#### `GET /api/share/:token`
Access shared document.

**Response:** Document content or redirect to download

### Versions

#### `GET /api/documents/:id/versions`
List document versions.

**Response:**
```json
{
  "data": [
    {
      "version": 2,
      "createdBy": "user123",
      "comment": "Updated terms",
      "fileSize": 245760,
      "createdAt": "2025-02-11T12:00:00Z"
    },
    {
      "version": 1,
      "createdBy": "user123",
      "comment": "Initial version",
      "fileSize": 238592,
      "createdAt": "2025-02-11T10:30:00Z"
    }
  ]
}
```

#### `GET /api/documents/:id/versions/:version/download`
Download specific version.

## Webhook Events

| Event Type | Description | Payload |
|------------|-------------|---------|
| `docs.document.created` | New document created | `{ documentId, title, creatorId }` |
| `docs.document.updated` | Document updated | `{ documentId, version, updatedBy }` |
| `docs.document.deleted` | Document deleted | `{ documentId, title }` |
| `docs.document.shared` | Share link created | `{ documentId, shareToken, expiresAt }` |
| `docs.template.created` | New template created | `{ templateId, name }` |

## Database Schema

### np_documents_documents

```sql
CREATE TABLE IF NOT EXISTS np_documents_documents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  title VARCHAR(500) NOT NULL,
  format VARCHAR(20) NOT NULL,
  creator_id VARCHAR(255) NOT NULL,
  current_version INTEGER DEFAULT 1,
  np_fileproc_size BIGINT,
  storage_path TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### np_documents_templates

```sql
CREATE TABLE IF NOT EXISTS np_documents_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  engine VARCHAR(50) NOT NULL,
  content TEXT NOT NULL,
  sample_data JSONB,
  created_by VARCHAR(255),
  active BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### np_documents_versions

```sql
CREATE TABLE IF NOT EXISTS np_documents_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  document_id UUID NOT NULL REFERENCES np_documents_documents(id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  content TEXT,
  np_fileproc_size BIGINT,
  storage_path TEXT,
  created_by VARCHAR(255),
  comment TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(document_id, version)
);
```

### np_documents_shares

```sql
CREATE TABLE IF NOT EXISTS np_documents_shares (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  document_id UUID NOT NULL REFERENCES np_documents_documents(id) ON DELETE CASCADE,
  share_token VARCHAR(255) UNIQUE NOT NULL,
  created_by VARCHAR(255),
  expires_at TIMESTAMPTZ,
  password_hash VARCHAR(255),
  allow_download BOOLEAN DEFAULT true,
  access_count INTEGER DEFAULT 0,
  last_accessed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

## Examples

### Example 1: Generate Invoice from Template

```javascript
const response = await fetch('http://localhost:3029/api/documents/generate', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    templateId: 'invoice-template-uuid',
    data: {
      invoiceNumber: 'INV-2025-001',
      date: '2025-02-11',
      customerName: 'ACME Corporation',
      customerEmail: 'billing@acme.com',
      items: [
        { description: 'Consulting Services', hours: 40, rate: 150, amount: 6000 },
        { description: 'Cloud Infrastructure', quantity: 1, rate: 2500, amount: 2500 }
      ],
      subtotal: 8500,
      tax: 850,
      total: 9350
    },
    format: 'pdf',
    title: 'Invoice INV-2025-001',
    creatorId: 'user123'
  })
});

const document = await response.json();
console.log(`Generated document: ${document.url}`);
```

### Example 2: Version Management

```sql
-- Get document with version history
SELECT
  d.id,
  d.title,
  d.current_version,
  COUNT(v.version) as total_versions,
  MAX(v.created_at) as last_updated
FROM np_documents_documents d
LEFT JOIN np_documents_versions v ON v.document_id = d.id
WHERE d.source_account_id = 'primary'
GROUP BY d.id
ORDER BY last_updated DESC;
```

### Example 3: Secure Document Sharing

```bash
# Create time-limited share link
curl -X POST http://localhost:3029/api/documents/doc-uuid/share \
  -H "Content-Type: application/json" \
  -d '{
    "expiresIn": 7,
    "allowDownload": true,
    "password": "secure123"
  }'

# Response includes share URL
# https://example.com/share/abc123def456

# Access shared document (prompts for password)
curl https://example.com/share/abc123def456
```

## Troubleshooting

### Common Issues

#### 1. PDF Generation Fails

**Symptom:** Error when generating PDFs.

**Solutions:**
- Verify PDF engine is installed (Puppeteer requires Chrome/Chromium)
- Check template syntax is valid
- Ensure sufficient memory for PDF rendering
- Try alternative PDF engine: `DOCS_PDF_ENGINE=wkhtmltopdf`

#### 2. Storage Path Not Writable

**Symptom:** Documents fail to save.

**Solutions:**
- Verify storage path exists: `ls -la /data/documents`
- Check write permissions: `chmod -R 755 /data/documents`
- Ensure sufficient disk space: `df -h /data`

#### 3. Template Rendering Errors

**Symptom:** Template fails to render.

**Solutions:**
- Validate template syntax
- Check data structure matches template expectations
- Test template with sample data
- Review template engine documentation

---

**Need more help?** Check the [main documentation](https://github.com/acamarata/nself-plugins) or [open an issue](https://github.com/acamarata/nself-plugins/issues).
