# Calendar Plugin

Complete calendar and event management with recurring events, iCal export, RSVP tracking, and multi-account support.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Recurring Events](#recurring-events)
- [iCalendar Feeds](#icalendar-feeds)
- [Multi-Account Support](#multi-account-support)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Calendar plugin provides comprehensive calendar and event management for nself. It supports:

- **6 Database Tables** - Calendars, events, attendees, reminders, iCal feeds, webhooks
- **Recurring Events** - Full RRULE support for complex recurrence patterns
- **RSVP Tracking** - Attendee management with check-in capabilities
- **iCalendar Export** - Generate iCal feeds for calendar subscriptions
- **Multi-Account Support** - Isolated calendars per `source_account_id`
- **Full REST API** - Complete HTTP API for all operations
- **CLI Interface** - Manage calendars and events from the command line

### Key Features

| Feature | Description |
|---------|-------------|
| Multiple Calendars | Create unlimited calendars per account with color coding |
| Event Types | Support for events, meetings, birthdays, holidays, trips, anniversaries |
| Recurring Events | Full RFC 5545 RRULE support (daily, weekly, monthly, yearly patterns) |
| RSVP Management | Track attendee responses (accepted, declined, tentative) |
| Check-In System | Mark attendees as checked-in for physical events |
| Reminders | Schedule reminders via push, email, or SMS channels |
| iCal Feeds | Generate unique iCal URLs for calendar subscriptions |
| Availability API | Query free/busy time slots |
| Location Support | Store location names, addresses, and coordinates |
| Timezones | Full timezone support for all events |
| Visibility Control | Public, shared, private, and confidential events |

---

## Quick Start

```bash
# Install the plugin
nself plugin install calendar

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env

# Initialize database schema
nself plugin calendar init

# Start the API server
nself plugin calendar server --port 3505

# View statistics
nself plugin calendar stats
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `CALENDAR_PLUGIN_PORT` | No | `3505` | HTTP server port |
| `CALENDAR_DEFAULT_TIMEZONE` | No | `UTC` | Default timezone for events |
| `CALENDAR_MAX_ATTENDEES` | No | `500` | Maximum attendees per event |
| `CALENDAR_REMINDER_CHECK_INTERVAL_MS` | No | `60000` | Reminder check interval (ms) |
| `CALENDAR_MAX_RECURRENCE_EXPAND` | No | `365` | Max occurrences to expand for recurring events |
| `CALENDAR_ICAL_TOKEN_LENGTH` | No | `64` | Length of iCal feed tokens |
| `CALENDAR_API_KEY` | No | - | API key for authentication (optional) |
| `CALENDAR_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `CALENDAR_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL user |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Database Configuration

The plugin uses PostgreSQL with the following connection methods:

1. **DATABASE_URL** (recommended): Full connection string
2. **Individual variables**: POSTGRES_HOST, POSTGRES_PORT, etc.

---

## CLI Commands

### Initialize Schema

```bash
nself plugin calendar init
```

Initializes all database tables and indexes.

### Start Server

```bash
nself plugin calendar server [options]

Options:
  -p, --port <port>    Server port (default: 3505)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

Starts the HTTP API server.

### View Statistics

```bash
nself plugin calendar status
# or
nself plugin calendar stats
```

Displays:
- Total calendars
- Total events
- Total attendees
- Total reminders
- Total iCal feeds
- Upcoming events count
- Last event date

### List Events

```bash
nself plugin calendar events [options]

Options:
  -c, --calendar <id>  Filter by calendar ID
  -t, --type <type>    Filter by event type
  -l, --limit <limit>  Limit results (default: 20)
  --upcoming           Show only upcoming events
  --today              Show only today's events
```

### List Calendars

```bash
nself plugin calendar calendars [options]

Options:
  -o, --owner <id>     Filter by owner ID
```

### List Reminders

```bash
nself plugin calendar reminders
```

Shows all pending reminders.

### List iCal Feeds

```bash
nself plugin calendar ical [options]

Options:
  -c, --calendar <id>  Filter by calendar ID
```

---

## REST API

### Health Endpoints

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "calendar",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Database readiness check.

**Response:**
```json
{
  "ready": true,
  "plugin": "calendar",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /live
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "calendar",
  "version": "1.0.0",
  "uptime": 12345.67,
  "memory": { ... },
  "stats": {
    "calendars": 5,
    "events": 120,
    "upcomingEvents": 45
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /v1/status
Plugin status and statistics.

---

### Calendar Endpoints

#### POST /v1/calendars
Create a new calendar.

**Request Body:**
```json
{
  "name": "Work Calendar",
  "description": "My work events",
  "color": "#3B82F6",
  "owner_id": "user_123",
  "owner_type": "user",
  "is_default": false,
  "timezone": "America/New_York",
  "visibility": "private",
  "metadata": {}
}
```

**Response:** `201 Created` with calendar object.

#### GET /v1/calendars
List all calendars.

**Query Parameters:**
- `owner_id` (optional): Filter by owner ID

**Response:**
```json
{
  "calendars": [...],
  "count": 5
}
```

#### GET /v1/calendars/:id
Get a specific calendar.

#### PUT /v1/calendars/:id
Update a calendar.

#### DELETE /v1/calendars/:id
Delete a calendar (cascades to events, attendees, etc.).

---

### Event Endpoints

#### POST /v1/events
Create a new event.

**Request Body:**
```json
{
  "calendar_id": "cal_123",
  "title": "Team Meeting",
  "description": "Weekly sync",
  "event_type": "meeting",
  "start_at": "2026-02-11T14:00:00Z",
  "end_at": "2026-02-11T15:00:00Z",
  "all_day": false,
  "timezone": "America/New_York",
  "location_name": "Conference Room A",
  "location_address": "123 Main St",
  "recurrence_rule": "FREQ=WEEKLY;BYDAY=TU",
  "color": "#FF0000",
  "status": "confirmed",
  "visibility": "default",
  "reminder_minutes": [15, 60],
  "attendees": [
    {
      "user_id": "user_456",
      "email": "john@example.com",
      "name": "John Doe",
      "role": "attendee"
    }
  ],
  "metadata": {}
}
```

**Response:** `201 Created` with event object.

#### GET /v1/events
List events with filters.

**Query Parameters:**
- `calendar_id`: Filter by calendar
- `start`: ISO date for range start
- `end`: ISO date for range end
- `type`: Event type filter
- `status`: Status filter (confirmed, tentative, cancelled)
- `limit`: Max results
- `offset`: Pagination offset

#### GET /v1/events/:id
Get a specific event.

#### PUT /v1/events/:id
Update an event.

#### DELETE /v1/events/:id
Delete an event.

**Query Parameters:**
- `scope`: Deletion scope for recurring events
  - `this`: Delete only this occurrence (creates exception)
  - `this_and_future`: End recurrence at this date
  - `all`: Delete entire series (default)

#### POST /v1/events/:id/duplicate
Duplicate an event.

#### GET /v1/events/upcoming
Get upcoming events.

**Query Parameters:**
- `limit`: Max results (default: 10)

#### GET /v1/events/today
Get today's events.

#### GET /v1/events/range
Get events in a date range with recurring event expansion.

**Query Parameters (required):**
- `start`: ISO date
- `end`: ISO date
- `calendar_id` (optional): Filter by calendar

**Response:** Returns all event occurrences in the range, including expanded recurring events.

---

### Attendee Endpoints

#### POST /v1/events/:id/attendees
Add attendees to an event.

**Request Body:** Array of attendee objects.

#### GET /v1/events/:id/attendees
List event attendees.

#### DELETE /v1/events/:id/attendees/:userId
Remove an attendee.

#### POST /v1/events/:id/rsvp
RSVP to an event.

**Request Body:**
```json
{
  "user_id": "user_123",
  "status": "accepted"
}
```

Status values: `accepted`, `declined`, `tentative`

#### POST /v1/events/:id/checkin
Check in an attendee.

**Request Body:**
```json
{
  "user_id": "user_123"
}
```

---

### Birthday Endpoints

#### GET /v1/birthdays
Get upcoming birthdays.

**Query Parameters:**
- `days`: Days ahead to check (default: 30)

---

### iCal Feed Endpoints

#### POST /v1/ical-feeds
Create an iCal feed for a calendar.

**Request Body:**
```json
{
  "calendar_id": "cal_123",
  "name": "My Public Calendar"
}
```

**Response:** Includes a unique token for the feed URL.

#### GET /v1/ical-feeds
List iCal feeds.

**Query Parameters:**
- `calendar_id` (optional): Filter by calendar

#### DELETE /v1/ical-feeds/:id
Delete an iCal feed.

#### GET /v1/ical/:token
Public endpoint to download iCal feed (no authentication).

**Response:** `text/calendar` file (.ics)

---

### Availability Endpoint

#### GET /v1/availability
Check calendar availability.

**Query Parameters:**
- `calendar_id` (required): Calendar to check
- `start` (required): ISO date
- `end` (required): ISO date
- `duration`: Meeting duration in minutes (default: 60)

**Response:**
```json
{
  "calendar_id": "cal_123",
  "start": "2026-02-11T08:00:00Z",
  "end": "2026-02-11T18:00:00Z",
  "duration_minutes": 60,
  "busy_slots": [
    {
      "start": "2026-02-11T10:00:00Z",
      "end": "2026-02-11T11:00:00Z"
    }
  ],
  "busy_count": 1
}
```

---

### RRULE Helper Endpoints

#### POST /v1/rrule/validate
Validate an RRULE string.

**Request Body:**
```json
{
  "rrule": "FREQ=WEEKLY;BYDAY=MO,WE,FR"
}
```

**Response:**
```json
{
  "valid": true,
  "error": null
}
```

#### POST /v1/rrule/describe
Get human-readable description of an RRULE.

**Request Body:**
```json
{
  "rrule": "FREQ=WEEKLY;BYDAY=MO,WE,FR"
}
```

**Response:**
```json
{
  "description": "Every week on Monday, Wednesday, and Friday"
}
```

#### POST /v1/rrule/generate
Generate an RRULE from parameters.

**Request Body:**
```json
{
  "frequency": "WEEKLY",
  "interval": 2,
  "byDay": ["MO", "WE"],
  "count": 10
}
```

**Response:**
```json
{
  "rrule": "FREQ=WEEKLY;INTERVAL=2;COUNT=10;BYDAY=MO,WE",
  "description": "Every 2 weeks on Monday and Wednesday, 10 times"
}
```

---

## Database Schema

### calendar_calendars

Stores calendar definitions.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation (default: 'primary') |
| `name` | VARCHAR(255) | Calendar name |
| `description` | TEXT | Optional description |
| `color` | VARCHAR(7) | Hex color code (default: #3B82F6) |
| `owner_id` | VARCHAR(255) | Owner user/group ID |
| `owner_type` | VARCHAR(32) | Owner type (user, group, system) |
| `is_default` | BOOLEAN | Default calendar flag |
| `timezone` | VARCHAR(64) | Calendar timezone |
| `visibility` | VARCHAR(16) | public, shared, private |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update timestamp |

**Indexes:**
- `idx_calendar_calendars_source_account` on `source_account_id`
- `idx_calendar_calendars_owner` on `owner_id`

---

### calendar_events

Stores calendar events.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `calendar_id` | UUID | Foreign key to calendars |
| `title` | VARCHAR(500) | Event title |
| `description` | TEXT | Event description |
| `event_type` | VARCHAR(32) | event, meeting, birthday, etc. |
| `start_at` | TIMESTAMPTZ | Event start time |
| `end_at` | TIMESTAMPTZ | Event end time (nullable) |
| `all_day` | BOOLEAN | All-day event flag |
| `timezone` | VARCHAR(64) | Event timezone |
| `location_name` | VARCHAR(255) | Location name |
| `location_address` | TEXT | Full address |
| `location_lat` | DOUBLE PRECISION | Latitude |
| `location_lon` | DOUBLE PRECISION | Longitude |
| `recurrence_rule` | TEXT | RRULE string (RFC 5545) |
| `recurrence_end_at` | TIMESTAMPTZ | Recurrence end date |
| `series_id` | UUID | Groups recurring instances |
| `is_exception` | BOOLEAN | Exception to recurrence |
| `original_start_at` | TIMESTAMPTZ | Original start for exceptions |
| `color` | VARCHAR(7) | Event color override |
| `url` | TEXT | Related URL |
| `organizer_id` | VARCHAR(255) | Event organizer |
| `status` | VARCHAR(16) | confirmed, tentative, cancelled |
| `visibility` | VARCHAR(16) | default, public, private, confidential |
| `reminder_minutes` | INTEGER[] | Minutes before event to remind |
| `attendee_count` | INTEGER | Cached attendee count |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update timestamp |

**Indexes:**
- `idx_calendar_events_source_account` on `source_account_id`
- `idx_calendar_events_calendar` on `calendar_id`
- `idx_calendar_events_start` on `start_at`
- `idx_calendar_events_end` on `end_at`
- `idx_calendar_events_series` on `series_id`
- `idx_calendar_events_type` on `event_type`

---

### calendar_attendees

Stores event attendees and RSVP status.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `event_id` | UUID | Foreign key to events |
| `user_id` | VARCHAR(255) | User ID (nullable) |
| `email` | VARCHAR(255) | Email address (nullable) |
| `name` | VARCHAR(255) | Attendee name (nullable) |
| `rsvp_status` | VARCHAR(16) | pending, accepted, declined, tentative |
| `rsvp_at` | TIMESTAMPTZ | RSVP timestamp |
| `role` | VARCHAR(16) | organizer, attendee, optional |
| `checked_in` | BOOLEAN | Check-in status |
| `checked_in_at` | TIMESTAMPTZ | Check-in timestamp |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Constraints:**
- UNIQUE on `(event_id, user_id)`

**Indexes:**
- `idx_calendar_attendees_source_account` on `source_account_id`
- `idx_calendar_attendees_event` on `event_id`
- `idx_calendar_attendees_user` on `user_id`

---

### calendar_reminders

Stores scheduled reminders.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `event_id` | UUID | Foreign key to events |
| `user_id` | VARCHAR(255) | User to remind |
| `remind_at` | TIMESTAMPTZ | Reminder time |
| `channel` | VARCHAR(16) | push, email, sms |
| `sent` | BOOLEAN | Sent status |
| `sent_at` | TIMESTAMPTZ | Send timestamp |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_calendar_reminders_source_account` on `source_account_id`
- `idx_calendar_reminders_event` on `event_id`
- `idx_calendar_reminders_user` on `user_id`
- `idx_calendar_reminders_time` on `remind_at` WHERE NOT sent

---

### calendar_ical_feeds

Stores iCal feed configurations.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `calendar_id` | UUID | Foreign key to calendars |
| `token` | VARCHAR(128) | Unique feed token |
| `name` | VARCHAR(255) | Feed name |
| `enabled` | BOOLEAN | Enabled status |
| `last_accessed_at` | TIMESTAMPTZ | Last access timestamp |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Constraints:**
- UNIQUE on `token`

**Indexes:**
- `idx_calendar_ical_feeds_source_account` on `source_account_id`
- `idx_calendar_ical_feeds_calendar` on `calendar_id`
- `idx_calendar_ical_feeds_token` on `token`

---

### calendar_webhook_events

Stores received webhook events (if webhooks are implemented).

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (event ID) |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `event_type` | VARCHAR(128) | Event type identifier |
| `payload` | JSONB | Full event payload |
| `processed` | BOOLEAN | Processing status |
| `processed_at` | TIMESTAMPTZ | Processing timestamp |
| `error` | TEXT | Error message if failed |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_calendar_webhook_events_source_account` on `source_account_id`
- `idx_calendar_webhook_events_processed` on `processed`

---

## Recurring Events

The Calendar plugin supports RFC 5545 RRULE format for recurring events.

### Supported Frequencies

- `DAILY` - Every day or every N days
- `WEEKLY` - Every week or every N weeks
- `MONTHLY` - Every month or every N months
- `YEARLY` - Every year or every N years

### Recurrence Rules

**Daily:**
```
FREQ=DAILY                    # Every day
FREQ=DAILY;INTERVAL=2         # Every 2 days
FREQ=DAILY;COUNT=10           # 10 times
FREQ=DAILY;UNTIL=20261231     # Until Dec 31, 2026
```

**Weekly:**
```
FREQ=WEEKLY;BYDAY=MO,WE,FR    # Monday, Wednesday, Friday
FREQ=WEEKLY;INTERVAL=2;BYDAY=TU  # Every 2 weeks on Tuesday
```

**Monthly:**
```
FREQ=MONTHLY;BYMONTHDAY=15    # 15th of each month
FREQ=MONTHLY;BYDAY=2MO        # Second Monday of each month
FREQ=MONTHLY;BYDAY=-1FR       # Last Friday of each month
```

**Yearly:**
```
FREQ=YEARLY;BYMONTH=12;BYMONTHDAY=25  # Every Christmas
FREQ=YEARLY;BYMONTH=1;BYDAY=1MO       # First Monday of January
```

### Recurrence Expansion

When querying events with `GET /v1/events/range`, recurring events are automatically expanded into individual occurrences within the date range. The maximum expansion is controlled by `CALENDAR_MAX_RECURRENCE_EXPAND` (default: 365 occurrences).

---

## iCalendar Feeds

Generate public iCal feeds for calendar subscriptions.

### Creating a Feed

```bash
curl -X POST http://localhost:3505/v1/ical-feeds \
  -H "Content-Type: application/json" \
  -d '{
    "calendar_id": "cal_123",
    "name": "Public Work Calendar"
  }'
```

Response includes a unique token.

### Subscribing to Feed

Use the token in the iCal URL:

```
http://localhost:3505/v1/ical/abc123def456...
```

Add this URL to any calendar application (Google Calendar, Apple Calendar, Outlook, etc.).

### iCal Format

The plugin generates RFC 5545-compliant iCalendar files (.ics) with:
- VEVENT components for each event
- RRULE for recurring events
- VALARM for reminders
- ATTENDEE properties
- GEO coordinates for locations
- DTSTART, DTEND with timezone info

---

## Multi-Account Support

The Calendar plugin supports complete data isolation per account using `source_account_id`.

### How It Works

1. Every table has a `source_account_id` column (default: 'primary')
2. The database class has `forSourceAccount(accountId)` method
3. All queries automatically scope to the current account
4. The HTTP server extracts `source_account_id` from request headers:
   - `x-source-account-id` header
   - `x-account-id` header (fallback)
   - Defaults to 'primary' if not provided

### Using Multi-Account Mode

**Via API:**
```bash
curl -H "x-source-account-id: customer_abc" \
  http://localhost:3505/v1/calendars
```

**In Code:**
```typescript
const db = new CalendarDatabase();
const customerDb = db.forSourceAccount('customer_abc');
const calendars = await customerDb.listCalendars();
```

### Account ID Normalization

Account IDs are normalized:
- Lowercased
- Non-alphanumeric characters converted to hyphens
- Leading/trailing hyphens removed
- Empty strings default to 'primary'

---

## Examples

### Create a Calendar

```bash
curl -X POST http://localhost:3505/v1/calendars \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Work Calendar",
    "description": "My work schedule",
    "color": "#FF5722",
    "owner_id": "user_123",
    "timezone": "America/Los_Angeles",
    "visibility": "private"
  }'
```

### Create a One-Time Event

```bash
curl -X POST http://localhost:3505/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "calendar_id": "cal_abc123",
    "title": "Project Kickoff Meeting",
    "description": "Q1 planning session",
    "event_type": "meeting",
    "start_at": "2026-02-15T14:00:00Z",
    "end_at": "2026-02-15T16:00:00Z",
    "location_name": "Conference Room A",
    "location_address": "Building 1, Floor 3",
    "reminder_minutes": [15, 60, 1440],
    "attendees": [
      {
        "user_id": "user_456",
        "email": "alice@example.com",
        "name": "Alice Smith",
        "role": "attendee"
      },
      {
        "user_id": "user_789",
        "email": "bob@example.com",
        "name": "Bob Jones",
        "role": "attendee"
      }
    ]
  }'
```

### Create a Recurring Event

```bash
curl -X POST http://localhost:3505/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "calendar_id": "cal_abc123",
    "title": "Weekly Team Sync",
    "event_type": "meeting",
    "start_at": "2026-02-11T10:00:00Z",
    "end_at": "2026-02-11T11:00:00Z",
    "recurrence_rule": "FREQ=WEEKLY;BYDAY=TU;UNTIL=20261231T235959Z",
    "reminder_minutes": [15]
  }'
```

This creates a weekly meeting every Tuesday until end of year.

### Query Upcoming Events

```bash
curl "http://localhost:3505/v1/events?start=2026-02-11T00:00:00Z&end=2026-02-18T00:00:00Z"
```

### RSVP to an Event

```bash
curl -X POST http://localhost:3505/v1/events/evt_123/rsvp \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_456",
    "status": "accepted"
  }'
```

### Check In an Attendee

```bash
curl -X POST http://localhost:3505/v1/events/evt_123/checkin \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_456"
  }'
```

### Generate an iCal Feed

```bash
curl -X POST http://localhost:3505/v1/ical-feeds \
  -H "Content-Type: application/json" \
  -d '{
    "calendar_id": "cal_abc123",
    "name": "My Public Calendar"
  }'
```

Response:
```json
{
  "id": "feed_xyz",
  "calendar_id": "cal_abc123",
  "token": "a1b2c3d4e5f6...",
  "name": "My Public Calendar",
  "enabled": true,
  "created_at": "2026-02-11T10:00:00Z"
}
```

Subscribe URL: `http://localhost:3505/v1/ical/a1b2c3d4e5f6...`

### SQL Queries

**Get all events for a user's calendars:**
```sql
SELECT e.*
FROM calendar_events e
JOIN calendar_calendars c ON e.calendar_id = c.id
WHERE c.owner_id = 'user_123'
  AND c.source_account_id = 'primary'
  AND e.source_account_id = 'primary'
ORDER BY e.start_at;
```

**Find conflicting events:**
```sql
SELECT e1.id, e1.title, e1.start_at, e1.end_at
FROM calendar_events e1
JOIN calendar_events e2 ON e1.calendar_id = e2.calendar_id
WHERE e1.id != e2.id
  AND e1.source_account_id = 'primary'
  AND e2.source_account_id = 'primary'
  AND e1.start_at < e2.end_at
  AND e1.end_at > e2.start_at
  AND e1.status != 'cancelled'
  AND e2.status != 'cancelled';
```

**Get RSVP summary for an event:**
```sql
SELECT
  rsvp_status,
  COUNT(*) as count
FROM calendar_attendees
WHERE event_id = 'evt_123'
  AND source_account_id = 'primary'
GROUP BY rsvp_status;
```

**Find users with most events created:**
```sql
SELECT
  c.owner_id,
  COUNT(e.id) as event_count
FROM calendar_calendars c
JOIN calendar_events e ON c.id = e.calendar_id
WHERE c.source_account_id = 'primary'
  AND e.source_account_id = 'primary'
GROUP BY c.owner_id
ORDER BY event_count DESC
LIMIT 10;
```

---

## Troubleshooting

### Database Connection Issues

**Error:** `Connection refused`

Check that:
1. PostgreSQL is running
2. DATABASE_URL is correct
3. Credentials are valid
4. Database exists

### RRULE Validation Errors

**Error:** `Invalid recurrence rule`

- Use the `/v1/rrule/validate` endpoint to test your RRULE
- Ensure proper RFC 5545 syntax
- Use `/v1/rrule/generate` to build complex rules

### iCal Feed Not Working

**Error:** `Feed not found`

Check:
1. Feed is enabled (`enabled = true`)
2. Token is correct
3. Calendar hasn't been deleted

### Attendee Count Mismatch

If `attendee_count` doesn't match actual attendees, the database trigger may need repair. Manually update:

```sql
UPDATE calendar_events
SET attendee_count = (
  SELECT COUNT(*)
  FROM calendar_attendees
  WHERE event_id = calendar_events.id
    AND source_account_id = calendar_events.source_account_id
)
WHERE source_account_id = 'primary';
```

### Timezone Issues

- Always store times in UTC when possible
- Set `timezone` field to event's local timezone
- Use `CALENDAR_DEFAULT_TIMEZONE` for server default

### Performance Issues

For large event sets:
1. Ensure indexes are created (`nself plugin calendar init`)
2. Use date range filters on queries
3. Limit recurrence expansion with `CALENDAR_MAX_RECURRENCE_EXPAND`
4. Consider archiving old events

### Multi-Account Data Leakage

If seeing data from other accounts:
1. Always pass `x-source-account-id` header
2. Verify database queries include `source_account_id` in WHERE clause
3. Check `forSourceAccount()` is being called

---

## Support

For issues, questions, or feature requests:
- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
