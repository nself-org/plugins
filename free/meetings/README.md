# meetings

Calendar integration and meeting management with room booking, recurring meetings, and availability tracking.

## Features

### Currently Available
- Meeting event storage and management
- Attendee tracking and RSVP management
- Room booking system
- Recurring meetings support
- Availability tracking
- Meeting templates
- Reminders and notifications
- Internal calendar sharing

### Planned Features
- **Google Calendar sync** (bidirectional)
- **Outlook Calendar sync** (bidirectional)
- External calendar integration

**Note**: Calendar sync endpoints currently return HTTP 501 (Not Implemented) until OAuth providers are integrated with nself Auth service. The database schema and webhook infrastructure are ready for when external sync is enabled.

## Installation

```bash
nself plugin install meetings
```

## Configuration

### Required Environment Variables
- `DATABASE_URL` - PostgreSQL connection string

### Optional (for future calendar sync)
- `GOOGLE_CALENDAR_CLIENT_ID`
- `GOOGLE_CALENDAR_CLIENT_SECRET`
- `GOOGLE_CALENDAR_REDIRECT_URI`
- `OUTLOOK_CALENDAR_CLIENT_ID`
- `OUTLOOK_CALENDAR_CLIENT_SECRET`
- `OUTLOOK_CALENDAR_REDIRECT_URI`

See plugin.json for complete configuration options.

## Usage

### CLI Commands
```bash
nself plugin meetings init        # Initialize meetings system
nself plugin meetings server      # Start meetings server
nself plugin meetings status      # View system status
nself plugin meetings events      # Manage events
nself plugin meetings rooms       # Manage rooms
nself plugin meetings calendars   # Manage calendars
nself plugin meetings templates   # Manage meeting templates
```

### API Endpoints
See the API documentation for complete endpoint reference. Note that `/calendars/sync/*` endpoints return 501 until OAuth integration is complete.

## License

See LICENSE file in repository root.
