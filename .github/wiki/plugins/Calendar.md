# Calendar Plugin

Complete calendar and event management with recurring events (RRULE), iCal feed generation, RSVP tracking, attendee check-in, availability checking, and multi-account support.

| Property | Value |
|----------|-------|
| **Port** | `3505` |
| **Category** | `content` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run calendar init
nself plugin run calendar server
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CALENDAR_PLUGIN_PORT` | `3505` | Server port |
| `CALENDAR_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `CALENDAR_DEFAULT_TIMEZONE` | `UTC` | Default timezone for events |
| `CALENDAR_MAX_RECURRENCE_EXPANSIONS` | `365` | Maximum occurrences when expanding recurring events |
| `CALENDAR_ICAL_TOKEN_SECRET` | - | Secret for signing iCal feed tokens |
| `CALENDAR_API_KEY` | - | API key for authentication |
| `CALENDAR_RATE_LIMIT_MAX` | `200` | Max requests per window |
| `CALENDAR_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (ms) |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (6 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`, `-h`/`--host`) |
| `status` | Show calendar/event/attendee counts |
| `events` | List events (`-c`/`--calendar`, `-t`/`--type`, `-l`/`--limit`, `--upcoming`, `--today`) |
| `calendars` | List calendars (`-o`/`--owner`) |
| `reminders` | List pending reminders |
| `ical` | Generate iCal feed (`-c`/`--calendar`) |
| `stats` | Show detailed statistics |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |
| `GET` | `/live` | Liveness with memory/uptime |
| `GET` | `/v1/status` | Plugin status with counts |

### Calendars

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/calendars` | Create calendar (body: `name`, `description?`, `color?`, `timezone?`, `owner_id?`, `owner_type?`, `visibility?`, `metadata?`) |
| `GET` | `/v1/calendars` | List calendars (query: `owner_id?`, `visibility?`, `limit?`, `offset?`) |
| `GET` | `/v1/calendars/:id` | Get calendar details |
| `PUT` | `/v1/calendars/:id` | Update calendar |
| `DELETE` | `/v1/calendars/:id` | Delete calendar (and all events) |

### Events

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/events` | Create event (body: `calendar_id`, `title`, `description?`, `event_type?`, `start_time`, `end_time?`, `all_day?`, `location?`, `recurrence_rule?`, `reminder_minutes?`, `visibility?`, `metadata?`) |
| `GET` | `/v1/events` | List events (query: `calendar_id?`, `type?`, `limit?`, `offset?`) |
| `GET` | `/v1/events/:id` | Get event details |
| `PUT` | `/v1/events/:id` | Update event |
| `DELETE` | `/v1/events/:id` | Delete event (query: `scope?` -- `this`, `this_and_future`, `all` for recurring) |
| `GET` | `/v1/events/upcoming` | Get upcoming events (query: `days?`, `limit?`) |
| `GET` | `/v1/events/today` | Get today's events |
| `GET` | `/v1/events/range` | Get events in range (query: `start`, `end`) -- expands recurring events |
| `POST` | `/v1/events/:id/duplicate` | Duplicate an event (body: `new_start_time?`, `new_title?`) |

### Attendees

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/events/:id/attendees` | Add attendee (body: `user_id`, `email?`, `name?`, `role?`) |
| `GET` | `/v1/events/:id/attendees` | List attendees |
| `DELETE` | `/v1/events/:id/attendees/:attendeeId` | Remove attendee |
| `POST` | `/v1/events/:id/rsvp` | RSVP to event (body: `user_id`, `status`) -- status: `accepted`, `declined`, `tentative` |
| `POST` | `/v1/events/:id/checkin` | Check in attendee (body: `user_id`) |

### Birthdays

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/birthdays` | Get upcoming birthdays (query: `days?`, `limit?`) |

### iCal Feeds

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/ical-feeds` | Create iCal feed (body: `calendar_id`, `name?`, `include_private?`) -- returns `token` |
| `GET` | `/v1/ical-feeds` | List iCal feeds |
| `DELETE` | `/v1/ical-feeds/:id` | Delete iCal feed |
| `GET` | `/v1/ical/:token` | Public iCal endpoint (returns `text/calendar`) -- no auth required |

### Availability

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/availability` | Check availability (query: `user_id`, `start`, `end`, `calendar_ids?`) -- returns free/busy intervals |

### Recurrence Rules (RRULE)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/rrule/validate` | Validate an RRULE string (body: `rrule`) |
| `POST` | `/v1/rrule/describe` | Get human-readable description of RRULE (body: `rrule`) |
| `POST` | `/v1/rrule/generate` | Generate occurrences (body: `rrule`, `start`, `count?`, `until?`) |

---

## Event Types

| Type | Description |
|------|-------------|
| `event` | Standard calendar event |
| `birthday` | Birthday (auto-recurs yearly) |
| `anniversary` | Anniversary (auto-recurs yearly) |
| `holiday` | Holiday |
| `reminder` | Simple reminder (no end time) |
| `meeting` | Meeting with attendees |
| `trip` | Multi-day trip |

---

## RSVP Status

| Status | Description |
|--------|-------------|
| `pending` | No response yet |
| `accepted` | Attendee accepted |
| `declined` | Attendee declined |
| `tentative` | Attendee tentatively accepted |

---

## Attendee Roles

| Role | Description |
|------|-------------|
| `organizer` | Event organizer |
| `required` | Required attendee |
| `optional` | Optional attendee |
| `resource` | Room or resource |

---

## Calendar Visibility

| Visibility | Description |
|------------|-------------|
| `public` | Visible to everyone |
| `shared` | Visible to shared users |
| `private` | Visible only to owner |

---

## Recurrence Rules

The plugin supports standard iCal RRULE syntax for recurring events. Use the `/v1/rrule/*` endpoints to validate, describe, and generate occurrences.

### Common RRULE Examples

| Rule | Description |
|------|-------------|
| `FREQ=DAILY` | Every day |
| `FREQ=WEEKLY;BYDAY=MO,WE,FR` | Every Monday, Wednesday, Friday |
| `FREQ=MONTHLY;BYMONTHDAY=15` | 15th of every month |
| `FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=1` | March 1st every year |
| `FREQ=WEEKLY;INTERVAL=2;BYDAY=TU` | Every other Tuesday |
| `FREQ=DAILY;COUNT=10` | Daily for 10 occurrences |
| `FREQ=MONTHLY;BYDAY=2FR` | Second Friday of every month |

### Recurring Event Deletion Scopes

When deleting a recurring event, the `scope` query parameter controls what is removed:

| Scope | Description |
|-------|-------------|
| `this` | Delete only this occurrence (creates an exception) |
| `this_and_future` | Delete this and all future occurrences |
| `all` | Delete the entire recurring series |

---

## iCal Feed Generation

iCal feeds produce standard `.ics` files compatible with Google Calendar, Apple Calendar, Outlook, and other calendar applications. The feed is accessible via a token-based URL that requires no authentication, making it suitable for subscription.

Feed URL format: `GET /v1/ical/:token`

The response uses `Content-Type: text/calendar` with `VCALENDAR` and `VEVENT` components.

---

## Location Support

Events support structured location data:

```json
{
  "location": {
    "name": "Conference Room A",
    "address": "123 Main St, New York, NY",
    "lat": 40.7128,
    "lon": -74.0060
  }
}
```

---

## Database Schema

### `calendars`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Calendar ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Calendar name |
| `description` | `TEXT` | Calendar description |
| `color` | `VARCHAR(20)` | Display color (hex) |
| `timezone` | `VARCHAR(64)` | Calendar timezone |
| `owner_id` | `VARCHAR(255)` | Owner user ID |
| `owner_type` | `VARCHAR(50)` | `user`, `group`, `system` |
| `visibility` | `VARCHAR(20)` | `public`, `shared`, `private` |
| `is_default` | `BOOLEAN` | Whether this is the default calendar |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `events`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Event ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `calendar_id` | `UUID` (FK) | References `calendars` |
| `title` | `VARCHAR(255)` | Event title |
| `description` | `TEXT` | Event description |
| `event_type` | `VARCHAR(50)` | `event`, `birthday`, `anniversary`, `holiday`, `reminder`, `meeting`, `trip` |
| `start_time` | `TIMESTAMPTZ` | Start time |
| `end_time` | `TIMESTAMPTZ` | End time |
| `all_day` | `BOOLEAN` | Whether this is an all-day event |
| `location` | `JSONB` | Location with name, address, lat/lon |
| `recurrence_rule` | `TEXT` | iCal RRULE string |
| `series_id` | `UUID` | Links recurring event instances |
| `status` | `VARCHAR(20)` | `confirmed`, `tentative`, `cancelled` |
| `visibility` | `VARCHAR(20)` | `public`, `shared`, `private` |
| `reminder_minutes` | `INTEGER[]` | Array of reminder offsets (e.g., `[15, 60]`) |
| `organizer_id` | `VARCHAR(255)` | Event organizer |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `attendees`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Attendee record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `event_id` | `UUID` (FK) | References `events` |
| `user_id` | `VARCHAR(255)` | Attendee user ID |
| `email` | `VARCHAR(255)` | Attendee email |
| `name` | `VARCHAR(255)` | Attendee display name |
| `role` | `VARCHAR(20)` | `organizer`, `required`, `optional`, `resource` |
| `rsvp_status` | `VARCHAR(20)` | `pending`, `accepted`, `declined`, `tentative` |
| `rsvp_at` | `TIMESTAMPTZ` | RSVP timestamp |
| `checked_in` | `BOOLEAN` | Whether attendee checked in |
| `checked_in_at` | `TIMESTAMPTZ` | Check-in timestamp |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `reminders`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Reminder ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `event_id` | `UUID` (FK) | References `events` |
| `user_id` | `VARCHAR(255)` | Reminder recipient |
| `channel` | `VARCHAR(20)` | `push`, `email`, `sms` |
| `remind_at` | `TIMESTAMPTZ` | When to send the reminder |
| `sent` | `BOOLEAN` | Whether reminder was sent |
| `sent_at` | `TIMESTAMPTZ` | When reminder was sent |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `ical_feeds`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Feed ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `calendar_id` | `UUID` (FK) | References `calendars` |
| `name` | `VARCHAR(255)` | Feed name |
| `token` | `VARCHAR(128)` | Unique access token |
| `include_private` | `BOOLEAN` | Whether to include private events |
| `last_accessed_at` | `TIMESTAMPTZ` | Last feed access |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `webhook_events`

Standard webhook event tracking table with `id`, `source_account_id`, `event_type`, `payload` (JSONB), `processed`, `processed_at`, `error`, `created_at`.

---

## Troubleshooting

**Recurring events not expanding** -- Use `GET /v1/events/range` with `start` and `end` parameters to expand recurring events. Plain `GET /v1/events` returns the master event only.

**iCal feed returns empty** -- Verify the token is valid and the calendar has events. Check `include_private` if events are private.

**RRULE validation fails** -- Use `POST /v1/rrule/validate` to check the rule syntax. Common issues: missing `FREQ`, invalid `BYDAY` values, or `COUNT` and `UNTIL` used together.

**Availability shows incorrect results** -- Ensure events have both `start_time` and `end_time` set. All-day events occupy the entire day.

**Attendee check-in not working** -- The attendee must exist on the event. Use `POST /v1/events/:id/attendees` to add them first, then `POST /v1/events/:id/checkin`.
