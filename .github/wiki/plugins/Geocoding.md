# Geocoding Plugin

Geocoding and location services plugin - forward/reverse geocoding, place search, geofences

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Geocoding plugin provides comprehensive location services for the nself platform. It enables forward and reverse geocoding, place search, geofencing capabilities, and maintains a high-performance cache to minimize external API costs.

### Key Features

- **Multi-Provider Support** - Google Maps, Mapbox, Nominatim (OpenStreetMap)
- **Forward Geocoding** - Convert addresses to coordinates
- **Reverse Geocoding** - Convert coordinates to addresses
- **Place Search** - Find places by name, category, or proximity
- **Geofencing** - Create circular and polygon geofences with enter/exit events
- **Smart Caching** - Reduce costs with 365-day default cache TTL
- **Batch Operations** - Geocode multiple addresses efficiently
- **Cache Analytics** - Track hit rates and provider usage
- **Multi-Account Support** - `source_account_id` isolation for multi-workspace deployments

### Supported Providers

| Provider | Type | Cost | Features |
|----------|------|------|----------|
| Google Maps | Commercial | Paid | High accuracy, place details, autocomplete |
| Mapbox | Commercial | Paid | Global coverage, custom styling |
| Nominatim | Open Source | Free | OpenStreetMap data, no API key required |

---

## Quick Start

```bash
# Install the plugin
nself plugin install geocoding

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export GEOCODING_PLUGIN_PORT=3203

# Optional: Configure a provider
export GEOCODING_PROVIDERS=nominatim
export GEOCODING_NOMINATIM_EMAIL=your-email@example.com

# Initialize database schema
nself plugin geocoding init

# Start the server
nself plugin geocoding server --port 3203

# Check status
nself plugin geocoding status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `GEOCODING_PLUGIN_PORT` | No | `3203` | HTTP server port |
| `GEOCODING_PROVIDERS` | No | `nominatim` | Comma-separated provider list |
| `GEOCODING_GOOGLE_API_KEY` | No | - | Google Maps API key |
| `GEOCODING_MAPBOX_ACCESS_TOKEN` | No | - | Mapbox access token |
| `GEOCODING_NOMINATIM_URL` | No | `https://nominatim.openstreetmap.org` | Nominatim API URL |
| `GEOCODING_NOMINATIM_EMAIL` | No | - | Email for Nominatim usage policy |
| `GEOCODING_CACHE_TTL_DAYS` | No | `365` | Cache TTL in days (0 = no expiration) |
| `GEOCODING_CACHE_ENABLED` | No | `true` | Enable caching layer |
| `GEOCODING_MAX_BATCH_SIZE` | No | `100` | Maximum addresses per batch request |
| `GEOCODING_RATE_LIMIT_PROVIDER` | No | `10` | Provider API rate limit (req/sec) |
| `GEOCODING_GEOFENCE_CHECK_TOLERANCE_METERS` | No | `50` | Tolerance for geofence boundary checks |
| `GEOCODING_NOTIFY_URL` | No | - | Webhook URL for geofence events |
| `GEOCODING_API_KEY` | No | - | API key for authentication (optional) |
| `GEOCODING_RATE_LIMIT_MAX` | No | `500` | Maximum requests per window |
| `GEOCODING_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=nself
POSTGRES_PASSWORD=secure_password
POSTGRES_SSL=false

# Server
GEOCODING_PLUGIN_PORT=3203
GEOCODING_PLUGIN_HOST=0.0.0.0

# Provider Configuration
GEOCODING_PROVIDERS=nominatim,google
GEOCODING_GOOGLE_API_KEY=your_google_api_key
GEOCODING_MAPBOX_ACCESS_TOKEN=your_mapbox_token
GEOCODING_NOMINATIM_URL=https://nominatim.openstreetmap.org
GEOCODING_NOMINATIM_EMAIL=your-email@example.com

# Cache Configuration
GEOCODING_CACHE_TTL_DAYS=365
GEOCODING_CACHE_ENABLED=true
GEOCODING_MAX_BATCH_SIZE=100

# Rate Limiting
GEOCODING_RATE_LIMIT_PROVIDER=10

# Geofencing
GEOCODING_GEOFENCE_CHECK_TOLERANCE_METERS=50
GEOCODING_NOTIFY_URL=http://localhost:4000/webhooks/geofence

# Security (optional)
GEOCODING_API_KEY=your_api_key_here
GEOCODING_RATE_LIMIT_MAX=500
GEOCODING_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin geocoding init

# Start the server
nself plugin geocoding server
nself plugin geocoding server --port 3203 --host 0.0.0.0

# Check status and statistics
nself plugin geocoding status
```

### Geocoding Commands

```bash
# Forward geocode an address
nself plugin geocoding geocode "1600 Pennsylvania Avenue NW, Washington, DC"

# Reverse geocode coordinates
nself plugin geocoding reverse 38.8977 -77.0365

# Search for places
nself plugin geocoding search "coffee shops"

# Search near location
nself plugin geocoding search "pizza" --near "40.7589,-73.9851" --radius 2000

# Search with category filter
nself plugin geocoding search "restaurant" --category "italian"

# Limit results
nself plugin geocoding search "parks" --limit 10

# Batch geocode from CSV
nself plugin geocoding batch addresses.csv
```

### Geofence Commands

```bash
# List all geofences
nself plugin geocoding geofences list

# Create a circular geofence
nself plugin geocoding geofences create \
  --name "Office Area" \
  --lat 37.7749 \
  --lng -122.4194 \
  --radius 500

# Update a geofence
nself plugin geocoding geofences update <geofence-id> --radius 1000

# Delete a geofence
nself plugin geocoding geofences delete <geofence-id>
```

### Cache Commands

```bash
# Show cache statistics
nself plugin geocoding cache stats

# Clear all cache
nself plugin geocoding cache clear

# Clear cache older than N days
nself plugin geocoding cache clear --days 180
```

---

## REST API

### Base URL

```
http://localhost:3203
```

### Health & Status

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "geocoding",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check endpoint (checks database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "geocoding",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /live
Liveness endpoint with runtime stats.

**Response:**
```json
{
  "alive": true,
  "plugin": "geocoding",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 104857600,
    "heapTotal": 52428800,
    "heapUsed": 41943040,
    "external": 1048576
  },
  "stats": {
    "cacheEntries": 12543,
    "geofences": 23,
    "places": 456,
    "cacheHitRate": 87.3
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Geocoding Endpoints

#### POST /api/geocode
Forward geocode an address to coordinates.

**Request Body:**
```json
{
  "address": "1600 Pennsylvania Avenue NW",
  "city": "Washington",
  "state": "DC",
  "country": "USA"
}
```

**Response:**
```json
{
  "data": [
    {
      "lat": 38.8977,
      "lng": -77.0365,
      "formatted_address": "1600 Pennsylvania Avenue NW, Washington, DC 20500, USA",
      "street_number": "1600",
      "street_name": "Pennsylvania Avenue NW",
      "city": "Washington",
      "state": "District of Columbia",
      "state_code": "DC",
      "country": "United States",
      "country_code": "US",
      "postal_code": "20500",
      "place_id": "ChIJGVtI4by3t4kRr51d_Qm_x58",
      "place_type": "street_address",
      "accuracy": "rooftop",
      "provider": "google",
      "cached": false
    }
  ]
}
```

#### POST /api/reverse
Reverse geocode coordinates to an address.

**Request Body:**
```json
{
  "lat": 38.8977,
  "lng": -77.0365
}
```

**Response:**
```json
{
  "data": [
    {
      "lat": 38.8977,
      "lng": -77.0365,
      "formatted_address": "1600 Pennsylvania Avenue NW, Washington, DC 20500, USA",
      "city": "Washington",
      "state": "District of Columbia",
      "state_code": "DC",
      "country": "United States",
      "country_code": "US",
      "postal_code": "20500",
      "provider": "google",
      "cached": true
    }
  ]
}
```

#### POST /api/search
Search for places.

**Request Body:**
```json
{
  "query": "coffee shop",
  "lat": 37.7749,
  "lng": -122.4194,
  "radius": 5000,
  "category": "cafe",
  "limit": 20
}
```

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "provider": "google",
      "provider_place_id": "ChIJxeyK9Z3wloAR_gOA7SycJC0",
      "name": "Blue Bottle Coffee",
      "category": "cafe",
      "lat": 37.7750,
      "lng": -122.4183,
      "formatted_address": "66 Mint St, San Francisco, CA 94103",
      "phone": "+14158963394",
      "website": "https://bluebottlecoffee.com",
      "rating": 4.5,
      "review_count": 1234,
      "hours": {
        "monday": "7:00 AM - 6:00 PM",
        "tuesday": "7:00 AM - 6:00 PM"
      },
      "photos": [
        "https://example.com/photo1.jpg"
      ],
      "metadata": {}
    }
  ],
  "total": 15
}
```

#### POST /api/autocomplete
Autocomplete address or place input.

**Request Body:**
```json
{
  "input": "1600 Penn",
  "lat": 38.9072,
  "lng": -77.0369
}
```

**Response:**
```json
{
  "data": [],
  "message": "Autocomplete provider integration pending"
}
```

#### POST /api/batch
Batch geocode multiple addresses.

**Request Body:**
```json
{
  "addresses": [
    "1600 Pennsylvania Avenue NW, Washington, DC",
    "350 Fifth Avenue, New York, NY",
    "1 Infinite Loop, Cupertino, CA"
  ]
}
```

**Response:**
```json
{
  "data": [
    {
      "address": "1600 Pennsylvania Avenue NW, Washington, DC",
      "result": {
        "lat": 38.8977,
        "lng": -77.0365,
        "formatted_address": "1600 Pennsylvania Avenue NW, Washington, DC 20500, USA",
        "cached": true
      }
    },
    {
      "address": "350 Fifth Avenue, New York, NY",
      "result": {
        "lat": 40.748817,
        "lng": -73.985428,
        "formatted_address": "350 5th Ave, New York, NY 10118, USA",
        "cached": false
      }
    }
  ],
  "total": 3,
  "cached": 1,
  "failed": 0
}
```

### Geofence Endpoints

#### GET /api/geofences
List all geofences.

**Query Parameters:**
- `active` (optional): Filter by active status (true/false)
- `near_lat` (optional): Filter by proximity (latitude)
- `near_lng` (optional): Filter by proximity (longitude)
- `radius` (optional): Proximity search radius in meters (default: 10000)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "source_account_id": "primary",
      "name": "Office Area",
      "description": "Main office geofence",
      "fence_type": "circle",
      "center_lat": 37.7749,
      "center_lng": -122.4194,
      "radius_meters": 500,
      "polygon": null,
      "active": true,
      "notify_on_enter": true,
      "notify_on_exit": true,
      "notify_url": "http://localhost:4000/webhooks/geofence",
      "metadata": {},
      "created_by": "admin",
      "created_at": "2026-01-01T00:00:00.000Z",
      "updated_at": "2026-02-11T10:00:00.000Z",
      "deleted_at": null
    }
  ],
  "total": 23
}
```

#### POST /api/geofences
Create a new geofence.

**Request Body:**
```json
{
  "name": "Office Area",
  "description": "Main office geofence",
  "fence_type": "circle",
  "center_lat": 37.7749,
  "center_lng": -122.4194,
  "radius_meters": 500,
  "notify_on_enter": true,
  "notify_on_exit": true,
  "notify_url": "http://localhost:4000/webhooks/geofence",
  "metadata": {
    "alert_level": "high"
  },
  "created_by": "admin"
}
```

**Response:** Returns created geofence object (201 status).

#### PUT /api/geofences/:id
Update an existing geofence.

**Request Body:**
```json
{
  "name": "Extended Office Area",
  "radius_meters": 1000,
  "active": true
}
```

**Response:** Returns updated geofence object.

#### DELETE /api/geofences/:id
Soft delete a geofence.

**Response:**
```json
{
  "deleted": true
}
```

#### POST /api/geofences/evaluate
Evaluate which geofences a point is inside.

**Request Body:**
```json
{
  "lat": 37.7750,
  "lng": -122.4195,
  "entity_id": "user123",
  "entity_type": "user"
}
```

**Response:**
```json
{
  "data": [
    {
      "geofence_id": "550e8400-e29b-41d4-a716-446655440001",
      "geofence_name": "Office Area",
      "inside": true,
      "distance_meters": 45
    },
    {
      "geofence_id": "550e8400-e29b-41d4-a716-446655440002",
      "geofence_name": "Downtown Zone",
      "inside": false,
      "distance_meters": 2340
    }
  ],
  "inside_count": 1
}
```

#### GET /api/geofences/:id/events
Get events for a specific geofence.

**Query Parameters:**
- `from` (optional): Start date/time (ISO 8601)
- `to` (optional): End date/time (ISO 8601)
- `entity_id` (optional): Filter by entity ID

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440003",
      "geofence_id": "550e8400-e29b-41d4-a716-446655440001",
      "event_type": "enter",
      "entity_id": "user123",
      "entity_type": "user",
      "lat": 37.7750,
      "lng": -122.4195,
      "notified": true,
      "notified_at": "2026-02-11T10:05:00.000Z",
      "created_at": "2026-02-11T10:05:00.000Z"
    }
  ],
  "total": 45
}
```

### Cache Endpoints

#### GET /api/cache/stats
Get cache statistics.

**Response:**
```json
{
  "total_entries": 12543,
  "active_entries": 12234,
  "expired_entries": 309,
  "total_hits": 45678,
  "avg_hits_per_entry": 3.64,
  "reuse_percentage": 72.3,
  "by_query_type": {
    "forward": 8934,
    "reverse": 3609
  },
  "by_provider": {
    "google": 7234,
    "nominatim": 5309
  }
}
```

#### POST /api/cache/clear
Clear cache entries.

**Request Body:**
```json
{
  "older_than_days": 180
}
```

**Response:**
```json
{
  "cleared": 309,
  "older_than_days": 180
}
```

### Stats Endpoint

#### GET /api/stats
Get overall plugin statistics.

**Response:**
```json
{
  "total_cache_entries": 12543,
  "total_geofences": 23,
  "active_geofences": 21,
  "total_geofence_events": 4567,
  "total_places": 456,
  "cache_hit_rate": 87.3,
  "by_provider": {
    "google": 7234,
    "nominatim": 5309
  }
}
```

---

## Database Schema

### np_geoc_cache
Stores geocoding results with hit tracking.

```sql
CREATE TABLE np_geoc_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  query_type VARCHAR(16) NOT NULL,
  query_hash VARCHAR(64) NOT NULL,
  query_text TEXT NOT NULL,
  provider VARCHAR(64) NOT NULL,
  lat DOUBLE PRECISION,
  lng DOUBLE PRECISION,
  formatted_address TEXT,
  street_number VARCHAR(32),
  street_name VARCHAR(255),
  city VARCHAR(255),
  state VARCHAR(128),
  state_code VARCHAR(8),
  country VARCHAR(128),
  country_code VARCHAR(4),
  postal_code VARCHAR(32),
  place_id VARCHAR(255),
  place_type VARCHAR(64),
  accuracy VARCHAR(32),
  bounds JSONB,
  raw_response JSONB,
  hit_count INTEGER DEFAULT 1,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  expires_at TIMESTAMPTZ,
  UNIQUE(source_account_id, query_hash, provider)
);

CREATE INDEX idx_geo_cache_source_account ON np_geoc_cache(source_account_id);
CREATE INDEX idx_geo_cache_hash ON np_geoc_cache(query_hash);
CREATE INDEX idx_geo_cache_coords ON np_geoc_cache(lat, lng);
CREATE INDEX idx_geo_cache_city ON np_geoc_cache(city, state_code);
CREATE INDEX idx_geo_cache_expires ON np_geoc_cache(expires_at);
```

### np_geoc_geofences
Stores geofence definitions.

```sql
CREATE TABLE np_geoc_geofences (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  fence_type VARCHAR(16) NOT NULL DEFAULT 'circle',
  center_lat DOUBLE PRECISION NOT NULL,
  center_lng DOUBLE PRECISION NOT NULL,
  radius_meters DOUBLE PRECISION,
  polygon JSONB,
  active BOOLEAN DEFAULT TRUE,
  notify_on_enter BOOLEAN DEFAULT TRUE,
  notify_on_exit BOOLEAN DEFAULT TRUE,
  notify_url TEXT,
  metadata JSONB DEFAULT '{}',
  created_by VARCHAR(255),
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_geo_geofences_source_account ON np_geoc_geofences(source_account_id);
CREATE INDEX idx_geo_geofences_center ON np_geoc_geofences(center_lat, center_lng);
CREATE INDEX idx_geo_geofences_active ON np_geoc_geofences(active);
```

### np_geoc_geofence_events
Logs geofence entry/exit events.

```sql
CREATE TABLE np_geoc_geofence_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  geofence_id UUID REFERENCES np_geoc_geofences(id),
  event_type VARCHAR(16) NOT NULL,
  entity_id VARCHAR(255) NOT NULL,
  entity_type VARCHAR(64) NOT NULL DEFAULT 'user',
  lat DOUBLE PRECISION NOT NULL,
  lng DOUBLE PRECISION NOT NULL,
  notified BOOLEAN DEFAULT FALSE,
  notified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_geo_events_source_account ON np_geoc_geofence_events(source_account_id);
CREATE INDEX idx_geo_events_fence ON np_geoc_geofence_events(geofence_id);
CREATE INDEX idx_geo_events_entity ON np_geoc_geofence_events(entity_id);
```

### np_geoc_places
Stores place information from searches.

```sql
CREATE TABLE np_geoc_places (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  provider VARCHAR(64) NOT NULL,
  provider_place_id VARCHAR(255) NOT NULL,
  name VARCHAR(512) NOT NULL,
  category VARCHAR(128),
  lat DOUBLE PRECISION NOT NULL,
  lng DOUBLE PRECISION NOT NULL,
  formatted_address TEXT,
  phone VARCHAR(32),
  website TEXT,
  rating FLOAT,
  review_count INTEGER,
  hours JSONB,
  photos JSONB DEFAULT '[]',
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, provider, provider_place_id)
);

CREATE INDEX idx_geo_places_source_account ON np_geoc_places(source_account_id);
CREATE INDEX idx_geo_places_coords ON np_geoc_places(lat, lng);
CREATE INDEX idx_geo_places_name ON np_geoc_places(name);
CREATE INDEX idx_geo_places_category ON np_geoc_places(category);
```

### Analytics Views

#### np_geoc_cache_hit_rate
Cache performance metrics by query type.

```sql
CREATE OR REPLACE VIEW np_geoc_cache_hit_rate AS
SELECT source_account_id,
       query_type,
       COUNT(*) AS total_entries,
       SUM(hit_count) AS total_hits,
       ROUND(AVG(hit_count), 2) AS avg_hits_per_entry,
       COUNT(*) FILTER (WHERE hit_count > 1) AS reused_entries,
       ROUND(100.0 * COUNT(*) FILTER (WHERE hit_count > 1) / NULLIF(COUNT(*), 0), 1) AS reuse_pct
FROM np_geoc_cache
WHERE expires_at IS NULL OR expires_at > NOW()
GROUP BY source_account_id, query_type;
```

#### np_geoc_volume_daily
Daily geocoding volume.

```sql
CREATE OR REPLACE VIEW np_geoc_volume_daily AS
SELECT source_account_id,
       provider,
       DATE(created_at) AS day,
       COUNT(*) AS geocode_count,
       COUNT(DISTINCT query_hash) AS unique_queries
FROM np_geoc_cache
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY source_account_id, provider, DATE(created_at)
ORDER BY day DESC;
```

#### np_geoc_geofence_activity
Geofence event statistics.

```sql
CREATE OR REPLACE VIEW np_geoc_geofence_activity AS
SELECT g.source_account_id,
       g.id AS geofence_id,
       g.name AS geofence_name,
       g.fence_type,
       COUNT(e.id) AS total_events,
       COUNT(e.id) FILTER (WHERE e.event_type = 'enter') AS enter_count,
       COUNT(e.id) FILTER (WHERE e.event_type = 'exit') AS exit_count,
       COUNT(DISTINCT e.entity_id) AS unique_entities,
       MAX(e.created_at) AS last_event_at
FROM np_geoc_geofences g
LEFT JOIN np_geoc_geofence_events e ON g.id = e.geofence_id
WHERE g.active = TRUE AND g.deleted_at IS NULL
GROUP BY g.source_account_id, g.id, g.name, g.fence_type;
```

---

## Examples

### Example 1: Forward Geocode with Caching

```bash
# First request (cache miss)
curl -X POST http://localhost:3203/api/geocode \
  -H "Content-Type: application/json" \
  -d '{"address": "1600 Pennsylvania Avenue NW, Washington, DC"}'

# Response: cached=false, provider call made

# Second request (cache hit)
curl -X POST http://localhost:3203/api/geocode \
  -H "Content-Type: application/json" \
  -d '{"address": "1600 Pennsylvania Avenue NW, Washington, DC"}'

# Response: cached=true, instant response from database
```

### Example 2: Proximity Geofencing

```javascript
// Create a geofence around an office
const geofence = await fetch('http://localhost:3203/api/geofences', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    name: 'Office Area',
    center_lat: 37.7749,
    center_lng: -122.4194,
    radius_meters: 500,
    notify_on_enter: true,
    notify_url: 'https://myapp.com/webhooks/geofence'
  })
});

// Check if user is inside geofence
const evaluation = await fetch('http://localhost:3203/api/geofences/evaluate', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    lat: 37.7750,
    lng: -122.4195,
    entity_id: 'user123',
    entity_type: 'user'
  })
});

// Response shows user is inside (45 meters from center)
```

### Example 3: Batch Geocoding

```bash
# Create a CSV file
cat > addresses.csv <<EOF
1600 Pennsylvania Avenue NW, Washington, DC
350 Fifth Avenue, New York, NY
1 Infinite Loop, Cupertino, CA
EOF

# Batch geocode
curl -X POST http://localhost:3203/api/batch \
  -H "Content-Type: application/json" \
  -d '{
    "addresses": [
      "1600 Pennsylvania Avenue NW, Washington, DC",
      "350 Fifth Avenue, New York, NY",
      "1 Infinite Loop, Cupertino, CA"
    ]
  }'

# All results returned in one response
# Cached results are instant, new ones call provider
```

### Example 4: Place Search Near Location

```bash
# Find coffee shops within 2km of coordinates
curl -X POST http://localhost:3203/api/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "coffee shop",
    "lat": 37.7749,
    "lng": -122.4194,
    "radius": 2000,
    "limit": 10
  }'

# Returns up to 10 coffee shops with ratings, hours, etc.
```

### Example 5: Monitor Cache Performance

```sql
-- Check cache hit rate by provider
SELECT
  provider,
  COUNT(*) as total_entries,
  SUM(hit_count) as total_hits,
  ROUND(AVG(hit_count), 2) as avg_hits_per_entry,
  ROUND(100.0 * COUNT(*) FILTER (WHERE hit_count > 1) / COUNT(*), 1) as reuse_pct
FROM np_geoc_cache
WHERE source_account_id = 'primary'
GROUP BY provider;

-- Find most queried locations
SELECT
  query_text,
  provider,
  hit_count,
  formatted_address
FROM np_geoc_cache
WHERE source_account_id = 'primary'
ORDER BY hit_count DESC
LIMIT 20;

-- Daily geocoding volume
SELECT * FROM np_geoc_volume_daily
WHERE source_account_id = 'primary'
ORDER BY day DESC
LIMIT 30;
```

---

## Troubleshooting

### Provider not responding

**Issue:** Geocoding requests fail or timeout.

**Solution:**
1. Verify provider credentials:
   ```bash
   echo $GEOCODING_GOOGLE_API_KEY
   echo $GEOCODING_MAPBOX_ACCESS_TOKEN
   ```
2. Test provider directly:
   ```bash
   # Google Maps
   curl "https://maps.googleapis.com/maps/api/geocode/json?address=1600+Pennsylvania+Avenue+NW&key=$GEOCODING_GOOGLE_API_KEY"

   # Nominatim
   curl "https://nominatim.openstreetmap.org/search?q=Washington+DC&format=json&email=$GEOCODING_NOMINATIM_EMAIL"
   ```
3. Check rate limits:
   ```bash
   echo $GEOCODING_RATE_LIMIT_PROVIDER
   ```
4. Switch to fallback provider:
   ```bash
   export GEOCODING_PROVIDERS=nominatim  # Free, no API key required
   ```

### Cache not working

**Issue:** All requests hitting provider API.

**Solution:**
1. Verify cache is enabled:
   ```bash
   echo $GEOCODING_CACHE_ENABLED  # Should be "true"
   ```
2. Check cache stats:
   ```bash
   curl http://localhost:3203/api/cache/stats
   ```
3. Verify database connection:
   ```bash
   psql $DATABASE_URL -c "SELECT COUNT(*) FROM np_geoc_cache;"
   ```
4. Check query normalization:
   ```sql
   -- Queries are normalized (lowercase, trimmed)
   -- "123 Main St" and "123 main st" should have same query_hash
   SELECT query_text, query_hash, hit_count
   FROM np_geoc_cache
   WHERE query_text ILIKE '%main%'
   LIMIT 10;
   ```

### Geofences not triggering

**Issue:** Evaluation returns wrong results or doesn't fire events.

**Solution:**
1. Verify geofence is active:
   ```sql
   SELECT id, name, active, center_lat, center_lng, radius_meters
   FROM np_geoc_geofences
   WHERE deleted_at IS NULL;
   ```
2. Check distance calculation:
   ```bash
   # Test evaluation endpoint
   curl -X POST http://localhost:3203/api/geofences/evaluate \
     -H "Content-Type: application/json" \
     -d '{
       "lat": 37.7750,
       "lng": -122.4195,
       "entity_id": "test-user"
     }'
   ```
3. Review tolerance setting:
   ```bash
   echo $GEOCODING_GEOFENCE_CHECK_TOLERANCE_METERS  # Default: 50
   ```
4. Verify notify URL is reachable:
   ```bash
   curl -X POST $GEOCODING_NOTIFY_URL \
     -H "Content-Type: application/json" \
     -d '{"test": true}'
   ```

### High API costs

**Issue:** Provider billing higher than expected.

**Solution:**
1. Check cache hit rate:
   ```bash
   curl http://localhost:3203/api/cache/stats | jq '.reuse_percentage'
   # Should be > 60% for production usage
   ```
2. Increase cache TTL:
   ```bash
   export GEOCODING_CACHE_TTL_DAYS=730  # 2 years instead of 1
   ```
3. Normalize queries before geocoding:
   ```javascript
   // Trim and lowercase before calling API
   const normalized = address.trim().toLowerCase();
   ```
4. Use Nominatim for non-critical queries:
   ```bash
   export GEOCODING_PROVIDERS=nominatim,google
   # Nominatim is free but has rate limits
   ```
5. Review query volume:
   ```sql
   SELECT
     DATE(created_at) as day,
     provider,
     COUNT(*) FILTER (WHERE hit_count = 1) as new_queries,
     COUNT(*) FILTER (WHERE hit_count > 1) as cached_queries
   FROM np_geoc_cache
   WHERE created_at > NOW() - INTERVAL '30 days'
   GROUP BY DATE(created_at), provider
   ORDER BY day DESC;
   ```

### Batch operations failing

**Issue:** Batch endpoint returns errors or timeouts.

**Solution:**
1. Check batch size limit:
   ```bash
   echo $GEOCODING_MAX_BATCH_SIZE  # Default: 100
   ```
2. Reduce batch size if needed:
   ```bash
   export GEOCODING_MAX_BATCH_SIZE=50
   ```
3. Split large batches into chunks:
   ```javascript
   const chunkSize = 50;
   for (let i = 0; i < addresses.length; i += chunkSize) {
     const chunk = addresses.slice(i, i + chunkSize);
     await geocodeBatch(chunk);
   }
   ```
4. Monitor provider rate limits:
   ```bash
   # Google Maps has daily and per-second limits
   # Nominatim has 1 req/sec limit for free tier
   echo $GEOCODING_RATE_LIMIT_PROVIDER
   ```

### Places search returning no results

**Issue:** Place search queries return empty arrays.

**Solution:**
1. Verify places are in database:
   ```sql
   SELECT COUNT(*) FROM np_geoc_places WHERE source_account_id = 'primary';
   ```
2. Places are only stored after successful searches:
   ```bash
   # First search will populate database
   curl -X POST http://localhost:3203/api/search \
     -H "Content-Type: application/json" \
     -d '{"query": "pizza", "lat": 37.7749, "lng": -122.4194, "radius": 5000}'
   ```
3. Check provider has place search enabled:
   ```bash
   # Nominatim has limited place search
   # Google Maps and Mapbox have full place APIs
   echo $GEOCODING_PROVIDERS
   ```
4. Expand search radius:
   ```json
   {
     "query": "rare specialty",
     "lat": 37.7749,
     "lng": -122.4194,
     "radius": 50000  // 50km instead of 5km
   }
   ```

---

## Support

For issues, questions, or contributions:
- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- nself CLI: https://github.com/acamarata/nself
