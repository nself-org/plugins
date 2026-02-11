# Calendar Plugin for nself

Complete calendar and event management system with recurring events (RFC 5545 RRULE), iCalendar export, RSVP tracking, and reminders.

## Features

- **Multiple Calendars**: Support for multiple calendars per user/group/system
- **Event Types**: Events, birthdays, anniversaries, holidays, reminders, meetings, trips
- **Recurring Events**: Full RFC 5545 RRULE support (daily, weekly, monthly, yearly patterns)
- **RSVP Tracking**: Attendee management with pending/accepted/declined/tentative status
- **Check-in Support**: Track attendee check-ins for events
- **iCalendar Export**: Standard .ics format for calendar applications
- **Reminders**: Multi-channel reminders (push, email, SMS)
- **Location Support**: Name, address, and GPS coordinates
- **Availability Checking**: Check calendar availability for scheduling
- **Multi-App Support**: Isolated data per application via `source_account_id`

## Quick Start

### Installation

```bash
cd plugins/calendar/ts
npm install
npm run build
```

### Configuration

```bash
cp .env.example .env
# Edit .env with your database credentials
```

### Initialize Database

```bash
npm run build
node dist/cli.js init
```

### Start Server

```bash
npm run dev
# Or in production:
npm start
```

Server runs on port **3505** by default.

## Database Schema

### Tables

1. **calendar_calendars** - Calendar definitions
2. **calendar_events** - Events with recurrence support
3. **calendar_attendees** - Event attendees and RSVP status
4. **calendar_reminders** - Scheduled reminders
5. **calendar_ical_feeds** - Public iCal feed tokens
6. **calendar_webhook_events** - Webhook event log

## API Endpoints

### Health & Status

- `GET /health` - Basic health check
- `GET /ready` - Readiness check (includes DB connectivity)
- `GET /live` - Liveness check with stats
- `GET /v1/status` - Detailed status and statistics

### Calendars

- `POST /v1/calendars` - Create calendar
- `GET /v1/calendars` - List calendars
- `GET /v1/calendars/:id` - Get calendar
- `PUT /v1/calendars/:id` - Update calendar
- `DELETE /v1/calendars/:id` - Delete calendar

### Events

- `POST /v1/events` - Create event
- `GET /v1/events` - List events (with filters)
- `GET /v1/events/:id` - Get event
- `PUT /v1/events/:id` - Update event
- `DELETE /v1/events/:id` - Delete event (supports recurring)
- `POST /v1/events/:id/duplicate` - Duplicate event
- `GET /v1/events/upcoming` - Get upcoming events
- `GET /v1/events/today` - Get today's events
- `GET /v1/events/range` - Get events in date range (expands recurring)

### Attendees

- `POST /v1/events/:id/attendees` - Add attendees
- `GET /v1/events/:id/attendees` - List attendees
- `DELETE /v1/events/:id/attendees/:userId` - Remove attendee
- `POST /v1/events/:id/rsvp` - RSVP to event
- `POST /v1/events/:id/checkin` - Check in attendee

### Special Events

- `GET /v1/birthdays` - Get upcoming birthdays

### iCalendar

- `POST /v1/ical-feeds` - Create iCal feed
- `GET /v1/ical-feeds` - List feeds
- `DELETE /v1/ical-feeds/:id` - Delete feed
- `GET /v1/ical/:token` - Get iCalendar file (public, no auth)

### Utilities

- `GET /v1/availability` - Check availability
- `POST /v1/rrule/validate` - Validate RRULE
- `POST /v1/rrule/describe` - Get human-readable RRULE description
- `POST /v1/rrule/generate` - Generate RRULE from pattern

## CLI Commands

```bash
# Initialize database
nself-calendar init

# Start server
nself-calendar server [-p 3505] [-h 0.0.0.0]

# View statistics
nself-calendar status
nself-calendar stats

# List events
nself-calendar events [--upcoming] [--today] [-c calendar_id] [-t type] [-l limit]

# List calendars
nself-calendar calendars [-o owner_id]

# List reminders
nself-calendar reminders

# List iCal feeds
nself-calendar ical [-c calendar_id]
```

## Recurring Events

Supports RFC 5545 RRULE format:

```
FREQ=WEEKLY;BYDAY=MO,WE,FR;UNTIL=20261231T235959Z
```

### Examples

**Daily for 30 days:**
```
FREQ=DAILY;COUNT=30
```

**Every Monday and Wednesday:**
```
FREQ=WEEKLY;BYDAY=MO,WE
```

**Monthly on the 1st and 15th:**
```
FREQ=MONTHLY;BYMONTHDAY=1,15
```

**Yearly on birthday:**
```
FREQ=YEARLY;BYMONTH=6;BYMONTHDAY=15
```

### Deleting Recurring Events

When deleting a recurring event, specify scope:

- `scope=this` - Create exception for this occurrence
- `scope=this_and_future` - End recurrence at this point
- `scope=all` - Delete entire series

## iCalendar Export

Create a feed:

```bash
curl -X POST http://localhost:3505/v1/ical-feeds \
  -H "Content-Type: application/json" \
  -d '{"calendar_id": "uuid-here"}'
```

Response includes `token`. Access feed at:

```
http://localhost:3505/v1/ical/{token}
```

Subscribe in calendar apps (Apple Calendar, Google Calendar, Outlook, etc.)

## Example Requests

### Create Calendar

```bash
curl -X POST http://localhost:3505/v1/calendars \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Work Calendar",
    "description": "My work events",
    "color": "#FF5733",
    "owner_id": "user123",
    "timezone": "America/New_York"
  }'
```

### Create Event

```bash
curl -X POST http://localhost:3505/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "calendar_id": "uuid-here",
    "title": "Team Meeting",
    "description": "Weekly team sync",
    "start_at": "2026-02-15T10:00:00Z",
    "end_at": "2026-02-15T11:00:00Z",
    "recurrence_rule": "FREQ=WEEKLY;BYDAY=MO",
    "reminder_minutes": [15, 60],
    "attendees": [
      {"email": "alice@example.com", "name": "Alice"},
      {"email": "bob@example.com", "name": "Bob"}
    ]
  }'
```

### Get Events in Range

```bash
curl "http://localhost:3505/v1/events/range?start=2026-02-01T00:00:00Z&end=2026-02-28T23:59:59Z"
```

This expands recurring events into individual occurrences.

### RSVP to Event

```bash
curl -X POST http://localhost:3505/v1/events/uuid-here/rsvp \
  -H "Content-Type: application/json" \
  -d '{
    "status": "accepted",
    "user_id": "user123"
  }'
```

## Environment Variables

### Required

- `DATABASE_URL` - PostgreSQL connection string

### Optional

- `CALENDAR_PLUGIN_PORT` - Server port (default: 3505)
- `CALENDAR_DEFAULT_TIMEZONE` - Default timezone (default: UTC)
- `CALENDAR_MAX_ATTENDEES` - Max attendees per event (default: 500)
- `CALENDAR_REMINDER_CHECK_INTERVAL_MS` - Reminder check interval (default: 60000)
- `CALENDAR_MAX_RECURRENCE_EXPAND` - Max recurring occurrences (default: 365)
- `CALENDAR_ICAL_TOKEN_LENGTH` - iCal token length (default: 64)
- `CALENDAR_API_KEY` - API key for authentication
- `CALENDAR_RATE_LIMIT_MAX` - Rate limit max requests (default: 200)
- `CALENDAR_RATE_LIMIT_WINDOW_MS` - Rate limit window (default: 60000)

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

# Development server
npm run dev
```

## License

Source-Available (see repository for details)
