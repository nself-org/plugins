# Meetings

Calendar integration and meeting management with room booking, Google/Outlook sync, recurring meetings, and availability tracking.

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

The Meetings plugin provides comprehensive calendar and meeting management for nself applications. It supports room booking, external calendar synchronization (Google Calendar, Outlook), recurring meetings, waitlists, availability tracking, and meeting reminders.

This plugin is essential for applications requiring coordinated scheduling, resource management, and calendar integrations.

### Key Features

- **Meeting Management**: Create, update, and cancel meetings with rich details
- **Room Booking**: Reserve physical and virtual meeting rooms with capacity tracking
- **Calendar Sync**: Two-way sync with Google Calendar and Outlook
- **Recurring Meetings**: Support for daily, weekly, monthly recurring patterns
- **Availability Tracking**: Check participant availability across calendars
- **Meeting Reminders**: Automated email/notification reminders
- **Waitlist Management**: Queue participants when meetings are full
- **Calendar Sharing**: Share calendars with team members
- **Meeting Templates**: Pre-configured meeting templates for common scenarios
- **Time Zone Support**: Proper handling of multiple time zones
- **RSVP Tracking**: Track attendance responses and actual attendance
- **Multi-Account Isolation**: Full support for multi-tenant applications

### Supported Features

- **Event Types**: meetings, appointments, all-day events, recurring events
- **Recurrence**: daily, weekly, monthly, custom patterns
- **Calendar Providers**: Google Calendar, Microsoft Outlook, iCal
- **Room Types**: physical, virtual (Zoom, Teams, etc.)
- **RSVP Statuses**: accepted, declined, tentative, needs-action
- **Reminder Types**: email, notification, SMS

### Use Cases

1. **Team Scheduling**: Coordinate team meetings with availability checks
2. **Resource Booking**: Meeting room and equipment reservations
3. **Client Meetings**: Schedule and manage client appointments
4. **Event Management**: Organize company-wide events and conferences
5. **Interview Scheduling**: Coordinate interview panels and candidates

## Quick Start

```bash
# Install the plugin
nself plugin install meetings

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export MEETINGS_PLUGIN_PORT=3710

# Optional: Configure Google Calendar integration
export GOOGLE_CALENDAR_CLIENT_ID="your-client-id"
export GOOGLE_CALENDAR_CLIENT_SECRET="your-client-secret"

# Initialize database schema
nself plugin meetings init

# Start the meetings plugin server
nself plugin meetings server

# Check status
nself plugin meetings status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `MEETINGS_PLUGIN_PORT` | No | `3710` | HTTP server port |
| `GOOGLE_CALENDAR_CLIENT_ID` | No | - | Google Calendar OAuth client ID |
| `GOOGLE_CALENDAR_CLIENT_SECRET` | No | - | Google Calendar OAuth client secret |
| `GOOGLE_CALENDAR_REDIRECT_URI` | No | - | Google Calendar OAuth redirect URI |
| `OUTLOOK_CALENDAR_CLIENT_ID` | No | - | Outlook Calendar OAuth client ID |
| `OUTLOOK_CALENDAR_CLIENT_SECRET` | No | - | Outlook Calendar OAuth client secret |
| `OUTLOOK_CALENDAR_REDIRECT_URI` | No | - | Outlook Calendar OAuth redirect URI |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server Configuration
MEETINGS_PLUGIN_PORT=3710

# Google Calendar Integration
GOOGLE_CALENDAR_CLIENT_ID=your-google-client-id
GOOGLE_CALENDAR_CLIENT_SECRET=your-google-client-secret
GOOGLE_CALENDAR_REDIRECT_URI=https://yourdomain.com/oauth/google/callback

# Outlook Calendar Integration
OUTLOOK_CALENDAR_CLIENT_ID=your-outlook-client-id
OUTLOOK_CALENDAR_CLIENT_SECRET=your-outlook-client-secret
OUTLOOK_CALENDAR_REDIRECT_URI=https://yourdomain.com/oauth/outlook/callback
```

## CLI Commands

### Global Commands

#### `init`
Initialize the meetings plugin database schema.

```bash
nself plugin meetings init
```

#### `server`
Start the meetings plugin HTTP server.

```bash
nself plugin meetings server
nself plugin meetings server --port 3710
```

#### `status`
Display current meetings plugin status.

```bash
nself plugin meetings status
```

### Event Management

#### `events`
Manage meeting events.

```bash
nself plugin meetings events list
nself plugin meetings events create "Team Standup" --start "2024-02-10T10:00:00Z" --duration 30
nself plugin meetings events info EVENT_ID
nself plugin meetings events cancel EVENT_ID
```

### Room Management

#### `rooms`
Manage meeting rooms.

```bash
nself plugin meetings rooms list
nself plugin meetings rooms create "Conference Room A" --capacity 12 --floor 3
nself plugin meetings rooms book ROOM_ID --start "2024-02-10T14:00:00Z" --duration 60
```

### Calendar Management

#### `calendars`
Manage calendars.

```bash
nself plugin meetings calendars list
nself plugin meetings calendars create "Team Calendar"
nself plugin meetings calendars share CALENDAR_ID USER_ID
```

### Templates

#### `templates`
Manage meeting templates.

```bash
nself plugin meetings templates list
nself plugin meetings templates create "Daily Standup" --duration 15 --recurring daily
```

## REST API

### Event Management

#### `POST /api/meetings/events`
Create a meeting event.

**Request:**
```json
{
  "title": "Team Planning Meeting",
  "description": "Q1 planning session",
  "startTime": "2024-02-10T14:00:00Z",
  "endTime": "2024-02-10T15:00:00Z",
  "location": "Conference Room A",
  "attendees": [
    {"email": "user1@example.com", "optional": false},
    {"email": "user2@example.com", "optional": true}
  ],
  "roomId": "550e8400-e29b-41d4-a716-446655440000",
  "reminders": [
    {"type": "email", "minutesBefore": 15}
  ]
}
```

#### `GET /api/meetings/events/:eventId`
Get event details.

#### `PATCH /api/meetings/events/:eventId`
Update event.

#### `DELETE /api/meetings/events/:eventId`
Cancel event.

#### `GET /api/meetings/events`
List events with filters.

**Query Parameters:**
- `startDate` - Filter by start date
- `endDate` - Filter by end date
- `attendee` - Filter by attendee email
- `roomId` - Filter by room
- `calendarId` - Filter by calendar

### Room Management

#### `POST /api/meetings/rooms`
Create meeting room.

#### `GET /api/meetings/rooms/:roomId`
Get room details.

#### `GET /api/meetings/rooms`
List rooms.

#### `POST /api/meetings/rooms/:roomId/book`
Book a room.

#### `GET /api/meetings/rooms/:roomId/availability`
Check room availability.

### Calendar Management

#### `POST /api/meetings/calendars`
Create calendar.

#### `GET /api/meetings/calendars`
List calendars.

#### `POST /api/meetings/calendars/:calendarId/share`
Share calendar with user.

### External Calendar Sync

#### `POST /api/meetings/external/google/connect`
Connect Google Calendar.

#### `POST /api/meetings/external/outlook/connect`
Connect Outlook Calendar.

#### `POST /api/meetings/external/sync`
Trigger calendar sync.

### RSVP Management

#### `POST /api/meetings/events/:eventId/rsvp`
Update RSVP status.

**Request:**
```json
{
  "attendeeId": "550e8400-e29b-41d4-a716-446655440001",
  "status": "accepted"
}
```

### Reminders

#### `POST /api/meetings/events/:eventId/reminders`
Add reminder.

#### `GET /api/meetings/reminders/pending`
List pending reminders.

### Webhook Endpoint

#### `POST /webhook`
Receive webhook events.

## Webhook Events

### Event Webhooks

#### `event.created`
A new meeting event was created.

#### `event.updated`
A meeting event was updated.

#### `event.cancelled`
A meeting event was cancelled.

#### `event.deleted`
A meeting event was deleted.

#### `rsvp.updated`
An attendee RSVP was updated.

### Room Webhooks

#### `room.booked`
A meeting room was booked.

#### `room.released`
A meeting room booking was released.

### Sync Webhooks

#### `sync.completed`
External calendar sync completed.

#### `reminder.sent`
A meeting reminder was sent.

## Database Schema

### np_meetings_events

Meeting events.

```sql
CREATE TABLE np_meetings_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  np_calendar_id UUID NOT NULL REFERENCES np_meetings_calendars(id) ON DELETE CASCADE,
  title VARCHAR(500) NOT NULL,
  description TEXT,
  location VARCHAR(500),
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ NOT NULL,
  all_day BOOLEAN NOT NULL DEFAULT false,
  status VARCHAR(50) NOT NULL DEFAULT 'confirmed',
  visibility VARCHAR(50) NOT NULL DEFAULT 'default',
  recurrence_rule TEXT,
  recurrence_id UUID,
  organizer_id UUID NOT NULL,
  room_id UUID REFERENCES np_meetings_rooms(id) ON DELETE SET NULL,
  external_event_id VARCHAR(255),
  external_calendar_id VARCHAR(255),
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cancelled_at TIMESTAMPTZ
);

CREATE INDEX idx_meetings_events_account ON np_meetings_events(source_account_id);
CREATE INDEX idx_meetings_events_calendar ON np_meetings_events(np_calendar_id);
CREATE INDEX idx_meetings_events_start ON np_meetings_events(start_time);
CREATE INDEX idx_meetings_events_end ON np_meetings_events(end_time);
CREATE INDEX idx_meetings_events_room ON np_meetings_events(room_id);
CREATE INDEX idx_meetings_events_status ON np_meetings_events(status);
```

### np_meetings_attendees

Event attendees and RSVP tracking.

```sql
CREATE TABLE np_meetings_attendees (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_id UUID NOT NULL REFERENCES np_meetings_events(id) ON DELETE CASCADE,
  user_id UUID,
  email VARCHAR(255) NOT NULL,
  display_name VARCHAR(255),
  is_optional BOOLEAN NOT NULL DEFAULT false,
  is_organizer BOOLEAN NOT NULL DEFAULT false,
  rsvp_status VARCHAR(50) NOT NULL DEFAULT 'needs-action',
  rsvp_at TIMESTAMPTZ,
  attended BOOLEAN,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, event_id, email)
);

CREATE INDEX idx_meetings_attendees_account ON np_meetings_attendees(source_account_id);
CREATE INDEX idx_meetings_attendees_event ON np_meetings_attendees(event_id);
CREATE INDEX idx_meetings_attendees_user ON np_meetings_attendees(user_id);
CREATE INDEX idx_meetings_attendees_email ON np_meetings_attendees(email);
```

### np_meetings_rooms

Meeting rooms and resources.

```sql
CREATE TABLE np_meetings_rooms (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(200) NOT NULL,
  description TEXT,
  room_type VARCHAR(50) NOT NULL DEFAULT 'physical',
  location VARCHAR(500),
  floor INTEGER,
  building VARCHAR(100),
  capacity INTEGER NOT NULL DEFAULT 10,
  equipment TEXT[] DEFAULT '{}',
  amenities TEXT[] DEFAULT '{}',
  is_active BOOLEAN NOT NULL DEFAULT true,
  booking_url TEXT,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX idx_meetings_rooms_account ON np_meetings_rooms(source_account_id);
CREATE INDEX idx_meetings_rooms_active ON np_meetings_rooms(is_active) WHERE is_active = true;
CREATE INDEX idx_meetings_rooms_type ON np_meetings_rooms(room_type);
```

### np_meetings_calendars

User and shared calendars.

```sql
CREATE TABLE np_meetings_calendars (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id UUID NOT NULL,
  name VARCHAR(200) NOT NULL,
  description TEXT,
  color VARCHAR(20),
  timezone VARCHAR(50) DEFAULT 'UTC',
  is_primary BOOLEAN NOT NULL DEFAULT false,
  is_public BOOLEAN NOT NULL DEFAULT false,
  external_calendar_id VARCHAR(255),
  external_provider VARCHAR(50),
  sync_enabled BOOLEAN NOT NULL DEFAULT false,
  last_synced_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_meetings_calendars_account ON np_meetings_calendars(source_account_id);
CREATE INDEX idx_meetings_calendars_user ON np_meetings_calendars(user_id);
CREATE INDEX idx_meetings_calendars_primary ON np_meetings_calendars(is_primary) WHERE is_primary = true;
```

### np_meetings_calendar_shares

Calendar sharing permissions.

```sql
CREATE TABLE np_meetings_calendar_shares (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  np_calendar_id UUID NOT NULL REFERENCES np_meetings_calendars(id) ON DELETE CASCADE,
  shared_with_user_id UUID NOT NULL,
  permission_level VARCHAR(50) NOT NULL DEFAULT 'read',
  created_by UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, np_calendar_id, shared_with_user_id)
);

CREATE INDEX idx_calendar_shares_account ON np_meetings_calendar_shares(source_account_id);
CREATE INDEX idx_calendar_shares_calendar ON np_meetings_calendar_shares(np_calendar_id);
CREATE INDEX idx_calendar_shares_user ON np_meetings_calendar_shares(shared_with_user_id);
```

### np_meetings_external_calendars

External calendar connections.

```sql
CREATE TABLE np_meetings_external_calendars (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id UUID NOT NULL,
  provider VARCHAR(50) NOT NULL,
  external_calendar_id VARCHAR(255) NOT NULL,
  access_token_encrypted TEXT NOT NULL,
  refresh_token_encrypted TEXT,
  token_expires_at TIMESTAMPTZ,
  sync_enabled BOOLEAN NOT NULL DEFAULT true,
  last_synced_at TIMESTAMPTZ,
  sync_error TEXT,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, user_id, provider, external_calendar_id)
);

CREATE INDEX idx_external_calendars_account ON np_meetings_external_calendars(source_account_id);
CREATE INDEX idx_external_calendars_user ON np_meetings_external_calendars(user_id);
CREATE INDEX idx_external_calendars_provider ON np_meetings_external_calendars(provider);
```

### np_meetings_templates

Meeting templates.

```sql
CREATE TABLE np_meetings_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(200) NOT NULL,
  description TEXT,
  duration_minutes INTEGER NOT NULL DEFAULT 30,
  default_location VARCHAR(500),
  default_attendees TEXT[] DEFAULT '{}',
  recurrence_rule TEXT,
  reminder_minutes INTEGER[] DEFAULT '{15}',
  is_public BOOLEAN NOT NULL DEFAULT false,
  created_by UUID NOT NULL,
  usage_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_meetings_templates_account ON np_meetings_templates(source_account_id);
CREATE INDEX idx_meetings_templates_public ON np_meetings_templates(is_public) WHERE is_public = true;
```

### np_meetings_reminders

Meeting reminders.

```sql
CREATE TABLE np_meetings_reminders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_id UUID NOT NULL REFERENCES np_meetings_events(id) ON DELETE CASCADE,
  attendee_id UUID REFERENCES np_meetings_attendees(id) ON DELETE CASCADE,
  reminder_type VARCHAR(50) NOT NULL DEFAULT 'email',
  minutes_before INTEGER NOT NULL DEFAULT 15,
  scheduled_at TIMESTAMPTZ NOT NULL,
  sent_at TIMESTAMPTZ,
  status VARCHAR(50) NOT NULL DEFAULT 'pending',
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_meetings_reminders_account ON np_meetings_reminders(source_account_id);
CREATE INDEX idx_meetings_reminders_event ON np_meetings_reminders(event_id);
CREATE INDEX idx_meetings_reminders_scheduled ON np_meetings_reminders(scheduled_at);
CREATE INDEX idx_meetings_reminders_status ON np_meetings_reminders(status);
```

### np_meetings_waitlist

Meeting waitlist entries.

```sql
CREATE TABLE np_meetings_waitlist (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_id UUID NOT NULL REFERENCES np_meetings_events(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  email VARCHAR(255) NOT NULL,
  position INTEGER NOT NULL,
  notified_at TIMESTAMPTZ,
  invited_at TIMESTAMPTZ,
  status VARCHAR(50) NOT NULL DEFAULT 'waiting',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, event_id, user_id)
);

CREATE INDEX idx_meetings_waitlist_account ON np_meetings_waitlist(source_account_id);
CREATE INDEX idx_meetings_waitlist_event ON np_meetings_waitlist(event_id);
CREATE INDEX idx_meetings_waitlist_user ON np_meetings_waitlist(user_id);
CREATE INDEX idx_meetings_waitlist_status ON np_meetings_waitlist(status);
```

## Examples

### Example 1: Create Meeting with Room

```bash
# Create meeting and book room
curl -X POST http://localhost:3710/api/meetings/events \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Product Review",
    "startTime": "2024-02-10T14:00:00Z",
    "endTime": "2024-02-10T15:00:00Z",
    "roomId": "ROOM_ID",
    "attendees": [
      {"email": "john@example.com"},
      {"email": "jane@example.com"}
    ]
  }'
```

### Example 2: Create Recurring Meeting

```bash
# Weekly team standup
curl -X POST http://localhost:3710/api/meetings/events \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Team Standup",
    "startTime": "2024-02-12T09:00:00Z",
    "endTime": "2024-02-12T09:15:00Z",
    "recurrenceRule": "FREQ=WEEKLY;BYDAY=MO,WE,FR",
    "attendees": [
      {"email": "team@example.com"}
    ]
  }'
```

### Example 3: Check Room Availability

```bash
# Check if room is available
curl "http://localhost:3710/api/meetings/rooms/ROOM_ID/availability?start=2024-02-10T14:00:00Z&end=2024-02-10T15:00:00Z"
```

### Example 4: Connect Google Calendar

```bash
# Initiate OAuth flow
curl -X POST http://localhost:3710/api/meetings/external/google/connect \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "USER_ID",
    "redirectUri": "https://yourdomain.com/oauth/callback"
  }'
```

### Example 5: Update RSVP

```bash
# Accept meeting invitation
curl -X POST http://localhost:3710/api/meetings/events/EVENT_ID/rsvp \
  -H "Content-Type: application/json" \
  -d '{
    "attendeeId": "ATTENDEE_ID",
    "status": "accepted"
  }'
```

## Troubleshooting

### Calendar Sync Issues

**Problem:** External calendars not syncing

**Solutions:**
1. Verify OAuth tokens haven't expired
2. Check sync_enabled flag is true
3. Review sync error messages in np_meetings_external_calendars table
4. Re-authenticate with calendar provider

### Room Booking Conflicts

**Problem:** Double-booked rooms

**Solutions:**
1. Ensure proper locking during room booking
2. Check for overlapping events in np_meetings_events
3. Verify room capacity isn't exceeded
4. Review booking logic for race conditions

### Reminder Delivery

**Problem:** Reminders not sending

**Solutions:**
1. Check scheduled_at times are in the future
2. Verify email/notification service is configured
3. Review reminder status for errors
4. Check cron job or worker process is running

---

**Version:** 1.0.0
**Last Updated:** February 2024
**Support:** https://github.com/acamarata/nself-plugins/issues
