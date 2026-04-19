# Photos

Photo album management with EXIF extraction, tagging, face grouping, and thumbnails

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

The Photos plugin provides a comprehensive photo management system for the nself platform. It enables users to organize photos into albums, extract and store EXIF metadata, create and manage tags (including keywords, people, locations, and events), detect and group faces, generate responsive thumbnails, and perform full-text search across photo metadata.

This plugin is ideal for applications that need to manage user photo libraries, build photo galleries, implement facial recognition features, or provide advanced photo organization and search capabilities.

### Key Features

- **Album Management** - Create, organize, and manage photo albums with visibility controls (public, private, shared)
- **EXIF Extraction** - Automatically extract camera metadata, location data, and timestamps from uploaded photos
- **Thumbnail Generation** - Generate multiple thumbnail sizes (small, medium, large) with configurable quality and format
- **Face Detection** - Detect faces in photos and group similar faces together for identification
- **Advanced Tagging** - Tag photos with keywords, people, locations, and events with support for face region tracking
- **Full-Text Search** - Search photos by caption, filename, location, tags, and date ranges using PostgreSQL full-text search
- **Timeline View** - View photos grouped by time periods (day, week, month, year) with cover images
- **Multi-App Support** - Isolate photo libraries by application ID for multi-tenant architectures
- **Webhook Events** - Emit events for photo uploads, processing completion, tag additions, and face identification
- **Batch Processing** - Upload and process multiple photos in a single API call
- **Processing Queue** - Asynchronous photo processing with status tracking (pending, processing, completed, error)

## Quick Start

```bash
# Install the plugin
nself plugin install photos

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export PHOTOS_PLUGIN_PORT=3023

# Initialize the database schema
nself plugin photos init

# Start the server
nself plugin photos server
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
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL connection |
| `PHOTOS_PLUGIN_PORT` | No | `3023` | HTTP server port |
| `PHOTOS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `PHOTOS_LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `PHOTOS_APP_IDS` | No | `primary` | Comma-separated list of application IDs for multi-app support |
| `PHOTOS_THUMBNAIL_SMALL` | No | `150` | Small thumbnail width in pixels |
| `PHOTOS_THUMBNAIL_MEDIUM` | No | `600` | Medium thumbnail width in pixels |
| `PHOTOS_THUMBNAIL_LARGE` | No | `1200` | Large thumbnail width in pixels |
| `PHOTOS_THUMBNAIL_QUALITY` | No | `85` | JPEG/WebP quality (0-100) |
| `PHOTOS_THUMBNAIL_FORMAT` | No | `webp` | Thumbnail format (webp, jpeg, png) |
| `PHOTOS_EXIF_EXTRACTION` | No | `true` | Enable EXIF metadata extraction |
| `PHOTOS_FACE_DETECTION` | No | `false` | Enable face detection and grouping |
| `PHOTOS_FACE_DETECTION_PROVIDER` | No | `""` | Face detection provider (opencv, aws-rekognition, google-vision) |
| `PHOTOS_FACE_DETECTION_API_KEY` | No | `""` | API key for external face detection provider |
| `PHOTOS_PROCESSING_CONCURRENCY` | No | `4` | Number of photos to process concurrently |
| `PHOTOS_SEARCH_ENABLED` | No | `true` | Enable full-text search functionality |
| `PHOTOS_MAX_UPLOAD_BATCH` | No | `100` | Maximum number of photos per batch upload |
| `PHOTOS_API_KEY` | No | `""` | API key for authenticating HTTP requests (optional) |
| `PHOTOS_RATE_LIMIT_MAX` | No | `100` | Maximum requests per rate limit window |
| `PHOTOS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds (default: 1 minute) |

### Example .env

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server Configuration
PHOTOS_PLUGIN_PORT=3023
PHOTOS_PLUGIN_HOST=0.0.0.0
PHOTOS_LOG_LEVEL=info

# Multi-App Support
PHOTOS_APP_IDS=primary,app1,app2

# Thumbnail Configuration
PHOTOS_THUMBNAIL_SMALL=150
PHOTOS_THUMBNAIL_MEDIUM=600
PHOTOS_THUMBNAIL_LARGE=1200
PHOTOS_THUMBNAIL_QUALITY=85
PHOTOS_THUMBNAIL_FORMAT=webp

# Feature Flags
PHOTOS_EXIF_EXTRACTION=true
PHOTOS_FACE_DETECTION=true
PHOTOS_FACE_DETECTION_PROVIDER=aws-rekognition
PHOTOS_FACE_DETECTION_API_KEY=your-api-key-here
PHOTOS_PROCESSING_CONCURRENCY=4
PHOTOS_SEARCH_ENABLED=true

# API Configuration
PHOTOS_MAX_UPLOAD_BATCH=100
PHOTOS_API_KEY=your-secret-api-key
PHOTOS_RATE_LIMIT_MAX=100
PHOTOS_RATE_LIMIT_WINDOW_MS=60000
```

## CLI Commands

### `init`

Initialize the photos database schema.

```bash
nself plugin photos init
```

### `server`

Start the photos HTTP server.

```bash
nself plugin photos server
```

### `albums list`

List photo albums with optional filtering.

```bash
# List all albums
nself plugin photos albums list

# Filter by owner
nself plugin photos albums list --owner user123

# Filter by visibility
nself plugin photos albums list --visibility public

# Pagination
nself plugin photos albums list --limit 20 --offset 40

# Multi-app support
nself plugin photos albums list --app-id app1
```

### `albums create`

Create a new photo album.

```bash
# Create a basic album
nself plugin photos albums create --name "Vacation 2025"

# Create with description
nself plugin photos albums create \
  --name "Family Photos" \
  --description "Our family photo collection" \
  --visibility private

# Create with custom sort order
nself plugin photos albums create \
  --name "Portfolio" \
  --visibility public \
  --sort-order date_asc \
  --owner user123
```

### `albums get`

Get album details by ID.

```bash
nself plugin photos albums get --id a1b2c3d4-e5f6-7890-1234-567890abcdef
```

### `albums delete`

Delete an album.

```bash
nself plugin photos albums delete --id a1b2c3d4-e5f6-7890-1234-567890abcdef
```

### `list`

List photos with optional filtering.

```bash
# List all photos
nself plugin photos list

# Filter by album
nself plugin photos list --album a1b2c3d4-e5f6-7890-1234-567890abcdef

# Filter by uploader
nself plugin photos list --uploader user123

# Pagination
nself plugin photos list --limit 50 --offset 100
```

### `process-pending`

Process pending photos (extract EXIF, generate thumbnails, detect faces).

```bash
# Process all pending photos
nself plugin photos process-pending

# Process with custom limit
nself plugin photos process-pending --limit 10
```

### `tags list`

List tags with counts.

```bash
# List all tags
nself plugin photos tags list

# Filter by tag type
nself plugin photos tags list --type keyword

# Limit results
nself plugin photos tags list --top 20
```

### `tags add`

Add a tag to a photo.

```bash
# Add keyword tag
nself plugin photos tags add \
  --photo photo-id \
  --type keyword \
  --value "sunset"

# Add person tag
nself plugin photos tags add \
  --photo photo-id \
  --type person \
  --value "John Doe" \
  --user-id user123

# Add location tag
nself plugin photos tags add \
  --photo photo-id \
  --type location \
  --value "Paris, France"
```

### `tags photos`

Get photos with a specific tag.

```bash
nself plugin photos tags photos --value "sunset" --limit 50
```

### `faces list`

List detected face groups.

```bash
nself plugin photos faces list --limit 50 --offset 0
```

### `faces identify`

Identify a face group by associating it with a name or user.

```bash
# Identify by name
nself plugin photos faces identify \
  --id face-id \
  --name "John Doe"

# Identify by user ID
nself plugin photos faces identify \
  --id face-id \
  --user-id user123
```

### `faces merge`

Merge two face groups into one.

```bash
nself plugin photos faces merge \
  --target face-id-1 \
  --merge-with face-id-2
```

### `search`

Search photos by query, tags, location, and date range.

```bash
# Search by query
nself plugin photos search --query "beach vacation"

# Search with tags
nself plugin photos search --query "party" --tags "friends,celebration"

# Search with location
nself plugin photos search --query "sunset" --location "California"

# Search with date range
nself plugin photos search \
  --query "family" \
  --date-from "2025-01-01" \
  --date-to "2025-12-31"

# Combined search
nself plugin photos search \
  --query "wedding" \
  --tags "ceremony,reception" \
  --location "San Francisco" \
  --date-from "2025-06-01" \
  --date-to "2025-06-30" \
  --limit 100
```

### `timeline`

View photo timeline grouped by time periods.

```bash
# Monthly timeline (default)
nself plugin photos timeline

# Daily timeline
nself plugin photos timeline --granularity day

# Weekly timeline
nself plugin photos timeline --granularity week

# Yearly timeline
nself plugin photos timeline --granularity year

# Timeline for specific user
nself plugin photos timeline --user user123

# Timeline with date range
nself plugin photos timeline \
  --from "2025-01-01" \
  --to "2025-12-31" \
  --granularity month
```

### `stats`

Show photos plugin statistics.

```bash
nself plugin photos stats

# Example output:
# {
#   "totalAlbums": 42,
#   "totalPhotos": 1523,
#   "totalTags": 456,
#   "totalFaces": 23,
#   "pendingProcessing": 5,
#   "processedPhotos": 1518,
#   "totalStorageBytes": 5234567890
# }
```

### `status`

Show photos plugin status and configuration.

```bash
nself plugin photos status

# Multi-app support
nself plugin photos status --app-id app1
```

## REST API

### Health Check Endpoints

#### `GET /health`

Check if the server is running.

**Response:**
```json
{
  "status": "ok",
  "plugin": "photos",
  "timestamp": "2025-02-11T10:30:00Z",
  "version": "1.0.0"
}
```

#### `GET /ready`

Check if the server is ready to accept requests.

**Response:**
```json
{
  "ready": true,
  "database": "ok",
  "timestamp": "2025-02-11T10:30:00Z"
}
```

#### `GET /live`

Get server liveness information with statistics.

**Response:**
```json
{
  "alive": true,
  "uptime": 3600.5,
  "memory": {
    "used": 104857600,
    "total": 536870912
  },
  "stats": {
    "totalAlbums": 42,
    "totalPhotos": 1523,
    "totalTags": 456,
    "totalFaces": 23,
    "pendingProcessing": 5,
    "processedPhotos": 1518,
    "totalStorageBytes": 5234567890
  }
}
```

### Albums Endpoints

#### `POST /api/albums`

Create a new album.

**Headers:**
- `X-App-Name` (optional): Application ID for multi-app support
- `X-User-Id` (optional): User ID of the album owner

**Request Body:**
```json
{
  "name": "Vacation 2025",
  "description": "Summer vacation photos",
  "visibility": "private",
  "visibilityUserIds": ["user1", "user2"],
  "sortOrder": "date_desc",
  "metadata": {
    "location": "Hawaii",
    "tags": ["travel", "beach"]
  }
}
```

**Response:**
```json
{
  "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
  "source_account_id": "primary",
  "owner_id": "user123",
  "name": "Vacation 2025",
  "description": "Summer vacation photos",
  "cover_photo_id": null,
  "visibility": "private",
  "visibility_user_ids": ["user1", "user2"],
  "photo_count": 0,
  "sort_order": "date_desc",
  "date_range_start": null,
  "date_range_end": null,
  "location_name": null,
  "metadata": {
    "location": "Hawaii",
    "tags": ["travel", "beach"]
  },
  "created_at": "2025-02-11T10:30:00Z",
  "updated_at": "2025-02-11T10:30:00Z"
}
```

#### `GET /api/albums`

List albums with optional filtering and pagination.

**Query Parameters:**
- `ownerId` (optional): Filter by owner ID
- `visibility` (optional): Filter by visibility (public, private, shared)
- `limit` (optional, default: 50): Maximum number of albums to return
- `offset` (optional, default: 0): Number of albums to skip

**Response:**
```json
{
  "albums": [
    {
      "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
      "name": "Vacation 2025",
      "photo_count": 42,
      "cover_photo_id": "photo-id",
      "created_at": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 100
}
```

#### `GET /api/albums/:id`

Get album details by ID.

**Response:**
```json
{
  "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
  "source_account_id": "primary",
  "owner_id": "user123",
  "name": "Vacation 2025",
  "description": "Summer vacation photos",
  "photo_count": 42,
  "visibility": "private",
  "created_at": "2025-02-11T10:30:00Z",
  "updated_at": "2025-02-11T10:30:00Z"
}
```

#### `PUT /api/albums/:id`

Update album details.

**Request Body:**
```json
{
  "name": "Updated Album Name",
  "description": "Updated description",
  "coverPhotoId": "photo-id",
  "visibility": "public",
  "visibilityUserIds": ["user1"],
  "sortOrder": "date_asc"
}
```

#### `DELETE /api/albums/:id`

Delete an album.

**Response:** `204 No Content`

### Photos Endpoints

#### `POST /api/photos`

Register a new photo.

**Headers:**
- `X-App-Name` (optional): Application ID
- `X-User-Id` (optional): Uploader user ID

**Request Body:**
```json
{
  "albumId": "album-id",
  "fileId": "file-id",
  "originalUrl": "https://storage.example.com/photos/image.jpg",
  "originalFilename": "IMG_1234.jpg",
  "caption": "Beautiful sunset",
  "visibility": "album"
}
```

**Response:**
```json
{
  "id": "photo-id",
  "processingStatus": "pending"
}
```

#### `POST /api/photos/batch`

Register multiple photos in a single request.

**Request Body:**
```json
{
  "albumId": "album-id",
  "photos": [
    {
      "originalUrl": "https://storage.example.com/photos/image1.jpg",
      "originalFilename": "IMG_1234.jpg",
      "caption": "Photo 1"
    },
    {
      "originalUrl": "https://storage.example.com/photos/image2.jpg",
      "originalFilename": "IMG_1235.jpg",
      "caption": "Photo 2"
    }
  ]
}
```

**Response:**
```json
{
  "registered": 2,
  "photos": [
    { "id": "photo-id-1", "processingStatus": "pending" },
    { "id": "photo-id-2", "processingStatus": "pending" }
  ]
}
```

#### `GET /api/photos`

List photos with optional filtering.

**Query Parameters:**
- `albumId` (optional): Filter by album ID
- `uploaderId` (optional): Filter by uploader user ID
- `takenFrom` (optional): Filter photos taken after this date (ISO format)
- `takenTo` (optional): Filter photos taken before this date (ISO format)
- `limit` (optional, default: 50): Maximum results
- `offset` (optional, default: 0): Skip results

**Response:**
```json
{
  "photos": [
    {
      "id": "photo-id",
      "album_id": "album-id",
      "uploader_id": "user123",
      "original_url": "https://storage.example.com/photos/image.jpg",
      "thumbnail_medium_url": "https://storage.example.com/thumbs/medium/image.webp",
      "caption": "Beautiful sunset",
      "taken_at": "2025-02-10T18:30:00Z",
      "location_name": "San Francisco, CA",
      "camera_make": "Canon",
      "camera_model": "EOS R5",
      "processing_status": "completed",
      "created_at": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 1523
}
```

#### `GET /api/photos/:id`

Get photo details including tags.

**Response:**
```json
{
  "id": "photo-id",
  "album_id": "album-id",
  "uploader_id": "user123",
  "original_url": "https://storage.example.com/photos/image.jpg",
  "thumbnail_small_url": "https://storage.example.com/thumbs/small/image.webp",
  "thumbnail_medium_url": "https://storage.example.com/thumbs/medium/image.webp",
  "thumbnail_large_url": "https://storage.example.com/thumbs/large/image.webp",
  "width": 4000,
  "height": 3000,
  "np_fileproc_size_bytes": 2456789,
  "mime_type": "image/jpeg",
  "caption": "Beautiful sunset",
  "taken_at": "2025-02-10T18:30:00Z",
  "location_latitude": 37.7749,
  "location_longitude": -122.4194,
  "location_name": "San Francisco, CA",
  "camera_make": "Canon",
  "camera_model": "EOS R5",
  "focal_length": "24mm",
  "aperture": "f/2.8",
  "shutter_speed": "1/250",
  "iso": 100,
  "processing_status": "completed",
  "tags": [
    {
      "id": "tag-id",
      "tag_type": "keyword",
      "tag_value": "sunset",
      "created_at": "2025-02-11T10:30:00Z"
    },
    {
      "id": "tag-id-2",
      "tag_type": "location",
      "tag_value": "Golden Gate Bridge",
      "created_at": "2025-02-11T10:31:00Z"
    }
  ]
}
```

#### `PUT /api/photos/:id`

Update photo details.

**Request Body:**
```json
{
  "caption": "Updated caption",
  "albumId": "new-album-id",
  "visibility": "public"
}
```

#### `DELETE /api/photos/:id`

Delete a photo.

**Response:** `204 No Content`

#### `POST /api/photos/:id/move`

Move a photo to a different album.

**Request Body:**
```json
{
  "albumId": "new-album-id"
}
```

#### `POST /api/photos/:id/process`

Manually trigger processing for a specific photo.

**Response:**
```json
{
  "photoId": "photo-id",
  "status": "completed"
}
```

#### `POST /api/photos/process-pending`

Process pending photos in batch.

**Response:**
```json
{
  "processed": 10,
  "total": 10
}
```

### Tags Endpoints

#### `POST /api/photos/:id/tags`

Add a tag to a photo.

**Request Body:**
```json
{
  "tagType": "keyword",
  "tagValue": "sunset",
  "taggedUserId": null,
  "faceRegion": null
}
```

For person tags with face regions:
```json
{
  "tagType": "person",
  "tagValue": "John Doe",
  "taggedUserId": "user123",
  "faceRegion": {
    "x": 100,
    "y": 150,
    "width": 200,
    "height": 200
  }
}
```

**Response:**
```json
{
  "id": "tag-id",
  "source_account_id": "primary",
  "photo_id": "photo-id",
  "tag_type": "keyword",
  "tag_value": "sunset",
  "tagged_user_id": null,
  "face_region": null,
  "confidence": null,
  "created_by": null,
  "created_at": "2025-02-11T10:30:00Z"
}
```

#### `DELETE /api/photos/:id/tags/:tagId`

Remove a tag from a photo.

**Response:** `204 No Content`

#### `GET /api/tags`

List tags with counts.

**Query Parameters:**
- `type` (optional): Filter by tag type (keyword, person, location, event)
- `limit` (optional, default: 100): Maximum results

**Response:**
```json
{
  "tags": [
    { "value": "sunset", "count": 42 },
    { "value": "beach", "count": 38 },
    { "value": "family", "count": 156 }
  ]
}
```

#### `GET /api/tags/:value/photos`

Get photos with a specific tag.

**Query Parameters:**
- `limit` (optional, default: 50)
- `offset` (optional, default: 0)

**Response:**
```json
{
  "photos": [...],
  "total": 42
}
```

### Faces Endpoints

#### `GET /api/faces`

List detected face groups.

**Query Parameters:**
- `limit` (optional, default: 50)
- `offset` (optional, default: 0)

**Response:**
```json
{
  "faces": [
    {
      "id": "face-id",
      "name": "John Doe",
      "user_id": "user123",
      "representative_photo_id": "photo-id",
      "photo_count": 42,
      "confirmed": true,
      "created_at": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 23
}
```

#### `PUT /api/faces/:id`

Identify a face group.

**Request Body:**
```json
{
  "name": "John Doe",
  "userId": "user123"
}
```

#### `POST /api/faces/:id/merge`

Merge two face groups.

**Request Body:**
```json
{
  "mergeWithId": "face-id-to-merge"
}
```

### Timeline Endpoint

#### `GET /api/timeline`

Get photo timeline grouped by time periods.

**Query Parameters:**
- `userId` (optional): Filter by user ID
- `granularity` (optional, default: "month"): Time period granularity (day, week, month, year)
- `from` (optional): Start date (ISO format)
- `to` (optional): End date (ISO format)

**Response:**
```json
{
  "periods": [
    {
      "period": "2025-02",
      "count": 42,
      "coverPhotoUrl": "https://storage.example.com/thumbs/medium/cover.webp",
      "location": "San Francisco"
    },
    {
      "period": "2025-01",
      "count": 38,
      "coverPhotoUrl": "https://storage.example.com/thumbs/medium/cover2.webp",
      "location": "New York"
    }
  ]
}
```

### Search Endpoint

#### `POST /api/photos/search`

Search photos by query, tags, location, and date range.

**Request Body:**
```json
{
  "query": "beach vacation",
  "tags": ["sunset", "family"],
  "location": "California",
  "dateFrom": "2025-01-01",
  "dateTo": "2025-12-31",
  "uploaderId": "user123",
  "albumId": "album-id",
  "limit": 50,
  "offset": 0
}
```

**Response:**
```json
{
  "photos": [...],
  "total": 156
}
```

## Webhook Events

The Photos plugin emits webhook events that are stored in the `np_photos_webhook_events` table. Applications can poll this table or implement real-time webhooks to react to photo-related events.

| Event Type | Description | Payload |
|------------|-------------|---------|
| `photos.album.created` | A new album was created | `{ albumId, name, ownerId }` |
| `photos.album.deleted` | An album was deleted | `{ albumId }` |
| `photos.photo.uploaded` | A new photo was uploaded | `{ photoId, uploaderId }` |
| `photos.photo.processed` | Photo processing completed | `{ photoId }` |
| `photos.photo.deleted` | A photo was deleted | `{ photoId }` |
| `photos.tag.added` | A tag was added to a photo | `{ tagId, photoId, tagValue }` |
| `photos.face.identified` | A face group was identified | `{ faceId, name, userId }` |
| `photos.face.merged` | Two face groups were merged | `{ targetFaceId, mergedFaceId }` |

### Example Webhook Event Record

```sql
SELECT * FROM np_photos_webhook_events WHERE event_type = 'photos.photo.uploaded';
```

```
| id | source_account_id | event_type | payload | processed | created_at |
|----|-------------------|------------|---------|-----------|------------|
| photos.photo.uploaded-abc123 | primary | photos.photo.uploaded | {"photoId":"abc123","uploaderId":"user123"} | false | 2025-02-11 10:30:00+00 |
```

## Database Schema

### np_photos_albums

Stores photo albums with visibility and organization settings.

```sql
CREATE TABLE IF NOT EXISTS np_photos_albums (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  owner_id VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  cover_photo_id UUID,
  visibility VARCHAR(20) DEFAULT 'private',
  visibility_user_ids TEXT[],
  photo_count INTEGER DEFAULT 0,
  sort_order VARCHAR(20) DEFAULT 'date_desc',
  date_range_start DATE,
  date_range_end DATE,
  location_name VARCHAR(255),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_photos_albums_source_app ON np_photos_albums(source_account_id);
CREATE INDEX IF NOT EXISTS idx_photos_albums_owner ON np_photos_albums(source_account_id, owner_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID for multi-app support |
| `owner_id` | VARCHAR(255) | No | - | User ID of the album owner |
| `name` | VARCHAR(255) | No | - | Album name |
| `description` | TEXT | Yes | NULL | Album description |
| `cover_photo_id` | UUID | Yes | NULL | ID of the photo to use as album cover |
| `visibility` | VARCHAR(20) | No | `'private'` | Album visibility (public, private, shared) |
| `visibility_user_ids` | TEXT[] | Yes | NULL | Array of user IDs who can view shared albums |
| `photo_count` | INTEGER | No | `0` | Number of photos in the album (updated automatically) |
| `sort_order` | VARCHAR(20) | No | `'date_desc'` | Photo sort order (date_asc, date_desc, name_asc) |
| `date_range_start` | DATE | Yes | NULL | Start date of photos in album (auto-computed) |
| `date_range_end` | DATE | Yes | NULL | End date of photos in album (auto-computed) |
| `location_name` | VARCHAR(255) | Yes | NULL | Primary location of photos in album |
| `metadata` | JSONB | No | `'{}'` | Additional album metadata |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Album creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Album last update timestamp |

### np_photos_items

Stores individual photo records with EXIF data and processing status.

```sql
CREATE TABLE IF NOT EXISTS np_photos_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  album_id UUID REFERENCES np_photos_albums(id) ON DELETE SET NULL,
  uploader_id VARCHAR(255) NOT NULL,
  np_fileproc_id VARCHAR(255),
  original_url TEXT NOT NULL,
  thumbnail_small_url TEXT,
  thumbnail_medium_url TEXT,
  thumbnail_large_url TEXT,
  width INTEGER,
  height INTEGER,
  np_fileproc_size_bytes BIGINT,
  mime_type VARCHAR(50),
  original_filename VARCHAR(500),
  caption TEXT,
  visibility VARCHAR(20) DEFAULT 'album',
  taken_at TIMESTAMPTZ,
  location_latitude DOUBLE PRECISION,
  location_longitude DOUBLE PRECISION,
  location_name VARCHAR(255),
  camera_make VARCHAR(100),
  camera_model VARCHAR(100),
  focal_length VARCHAR(20),
  aperture VARCHAR(20),
  shutter_speed VARCHAR(20),
  iso INTEGER,
  orientation INTEGER DEFAULT 1,
  processing_status VARCHAR(20) DEFAULT 'pending',
  np_search_vector tsvector,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_photos_items_source_app ON np_photos_items(source_account_id);
CREATE INDEX IF NOT EXISTS idx_photos_items_album ON np_photos_items(album_id);
CREATE INDEX IF NOT EXISTS idx_photos_items_uploader ON np_photos_items(source_account_id, uploader_id);
CREATE INDEX IF NOT EXISTS idx_photos_items_taken ON np_photos_items(source_account_id, taken_at DESC);
CREATE INDEX IF NOT EXISTS idx_photos_items_location ON np_photos_items(location_latitude, location_longitude) WHERE location_latitude IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_photos_items_search ON np_photos_items USING GIN(np_search_vector);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `album_id` | UUID | Yes | NULL | ID of the album containing this photo |
| `uploader_id` | VARCHAR(255) | No | - | User ID who uploaded the photo |
| `np_fileproc_id` | VARCHAR(255) | Yes | NULL | External file storage ID |
| `original_url` | TEXT | No | - | URL to the original high-resolution photo |
| `thumbnail_small_url` | TEXT | Yes | NULL | URL to small thumbnail (default: 150px) |
| `thumbnail_medium_url` | TEXT | Yes | NULL | URL to medium thumbnail (default: 600px) |
| `thumbnail_large_url` | TEXT | Yes | NULL | URL to large thumbnail (default: 1200px) |
| `width` | INTEGER | Yes | NULL | Original photo width in pixels |
| `height` | INTEGER | Yes | NULL | Original photo height in pixels |
| `np_fileproc_size_bytes` | BIGINT | Yes | NULL | File size in bytes |
| `mime_type` | VARCHAR(50) | Yes | NULL | MIME type (e.g., image/jpeg) |
| `original_filename` | VARCHAR(500) | Yes | NULL | Original filename |
| `caption` | TEXT | Yes | NULL | User-provided photo caption |
| `visibility` | VARCHAR(20) | No | `'album'` | Photo visibility (album, public, private) |
| `taken_at` | TIMESTAMPTZ | Yes | NULL | When the photo was taken (from EXIF) |
| `location_latitude` | DOUBLE PRECISION | Yes | NULL | GPS latitude (from EXIF) |
| `location_longitude` | DOUBLE PRECISION | Yes | NULL | GPS longitude (from EXIF) |
| `location_name` | VARCHAR(255) | Yes | NULL | Reverse-geocoded location name |
| `camera_make` | VARCHAR(100) | Yes | NULL | Camera manufacturer (from EXIF) |
| `camera_model` | VARCHAR(100) | Yes | NULL | Camera model (from EXIF) |
| `focal_length` | VARCHAR(20) | Yes | NULL | Focal length (from EXIF) |
| `aperture` | VARCHAR(20) | Yes | NULL | Aperture value (from EXIF) |
| `shutter_speed` | VARCHAR(20) | Yes | NULL | Shutter speed (from EXIF) |
| `iso` | INTEGER | Yes | NULL | ISO value (from EXIF) |
| `orientation` | INTEGER | No | `1` | EXIF orientation value (1-8) |
| `processing_status` | VARCHAR(20) | No | `'pending'` | Processing status (pending, processing, completed, error) |
| `np_search_vector` | tsvector | Yes | NULL | Full-text search vector |
| `metadata` | JSONB | No | `'{}'` | Additional photo metadata |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Record last update timestamp |

### np_photos_tags

Stores tags applied to photos (keywords, people, locations, events).

```sql
CREATE TABLE IF NOT EXISTS np_photos_tags (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  photo_id UUID NOT NULL REFERENCES np_photos_items(id) ON DELETE CASCADE,
  tag_type VARCHAR(20) NOT NULL DEFAULT 'keyword',
  tag_value VARCHAR(255) NOT NULL,
  tagged_user_id VARCHAR(255),
  face_region JSONB,
  confidence DOUBLE PRECISION,
  created_by VARCHAR(255),
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, photo_id, tag_type, tag_value)
);

CREATE INDEX IF NOT EXISTS idx_photos_tags_source_app ON np_photos_tags(source_account_id);
CREATE INDEX IF NOT EXISTS idx_photos_tags_photo ON np_photos_tags(photo_id);
CREATE INDEX IF NOT EXISTS idx_photos_tags_value ON np_photos_tags(source_account_id, tag_type, tag_value);
CREATE INDEX IF NOT EXISTS idx_photos_tags_user ON np_photos_tags(source_account_id, tagged_user_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `photo_id` | UUID | No | - | ID of the tagged photo |
| `tag_type` | VARCHAR(20) | No | `'keyword'` | Tag type (keyword, person, location, event) |
| `tag_value` | VARCHAR(255) | No | - | Tag value (e.g., "sunset", "John Doe") |
| `tagged_user_id` | VARCHAR(255) | Yes | NULL | User ID for person tags |
| `face_region` | JSONB | Yes | NULL | Face bounding box coordinates {x, y, width, height} |
| `confidence` | DOUBLE PRECISION | Yes | NULL | AI confidence score (0.0-1.0) for auto-detected tags |
| `created_by` | VARCHAR(255) | Yes | NULL | User ID who created the tag |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Tag creation timestamp |

### np_photos_faces

Stores detected face groups for facial recognition.

```sql
CREATE TABLE IF NOT EXISTS np_photos_faces (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255),
  user_id VARCHAR(255),
  representative_photo_id UUID REFERENCES np_photos_items(id),
  photo_count INTEGER DEFAULT 0,
  confirmed BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_photos_faces_source_app ON np_photos_faces(source_account_id);
CREATE INDEX IF NOT EXISTS idx_photos_faces_user ON np_photos_faces(source_account_id, user_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `name` | VARCHAR(255) | Yes | NULL | Identified person name |
| `user_id` | VARCHAR(255) | Yes | NULL | Associated user ID |
| `representative_photo_id` | UUID | Yes | NULL | Photo ID to use as face representative |
| `photo_count` | INTEGER | No | `0` | Number of photos containing this face |
| `confirmed` | BOOLEAN | No | `false` | Whether the face identification is confirmed |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Face group creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Face group last update timestamp |

### np_photos_webhook_events

Stores webhook events for asynchronous processing.

```sql
CREATE TABLE IF NOT EXISTS np_photos_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMPTZ,
  error TEXT,
  retry_count INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_photos_webhook_events_source_app ON np_photos_webhook_events(source_account_id);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | VARCHAR(255) | No | - | Event ID (generated by plugin) |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Application/tenant ID |
| `event_type` | VARCHAR(128) | No | - | Event type (e.g., photos.photo.uploaded) |
| `payload` | JSONB | No | - | Event payload data |
| `processed` | BOOLEAN | No | `false` | Whether the event has been processed |
| `processed_at` | TIMESTAMPTZ | Yes | NULL | When the event was processed |
| `error` | TEXT | Yes | NULL | Processing error message |
| `retry_count` | INTEGER | No | `0` | Number of processing retry attempts |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Event creation timestamp |

## Examples

### Example 1: Complete Photo Upload and Organization Workflow

```bash
# 1. Create an album
ALBUM_ID=$(nself plugin photos albums create \
  --name "Vacation 2025" \
  --description "Summer vacation photos" \
  --visibility private | jq -r '.id')

# 2. Upload photos via REST API
curl -X POST http://localhost:3023/api/photos/batch \
  -H "Content-Type: application/json" \
  -H "X-User-Id: user123" \
  -d '{
    "albumId": "'$ALBUM_ID'",
    "photos": [
      {
        "originalUrl": "https://storage.example.com/photos/beach1.jpg",
        "originalFilename": "IMG_1234.jpg",
        "caption": "Beautiful beach day"
      },
      {
        "originalUrl": "https://storage.example.com/photos/sunset.jpg",
        "originalFilename": "IMG_1235.jpg",
        "caption": "Amazing sunset"
      }
    ]
  }'

# 3. Process pending photos
nself plugin photos process-pending

# 4. Add tags to photos
PHOTO_ID=$(curl http://localhost:3023/api/photos?albumId=$ALBUM_ID | jq -r '.photos[0].id')

curl -X POST http://localhost:3023/api/photos/$PHOTO_ID/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tagType": "keyword",
    "tagValue": "beach"
  }'

curl -X POST http://localhost:3023/api/photos/$PHOTO_ID/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tagType": "location",
    "tagValue": "Hawaii"
  }'

# 5. View the album
nself plugin photos albums get --id $ALBUM_ID
```

### Example 2: Search Photos by Multiple Criteria

```sql
-- Search for beach photos in Hawaii taken in 2025
SELECT
  pi.id,
  pi.caption,
  pi.original_url,
  pi.taken_at,
  pi.location_name,
  pi.camera_model,
  ARRAY_AGG(pt.tag_value) as tags
FROM np_photos_items pi
LEFT JOIN np_photos_tags pt ON pt.photo_id = pi.id
WHERE pi.source_account_id = 'primary'
  AND pi.np_search_vector @@ plainto_tsquery('english', 'beach')
  AND pi.location_name ILIKE '%Hawaii%'
  AND pi.taken_at >= '2025-01-01'
  AND pi.taken_at < '2026-01-01'
GROUP BY pi.id
ORDER BY pi.taken_at DESC
LIMIT 50;
```

### Example 3: Generate Photo Timeline Report

```sql
-- Monthly photo timeline with counts and cover photos
SELECT
  TO_CHAR(taken_at, 'YYYY-MM') as month,
  COUNT(*) as photo_count,
  MODE() WITHIN GROUP (ORDER BY location_name) as primary_location,
  (SELECT thumbnail_medium_url FROM np_photos_items pi2
   WHERE pi2.source_account_id = np_photos_items.source_account_id
     AND TO_CHAR(pi2.taken_at, 'YYYY-MM') = TO_CHAR(np_photos_items.taken_at, 'YYYY-MM')
   ORDER BY pi2.taken_at ASC LIMIT 1) as cover_photo_url
FROM np_photos_items
WHERE source_account_id = 'primary'
  AND taken_at IS NOT NULL
  AND taken_at >= NOW() - INTERVAL '1 year'
GROUP BY month, source_account_id
ORDER BY month DESC;
```

### Example 4: Face Detection and Grouping

```bash
# 1. Enable face detection in .env
export PHOTOS_FACE_DETECTION=true
export PHOTOS_FACE_DETECTION_PROVIDER=aws-rekognition
export PHOTOS_FACE_DETECTION_API_KEY=your-api-key

# 2. Process photos to detect faces
nself plugin photos process-pending

# 3. List detected face groups
nself plugin photos faces list

# 4. Identify a face group
FACE_ID=$(nself plugin photos faces list | jq -r '.faces[0].id')
nself plugin photos faces identify \
  --id $FACE_ID \
  --name "John Doe" \
  --user-id user123

# 5. Merge duplicate face groups
FACE_ID_2=$(nself plugin photos faces list | jq -r '.faces[1].id')
nself plugin photos faces merge \
  --target $FACE_ID \
  --merge-with $FACE_ID_2

# 6. Query photos with a specific person
curl "http://localhost:3023/api/tags/John%20Doe/photos?limit=50"
```

### Example 5: Multi-App Photo Isolation

```bash
# Configure multiple apps
export PHOTOS_APP_IDS=app1,app2,app3

# Create album for app1
nself plugin photos albums create \
  --name "App1 Album" \
  --app-id app1

# Create album for app2
nself plugin photos albums create \
  --name "App2 Album" \
  --app-id app2

# List albums only for app1
nself plugin photos albums list --app-id app1

# List albums only for app2
nself plugin photos albums list --app-id app2

# Photos in app1 are completely isolated from app2
```

### Example 6: Advanced Photo Search

```http
POST http://localhost:3023/api/photos/search HTTP/1.1
Content-Type: application/json
X-App-Name: primary

{
  "query": "wedding ceremony",
  "tags": ["bride", "groom", "family"],
  "location": "San Francisco",
  "dateFrom": "2025-06-01",
  "dateTo": "2025-06-30",
  "limit": 100,
  "offset": 0
}
```

### Example 7: Photo Processing Status Monitoring

```sql
-- Monitor photo processing queue
SELECT
  processing_status,
  COUNT(*) as count,
  AVG(EXTRACT(EPOCH FROM (NOW() - created_at))) as avg_wait_seconds
FROM np_photos_items
WHERE source_account_id = 'primary'
  AND created_at >= NOW() - INTERVAL '1 day'
GROUP BY processing_status
ORDER BY
  CASE processing_status
    WHEN 'error' THEN 1
    WHEN 'processing' THEN 2
    WHEN 'pending' THEN 3
    WHEN 'completed' THEN 4
  END;
```

## Troubleshooting

### Common Issues

#### 1. Photos Not Processing

**Symptom:** Photos remain in "pending" status indefinitely.

**Solutions:**
- Check if the photos server is running: `curl http://localhost:3023/health`
- Manually trigger processing: `nself plugin photos process-pending`
- Check server logs for EXIF extraction errors
- Verify thumbnail storage is accessible and writable
- Increase processing concurrency: `export PHOTOS_PROCESSING_CONCURRENCY=8`

#### 2. Face Detection Not Working

**Symptom:** Face detection is enabled but no faces are detected.

**Solutions:**
- Verify face detection is enabled: `echo $PHOTOS_FACE_DETECTION` should output `true`
- Check provider configuration:
  ```bash
  echo $PHOTOS_FACE_DETECTION_PROVIDER  # Should output: aws-rekognition, google-vision, or opencv
  echo $PHOTOS_FACE_DETECTION_API_KEY   # Should be set if using external provider
  ```
- Test provider API key with a sample image
- Check server logs for face detection errors
- Verify image format is supported (JPEG, PNG)

#### 3. Search Not Returning Expected Results

**Symptom:** Full-text search returns no results or incomplete results.

**Solutions:**
- Ensure photos have been processed: Check `processing_status = 'completed'`
- Verify search vector is populated:
  ```sql
  SELECT id, caption, np_search_vector
  FROM np_photos_items
  WHERE np_search_vector IS NOT NULL
  LIMIT 5;
  ```
- Rebuild search vectors:
  ```sql
  UPDATE np_photos_items
  SET np_search_vector = to_tsvector('english',
    COALESCE(caption, '') || ' ' ||
    COALESCE(original_filename, '') || ' ' ||
    COALESCE(location_name, '')
  );
  ```
- Check search query syntax - use simple keywords, not complex phrases

#### 4. Large Storage Usage

**Symptom:** Database or file storage growing rapidly.

**Solutions:**
- Check total storage: `nself plugin photos stats` and inspect `totalStorageBytes`
- Identify largest albums:
  ```sql
  SELECT
    a.name,
    a.photo_count,
    SUM(p.np_fileproc_size_bytes) as total_bytes,
    pg_size_pretty(SUM(p.np_fileproc_size_bytes)) as total_size
  FROM np_photos_albums a
  JOIN np_photos_items p ON p.album_id = a.id
  WHERE a.source_account_id = 'primary'
  GROUP BY a.id
  ORDER BY total_bytes DESC
  LIMIT 10;
  ```
- Delete unused albums and photos
- Implement retention policy to auto-delete old photos
- Use smaller thumbnail sizes to reduce storage: `export PHOTOS_THUMBNAIL_LARGE=800`
- Use WebP format for thumbnails: `export PHOTOS_THUMBNAIL_FORMAT=webp`

#### 5. Slow Timeline or Search Queries

**Symptom:** Timeline or search API endpoints are slow.

**Solutions:**
- Ensure database indexes exist:
  ```sql
  SELECT indexname FROM pg_indexes WHERE tablename = 'np_photos_items';
  ```
- Add custom indexes for common queries:
  ```sql
  CREATE INDEX idx_photos_items_taken_at_location
  ON np_photos_items(source_account_id, taken_at DESC, location_name)
  WHERE taken_at IS NOT NULL;
  ```
- Use pagination with smaller page sizes
- Optimize search queries by adding date range filters
- Consider using a dedicated search index (Elasticsearch, Meilisearch) for large photo libraries

---

**Need more help?** Check the [main documentation](https://github.com/acamarata/nself-plugins) or [open an issue](https://github.com/acamarata/nself-plugins/issues).
