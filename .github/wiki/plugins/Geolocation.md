# Geolocation

Real-time location sharing, history tracking, geofencing, and proximity queries

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

The Geolocation plugin provides comprehensive location tracking and geofencing capabilities for the nself platform. It enables real-time location sharing, historical location tracking, geofence management with enter/exit notifications, proximity-based queries, and emergency SOS triggers.

This plugin is ideal for applications that need to track user locations, implement geofencing for attendance or delivery tracking, build proximity-based features, or provide safety monitoring with emergency alerts.

### Key Features

- **Real-Time Location Tracking** - Update and retrieve user locations with sub-second latency
- **Location History** - Store and query historical location data with configurable retention periods
- **Geofencing** - Create circular or polygon geofences with enter/exit event triggers
- **Proximity Queries** - Find nearby users within a specified radius using efficient spatial queries
- **Activity Recognition** - Track activity types (walking, driving, cycling, stationary)
- **Battery Monitoring** - Monitor device battery levels with low-battery threshold alerts
- **Address Resolution** - Optional reverse geocoding to convert coordinates to human-readable addresses
- **PostGIS Support** - Use PostgreSQL PostGIS extension for advanced spatial queries
- **Multi-App Support** - Isolate location data by application ID for multi-tenant architectures
- **Webhook Events** - Emit events for location updates, geofence triggers, and emergency alerts

## Quick Start

```bash
# Install the plugin
nself plugin install geolocation

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export GEO_PLUGIN_PORT=3026

# Initialize the database schema
nself plugin geolocation init

# Start the server
nself plugin geolocation server
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
| `GEO_PLUGIN_PORT` | No | `3026` | HTTP server port |
| `GEO_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `GEO_LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `GEO_APP_IDS` | No | `primary` | Comma-separated list of application IDs for multi-app support |
| `GEO_POSTGIS_ENABLED` | No | `true` | Enable PostGIS for advanced spatial queries |
| `GEO_HISTORY_RETENTION_DAYS` | No | `365` | Number of days to retain location history |
| `GEO_BATCH_MAX_POINTS` | No | `1000` | Maximum number of location points per batch upload |
| `GEO_MIN_UPDATE_INTERVAL_SECONDS` | No | `30` | Minimum seconds between location updates per user |
| `GEO_GEOFENCE_CHECK_ON_UPDATE` | No | `true` | Automatically check geofences when location is updated |
| `GEO_REVERSE_GEOCODE_ENABLED` | No | `false` | Enable reverse geocoding for addresses |
| `GEO_REVERSE_GEOCODE_PROVIDER` | No | `""` | Reverse geocoding provider (google, mapbox, nominatim) |
| `GEO_REVERSE_GEOCODE_API_KEY` | No | `""` | API key for reverse geocoding provider |
| `GEO_LOW_BATTERY_THRESHOLD` | No | `15` | Battery level percentage to trigger low-battery alerts |

### Example .env

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server Configuration
GEO_PLUGIN_PORT=3026
GEO_PLUGIN_HOST=0.0.0.0
GEO_LOG_LEVEL=info

# Multi-App Support
GEO_APP_IDS=primary,app1,app2

# PostGIS Configuration
GEO_POSTGIS_ENABLED=true

# History and Retention
GEO_HISTORY_RETENTION_DAYS=365
GEO_BATCH_MAX_POINTS=1000
GEO_MIN_UPDATE_INTERVAL_SECONDS=30

# Geofencing
GEO_GEOFENCE_CHECK_ON_UPDATE=true

# Reverse Geocoding (optional)
GEO_REVERSE_GEOCODE_ENABLED=true
GEO_REVERSE_GEOCODE_PROVIDER=google
GEO_REVERSE_GEOCODE_API_KEY=your-google-maps-api-key

# Battery Monitoring
GEO_LOW_BATTERY_THRESHOLD=15
```

## CLI Commands

### `init`

Initialize the geolocation database schema.

```bash
nself plugin geolocation init
```

### `server`

Start the geolocation HTTP server.

```bash
nself plugin geolocation server
```

### `locate`

Get current location for users.

```bash
# Get location for specific user
nself plugin geolocation locate --user user123

# Get locations for multiple users
nself plugin geolocation locate --users "user1,user2,user3"

# Multi-app support
nself plugin geolocation locate --user user123 --app-id app1
```

### `history`

View location history for a user.

```bash
# Get last 24 hours of history
nself plugin geolocation history --user user123

# Get history for specific date range
nself plugin geolocation history \
  --user user123 \
  --from "2025-02-01T00:00:00Z" \
  --to "2025-02-10T23:59:59Z"

# Limit results
nself plugin geolocation history --user user123 --limit 100
```

### `fences`

Manage geofences.

```bash
# List geofences
nself plugin geolocation fences list

# Create circular geofence
nself plugin geolocation fences create \
  --name "Home" \
  --latitude 37.7749 \
  --longitude -122.4194 \
  --radius 100 \
  --trigger both

# Create polygon geofence
nself plugin geolocation fences create \
  --name "Office Campus" \
  --polygon "[[lat1,lng1],[lat2,lng2],[lat3,lng3],[lat4,lng4]]" \
  --trigger enter

# Delete geofence
nself plugin geolocation fences delete --id fence-uuid
```

### `nearby`

Find nearby users.

```bash
# Find users within 1 km radius
nself plugin geolocation nearby \
  --latitude 37.7749 \
  --longitude -122.4194 \
  --radius 1000

# Find users near a specific user
nself plugin geolocation nearby --user user123 --radius 500
```

### `stats`

Show geolocation statistics.

```bash
nself plugin geolocation stats

# Example output:
# {
#   "totalLocations": 125000,
#   "totalUsers": 450,
#   "totalGeofences": 25,
#   "totalFenceEvents": 3200,
#   "activeUsers24h": 320,
#   "averageAccuracy": 15.5
# }
```

## REST API

### Health Check Endpoints

#### `GET /health`

Check if the server is running.

**Response:**
```json
{
  "status": "ok",
  "plugin": "geolocation",
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
    "totalLocations": 125000,
    "totalUsers": 450,
    "activeUsers24h": 320
  }
}
```

### Location Endpoints

#### `POST /api/location`

Update user location.

**Headers:**
- `X-App-Id` (optional): Application ID for multi-app support
- `X-User-Id` (required): User ID

**Request Body:**
```json
{
  "latitude": 37.7749,
  "longitude": -122.4194,
  "altitude": 50.5,
  "accuracy": 10.0,
  "speed": 5.5,
  "heading": 180.0,
  "batteryLevel": 75,
  "isCharging": false,
  "activityType": "walking",
  "deviceId": "device-123",
  "recordedAt": "2025-02-11T10:30:00Z",
  "metadata": {
    "network": "wifi",
    "provider": "gps"
  }
}
```

**Response:**
```json
{
  "id": "location-uuid",
  "userId": "user123",
  "latitude": 37.7749,
  "longitude": -122.4194,
  "accuracy": 10.0,
  "recordedAt": "2025-02-11T10:30:00Z",
  "geofenceEvents": [
    {
      "fenceId": "fence-uuid",
      "fenceName": "Home",
      "eventType": "enter",
      "triggeredAt": "2025-02-11T10:30:00Z"
    }
  ]
}
```

#### `POST /api/location/batch`

Update multiple user locations in batch.

**Request Body:**
```json
{
  "locations": [
    {
      "userId": "user1",
      "latitude": 37.7749,
      "longitude": -122.4194,
      "recordedAt": "2025-02-11T10:30:00Z"
    },
    {
      "userId": "user2",
      "latitude": 37.7750,
      "longitude": -122.4195,
      "recordedAt": "2025-02-11T10:30:05Z"
    }
  ]
}
```

**Response:**
```json
{
  "inserted": 2,
  "errors": []
}
```

#### `GET /api/location`

Get current location for users.

**Query Parameters:**
- `userIds` (optional): Comma-separated user IDs (returns all if omitted)

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "sourceAccountId": "primary",
      "userId": "user123",
      "deviceId": "device-123",
      "latitude": 37.7749,
      "longitude": -122.4194,
      "altitude": 50.5,
      "accuracy": 10.0,
      "speed": 5.5,
      "heading": 180.0,
      "batteryLevel": 75,
      "isCharging": false,
      "activityType": "walking",
      "address": "San Francisco, CA, USA",
      "recordedAt": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 1
}
```

#### `GET /api/location/history`

Get location history for a user.

**Query Parameters:**
- `userId` (required): User ID
- `from` (optional): Start timestamp (ISO 8601)
- `to` (optional): End timestamp (ISO 8601)
- `limit` (optional, default: 100): Maximum results
- `offset` (optional, default: 0): Pagination offset

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "userId": "user123",
      "latitude": 37.7749,
      "longitude": -122.4194,
      "accuracy": 10.0,
      "speed": 5.5,
      "activityType": "walking",
      "batteryLevel": 75,
      "recordedAt": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 1250
}
```

#### `GET /api/location/nearby`

Find users within a radius.

**Query Parameters:**
- `latitude` (required): Center latitude
- `longitude` (required): Center longitude
- `radius` (required): Radius in meters
- `limit` (optional, default: 50): Maximum results

**Response:**
```json
{
  "data": [
    {
      "userId": "user456",
      "latitude": 37.7750,
      "longitude": -122.4195,
      "distance": 15.5,
      "recordedAt": "2025-02-11T10:29:00Z"
    },
    {
      "userId": "user789",
      "latitude": 37.7751,
      "longitude": -122.4196,
      "distance": 25.8,
      "recordedAt": "2025-02-11T10:28:00Z"
    }
  ],
  "total": 2
}
```

### Geofence Endpoints

#### `POST /api/geofences`

Create a new geofence.

**Request Body (circular):**
```json
{
  "name": "Home",
  "description": "My home geofence",
  "fenceType": "circle",
  "latitude": 37.7749,
  "longitude": -122.4194,
  "radiusMeters": 100,
  "triggerOn": "both",
  "active": true,
  "notifyUserIds": ["user123", "user456"],
  "metadata": {
    "color": "#FF5733"
  }
}
```

**Request Body (polygon):**
```json
{
  "name": "Office Campus",
  "fenceType": "polygon",
  "polygon": [
    [37.7749, -122.4194],
    [37.7750, -122.4194],
    [37.7750, -122.4195],
    [37.7749, -122.4195]
  ],
  "triggerOn": "enter",
  "active": true
}
```

**Response:** `201 Created`
```json
{
  "id": "fence-uuid",
  "name": "Home",
  "fenceType": "circle",
  "latitude": 37.7749,
  "longitude": -122.4194,
  "radiusMeters": 100,
  "active": true,
  "createdAt": "2025-02-11T10:30:00Z"
}
```

#### `GET /api/geofences`

List geofences.

**Query Parameters:**
- `ownerId` (optional): Filter by owner ID
- `active` (optional): Filter by active status (true/false)

**Response:**
```json
{
  "data": [
    {
      "id": "fence-uuid",
      "sourceAccountId": "primary",
      "ownerId": "user123",
      "name": "Home",
      "description": "My home geofence",
      "fenceType": "circle",
      "latitude": 37.7749,
      "longitude": -122.4194,
      "radiusMeters": 100,
      "triggerOn": "both",
      "active": true,
      "createdAt": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 25
}
```

#### `GET /api/geofences/:id`

Get geofence details.

**Response:**
```json
{
  "id": "fence-uuid",
  "name": "Home",
  "fenceType": "circle",
  "latitude": 37.7749,
  "longitude": -122.4194,
  "radiusMeters": 100,
  "active": true,
  "notifyUserIds": ["user123"],
  "metadata": {}
}
```

#### `PUT /api/geofences/:id`

Update geofence.

**Request Body:**
```json
{
  "name": "Updated Name",
  "active": false,
  "radiusMeters": 150
}
```

#### `DELETE /api/geofences/:id`

Delete geofence.

**Response:** `204 No Content`

#### `GET /api/geofences/events`

Get geofence events history.

**Query Parameters:**
- `userId` (optional): Filter by user ID
- `fenceId` (optional): Filter by geofence ID
- `eventType` (optional): Filter by event type (enter/exit)
- `from` (optional): Start timestamp
- `to` (optional): End timestamp
- `limit` (optional, default: 100): Maximum results

**Response:**
```json
{
  "data": [
    {
      "id": "event-uuid",
      "fenceId": "fence-uuid",
      "fenceName": "Home",
      "userId": "user123",
      "eventType": "enter",
      "latitude": 37.7749,
      "longitude": -122.4194,
      "triggeredAt": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 3200
}
```

### Statistics Endpoint

#### `GET /api/stats`

Get geolocation statistics.

**Response:**
```json
{
  "totalLocations": 125000,
  "totalUsers": 450,
  "totalGeofences": 25,
  "totalFenceEvents": 3200,
  "activeUsers24h": 320,
  "averageAccuracy": 15.5,
  "oldestLocation": "2025-01-01T00:00:00Z",
  "newestLocation": "2025-02-11T10:30:00Z"
}
```

## Webhook Events

| Event Type | Description | Payload |
|------------|-------------|---------|
| `geo.location.updated` | User location was updated | `{ userId, latitude, longitude, accuracy, recordedAt }` |
| `geo.geofence.enter` | User entered geofence | `{ userId, fenceId, fenceName, latitude, longitude, triggeredAt }` |
| `geo.geofence.exit` | User exited geofence | `{ userId, fenceId, fenceName, latitude, longitude, triggeredAt }` |
| `geo.geofence.created` | New geofence created | `{ fenceId, name, ownerId }` |
| `geo.geofence.deleted` | Geofence deleted | `{ fenceId, name }` |
| `geo.sos.triggered` | Emergency SOS triggered | `{ userId, latitude, longitude, message, triggeredAt }` |
| `geo.battery.low` | Device battery below threshold | `{ userId, batteryLevel, latitude, longitude }` |

## Database Schema

### np_geoc_locations

Stores historical location data.

```sql
CREATE TABLE IF NOT EXISTS np_geoc_locations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  device_id VARCHAR(255),
  latitude DOUBLE PRECISION NOT NULL,
  longitude DOUBLE PRECISION NOT NULL,
  altitude DOUBLE PRECISION,
  accuracy DOUBLE PRECISION,
  speed DOUBLE PRECISION,
  heading DOUBLE PRECISION,
  battery_level INTEGER,
  is_charging BOOLEAN,
  activity_type VARCHAR(20),
  address TEXT,
  metadata JSONB DEFAULT '{}',
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_geo_locations_source_app ON np_geoc_locations(source_account_id);
CREATE INDEX IF NOT EXISTS idx_geo_locations_user ON np_geoc_locations(source_account_id, user_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_geo_locations_recorded ON np_geoc_locations(recorded_at);
```

### np_geoc_latest

Stores current location for each user (one row per user).

```sql
CREATE TABLE IF NOT EXISTS np_geoc_latest (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  device_id VARCHAR(255),
  latitude DOUBLE PRECISION NOT NULL,
  longitude DOUBLE PRECISION NOT NULL,
  altitude DOUBLE PRECISION,
  accuracy DOUBLE PRECISION,
  speed DOUBLE PRECISION,
  heading DOUBLE PRECISION,
  battery_level INTEGER,
  is_charging BOOLEAN,
  activity_type VARCHAR(20),
  address TEXT,
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_geo_latest_source_app ON np_geoc_latest(source_account_id);
```

### np_geoc_fences

Stores geofences with circular or polygon boundaries.

```sql
CREATE TABLE IF NOT EXISTS np_geoc_fences (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  owner_id VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  fence_type VARCHAR(20) DEFAULT 'circle',
  latitude DOUBLE PRECISION NOT NULL,
  longitude DOUBLE PRECISION NOT NULL,
  radius_meters DOUBLE PRECISION,
  polygon JSONB,
  address TEXT,
  trigger_on VARCHAR(20) DEFAULT 'both',
  active BOOLEAN DEFAULT true,
  schedule JSONB,
  notify_user_ids TEXT[] DEFAULT '{}',
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_geo_fences_source_app ON np_geoc_fences(source_account_id);
CREATE INDEX IF NOT EXISTS idx_geo_fences_owner ON np_geoc_fences(source_account_id, owner_id);
```

### np_geoc_fence_events

Stores geofence enter/exit events.

```sql
CREATE TABLE IF NOT EXISTS np_geoc_fence_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  fence_id UUID NOT NULL REFERENCES np_geoc_fences(id) ON DELETE CASCADE,
  user_id VARCHAR(255) NOT NULL,
  event_type VARCHAR(10) NOT NULL,
  latitude DOUBLE PRECISION NOT NULL,
  longitude DOUBLE PRECISION NOT NULL,
  triggered_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_geo_fence_events_source_app ON np_geoc_fence_events(source_account_id);
CREATE INDEX IF NOT EXISTS idx_geo_fence_events_fence ON np_geoc_fence_events(fence_id, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_geo_fence_events_user ON np_geoc_fence_events(source_account_id, user_id, triggered_at DESC);
```

### np_geoc_webhook_events

Stores webhook events for asynchronous processing.

```sql
CREATE TABLE IF NOT EXISTS np_geoc_webhook_events (
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

CREATE INDEX IF NOT EXISTS idx_geo_webhook_events_source_app ON np_geoc_webhook_events(source_account_id);
```

## Examples

### Example 1: Track User Movement

```bash
# User starts journey
curl -X POST http://localhost:3026/api/location \
  -H "Content-Type: application/json" \
  -H "X-User-Id: user123" \
  -d '{
    "latitude": 37.7749,
    "longitude": -122.4194,
    "accuracy": 10.0,
    "activityType": "walking",
    "batteryLevel": 95
  }'

# Update location as user moves
curl -X POST http://localhost:3026/api/location \
  -H "Content-Type: application/json" \
  -H "X-User-Id: user123" \
  -d '{
    "latitude": 37.7750,
    "longitude": -122.4195,
    "accuracy": 8.0,
    "speed": 1.5,
    "activityType": "walking",
    "batteryLevel": 94
  }'

# View location history
nself plugin geolocation history --user user123 --limit 10
```

### Example 2: Geofence for Home Monitoring

```sql
-- Create home geofence
INSERT INTO np_geoc_fences (
  source_account_id, owner_id, name, fence_type,
  latitude, longitude, radius_meters, trigger_on
) VALUES (
  'primary', 'user123', 'Home',
  'circle', 37.7749, -122.4194, 100, 'both'
);

-- Query geofence events for today
SELECT
  gfe.event_type,
  gfe.user_id,
  gfe.latitude,
  gfe.longitude,
  gfe.triggered_at,
  gf.name as fence_name
FROM np_geoc_fence_events gfe
JOIN np_geoc_fences gf ON gf.id = gfe.fence_id
WHERE gfe.source_account_id = 'primary'
  AND DATE(gfe.triggered_at) = CURRENT_DATE
ORDER BY gfe.triggered_at DESC;
```

### Example 3: Find Nearby Users for Meetup

```bash
# Find users within 500 meters
curl "http://localhost:3026/api/location/nearby?latitude=37.7749&longitude=-122.4194&radius=500"

# Response shows nearby users with distances
```

### Example 4: Activity Timeline Report

```sql
-- Daily activity summary
SELECT
  user_id,
  DATE(recorded_at) as date,
  COUNT(*) as location_updates,
  AVG(speed) as avg_speed,
  MODE() WITHIN GROUP (ORDER BY activity_type) as primary_activity,
  MAX(battery_level) - MIN(battery_level) as battery_consumption
FROM np_geoc_locations
WHERE source_account_id = 'primary'
  AND recorded_at >= NOW() - INTERVAL '7 days'
GROUP BY user_id, DATE(recorded_at)
ORDER BY date DESC, user_id;
```

### Example 5: Emergency SOS Implementation

```javascript
// Client-side: Trigger SOS
fetch('http://localhost:3026/api/location', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-User-Id': 'user123'
  },
  body: JSON.stringify({
    latitude: 37.7749,
    longitude: -122.4194,
    accuracy: 5.0,
    metadata: {
      sos: true,
      message: 'Emergency help needed!'
    }
  })
});

// Server-side: Monitor for SOS events
SELECT * FROM np_geoc_webhook_events
WHERE event_type = 'geo.sos.triggered'
  AND processed = false
ORDER BY created_at;
```

## Troubleshooting

### Common Issues

#### 1. PostGIS Extension Not Found

**Symptom:** Error: "PostGIS extension not available"

**Solutions:**
- Install PostGIS extension:
  ```sql
  CREATE EXTENSION IF NOT EXISTS postgis;
  ```
- Verify PostGIS is installed:
  ```sql
  SELECT PostGIS_Version();
  ```
- If not available, install PostGIS on your system and restart PostgreSQL
- Alternatively, disable PostGIS: `export GEO_POSTGIS_ENABLED=false`

#### 2. Location Updates Too Frequent

**Symptom:** Database growing rapidly with duplicate locations

**Solutions:**
- Increase minimum update interval: `export GEO_MIN_UPDATE_INTERVAL_SECONDS=60`
- Implement client-side throttling to reduce update frequency
- Clean up old history:
  ```sql
  DELETE FROM np_geoc_locations
  WHERE recorded_at < NOW() - INTERVAL '90 days';
  ```

#### 3. Geofence Not Triggering

**Symptom:** User enters/exits geofence but no events generated

**Solutions:**
- Verify geofence is active:
  ```sql
  SELECT * FROM np_geoc_fences WHERE id = 'fence-uuid';
  ```
- Check geofence trigger setting (`trigger_on` should be 'both', 'enter', or 'exit')
- Ensure `GEO_GEOFENCE_CHECK_ON_UPDATE=true`
- Verify coordinates are within geofence radius:
  ```sql
  SELECT
    ST_Distance(
      ST_MakePoint(-122.4194, 37.7749)::geography,
      ST_MakePoint(longitude, latitude)::geography
    ) as distance_meters
  FROM np_geoc_latest
  WHERE user_id = 'user123';
  ```

#### 4. Reverse Geocoding Fails

**Symptom:** Addresses not populated in location records

**Solutions:**
- Verify reverse geocoding is enabled: `echo $GEO_REVERSE_GEOCODE_ENABLED`
- Check API key is valid: `echo $GEO_REVERSE_GEOCODE_API_KEY`
- Test API key with provider directly
- Check rate limits for your geocoding provider
- Consider using local geocoding with Nominatim

#### 5. Slow Proximity Queries

**Symptom:** `/api/location/nearby` endpoint is slow

**Solutions:**
- Ensure PostGIS is enabled for spatial indexing
- Add spatial index if using PostGIS:
  ```sql
  CREATE INDEX idx_geo_latest_location
  ON np_geoc_latest USING GIST (ST_MakePoint(longitude, latitude)::geography);
  ```
- Reduce search radius to improve query performance
- Limit number of results: use smaller `limit` parameter
- Consider caching frequently-queried locations

---

**Need more help?** Check the [main documentation](https://github.com/acamarata/nself-plugins) or [open an issue](https://github.com/acamarata/nself-plugins/issues).
