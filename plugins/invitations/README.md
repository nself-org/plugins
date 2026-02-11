# Invitations Plugin

Production-ready invitation management system for nself. Create, track, and manage invitations with support for multiple channels (email, SMS, shareable links), templates, bulk sending, and comprehensive analytics.

## Features

- **Multiple Invitation Types**: App signup, family join, team join, event attendance, share access
- **Multi-Channel Delivery**: Email, SMS, or shareable links
- **Template System**: Reusable invitation templates with variable substitution
- **Bulk Sending**: Send up to 500 invitations at once
- **Invitation Tracking**: Track status from creation through acceptance/decline
- **Expiration Management**: Automatic expiration handling with configurable timeouts
- **Conversion Analytics**: Track acceptance rates and channel performance
- **Multi-App Support**: Isolate invitations per application via `source_account_id`

## Quick Start

### Installation

```bash
cd plugins/invitations/ts
npm install
npm run build
```

### Configuration

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

Required environment variables:
- `DATABASE_URL` or `POSTGRES_*` - PostgreSQL connection

Optional configuration:
- `INVITATIONS_PLUGIN_PORT` (default: 3402)
- `INVITATIONS_DEFAULT_EXPIRY_HOURS` (default: 168 = 7 days)
- `INVITATIONS_CODE_LENGTH` (default: 32)
- `INVITATIONS_MAX_BULK_SIZE` (default: 500)
- `INVITATIONS_ACCEPT_URL_TEMPLATE` (default: https://app.example.com/invite/{{code}})

### Initialize Database

```bash
npm run build
node dist/cli.js init
```

### Start Server

```bash
# Development
npm run dev

# Production
npm start
```

Server will be available at `http://localhost:3402`

## CLI Commands

### Server Management

```bash
# Initialize database schema
nself-invitations init

# Start HTTP server
nself-invitations server

# Show status and statistics
nself-invitations status
nself-invitations stats
```

### Invitation Management

```bash
# Create invitation
nself-invitations create \
  --inviter-id user123 \
  --email user@example.com \
  --type app_signup \
  --channel email

# List invitations
nself-invitations list --limit 50 --status pending

# Validate invitation code
nself-invitations validate ABC123XYZ
```

### Template Management

```bash
# List templates
nself-invitations templates list

# Create template
nself-invitations templates create \
  --name "Welcome Email" \
  --type app_signup \
  --channel email \
  --body "Join us at {{app_name}}!"
```

## REST API

### Health Checks

```bash
GET /health          # Basic health check
GET /ready           # Readiness check (DB connectivity)
GET /live            # Liveness with stats
GET /v1/status       # Detailed status
```

### Invitations

```bash
# Create invitation
POST /v1/invitations
{
  "type": "app_signup",
  "inviter_id": "user123",
  "invitee_email": "friend@example.com",
  "invitee_name": "John Doe",
  "channel": "email",
  "message": "Join our app!",
  "expires_in_hours": 168,
  "send_immediately": true
}

# List invitations
GET /v1/invitations?limit=100&offset=0&status=pending&type=app_signup

# Get invitation
GET /v1/invitations/:id

# Revoke invitation
DELETE /v1/invitations/:id

# Resend invitation
POST /v1/invitations/:id/resend
```

### Validation & Acceptance

```bash
# Validate code
GET /v1/validate/:code

# Accept invitation
POST /v1/accept/:code
{
  "accepted_by": "newuser456",
  "metadata": { "source": "mobile_app" }
}

# Decline invitation
POST /v1/decline/:code
```

### Bulk Operations

```bash
# Create bulk send
POST /v1/bulk
{
  "inviter_id": "admin123",
  "type": "team_join",
  "invitees": [
    { "email": "user1@example.com", "name": "Alice" },
    { "email": "user2@example.com", "name": "Bob" }
  ],
  "expires_in_hours": 72
}

# Get bulk send status
GET /v1/bulk/:id
```

### Templates

```bash
# Create template
POST /v1/templates
{
  "name": "Team Invitation",
  "type": "team_join",
  "channel": "email",
  "subject": "Join {{team_name}}",
  "body": "{{inviter_name}} has invited you to join {{team_name}}!",
  "variables": ["inviter_name", "team_name"],
  "enabled": true
}

# List templates
GET /v1/templates?type=team_join&enabled=true

# Update template
PUT /v1/templates/:id
{
  "body": "Updated template text",
  "enabled": false
}

# Delete template
DELETE /v1/templates/:id
```

### Statistics

```bash
# Get invitation statistics
GET /v1/stats
```

Returns:
```json
{
  "total": 1000,
  "pending": 50,
  "sent": 200,
  "delivered": 180,
  "accepted": 150,
  "declined": 20,
  "expired": 30,
  "revoked": 10,
  "conversionRate": 75.0,
  "byType": {
    "app_signup": 800,
    "team_join": 200
  },
  "byChannel": {
    "email": 900,
    "sms": 100
  }
}
```

## Database Schema

### inv_invitations

Primary invitations table with full lifecycle tracking.

| Column | Type | Description |
|--------|------|-------------|
| id | UUID | Primary key |
| source_account_id | VARCHAR(128) | Multi-app isolation |
| type | VARCHAR(64) | Invitation type |
| inviter_id | VARCHAR(255) | User who created invitation |
| invitee_email | VARCHAR(255) | Invitee email (nullable) |
| invitee_phone | VARCHAR(32) | Invitee phone (nullable) |
| invitee_name | VARCHAR(255) | Invitee name (nullable) |
| code | VARCHAR(64) | Unique invitation code |
| status | VARCHAR(32) | Current status |
| channel | VARCHAR(16) | Delivery channel |
| message | TEXT | Custom message |
| role | VARCHAR(64) | Role to assign |
| resource_type | VARCHAR(64) | Resource type |
| resource_id | VARCHAR(255) | Resource ID |
| expires_at | TIMESTAMPTZ | Expiration date |
| sent_at | TIMESTAMPTZ | Send timestamp |
| delivered_at | TIMESTAMPTZ | Delivery confirmation |
| accepted_at | TIMESTAMPTZ | Acceptance timestamp |
| accepted_by | VARCHAR(255) | User who accepted |
| declined_at | TIMESTAMPTZ | Decline timestamp |
| revoked_at | TIMESTAMPTZ | Revocation timestamp |
| metadata | JSONB | Custom metadata |
| created_at | TIMESTAMPTZ | Creation timestamp |
| updated_at | TIMESTAMPTZ | Last update timestamp |

### inv_templates

Reusable invitation templates.

| Column | Type | Description |
|--------|------|-------------|
| id | UUID | Primary key |
| source_account_id | VARCHAR(128) | Multi-app isolation |
| name | VARCHAR(255) | Template name (unique per account) |
| type | VARCHAR(64) | Invitation type |
| channel | VARCHAR(16) | Delivery channel |
| subject | VARCHAR(500) | Email subject (nullable) |
| body | TEXT | Template body |
| variables | TEXT[] | Variable placeholders |
| enabled | BOOLEAN | Template enabled |
| created_at | TIMESTAMPTZ | Creation timestamp |
| updated_at | TIMESTAMPTZ | Last update timestamp |

### inv_bulk_sends

Bulk invitation operations.

| Column | Type | Description |
|--------|------|-------------|
| id | UUID | Primary key |
| source_account_id | VARCHAR(128) | Multi-app isolation |
| inviter_id | VARCHAR(255) | User who created bulk send |
| template_id | UUID | Template reference |
| type | VARCHAR(64) | Invitation type |
| total_count | INTEGER | Total invitations |
| sent_count | INTEGER | Successfully sent |
| failed_count | INTEGER | Failed to send |
| status | VARCHAR(32) | Bulk send status |
| invitees | JSONB | List of invitees |
| metadata | JSONB | Custom metadata |
| started_at | TIMESTAMPTZ | Start timestamp |
| completed_at | TIMESTAMPTZ | Completion timestamp |
| created_at | TIMESTAMPTZ | Creation timestamp |

### inv_webhook_events

Webhook event log for external integrations.

| Column | Type | Description |
|--------|------|-------------|
| id | VARCHAR(255) | Event ID (primary key) |
| source_account_id | VARCHAR(128) | Multi-app isolation |
| event_type | VARCHAR(128) | Event type |
| payload | JSONB | Event payload |
| processed | BOOLEAN | Processing status |
| processed_at | TIMESTAMPTZ | Processing timestamp |
| error | TEXT | Error message (if failed) |
| created_at | TIMESTAMPTZ | Event timestamp |

## Invitation Types

- `app_signup` - New user signup invitation
- `family_join` - Family account invitation
- `team_join` - Team/workspace invitation
- `event_attend` - Event attendance invitation
- `share_access` - Resource sharing invitation

## Invitation Statuses

- `pending` - Created but not sent
- `sent` - Sent to recipient
- `delivered` - Confirmed delivery
- `accepted` - Invitation accepted
- `declined` - Invitation declined
- `expired` - Invitation expired
- `revoked` - Invitation revoked

## Channels

- `email` - Email delivery
- `sms` - SMS delivery
- `link` - Shareable link (no direct delivery)

## Multi-App Support

The plugin supports multi-app isolation via `source_account_id`. All operations are automatically scoped to the account context from the request headers:

```bash
# Set account context via header
curl -H "X-Source-Account-ID: myapp" http://localhost:3402/v1/invitations
```

## Security

### API Key Authentication

Enable API key authentication by setting `INVITATIONS_API_KEY`:

```bash
INVITATIONS_API_KEY=your-secret-key
```

All requests (except health checks) must include:

```bash
curl -H "Authorization: Bearer your-secret-key" http://localhost:3402/v1/invitations
```

### Rate Limiting

Configure rate limiting:

```bash
INVITATIONS_RATE_LIMIT_MAX=100           # Max requests
INVITATIONS_RATE_LIMIT_WINDOW_MS=60000   # Time window (60 seconds)
```

## Development

```bash
# Install dependencies
npm install

# Type checking
npm run typecheck

# Watch mode
npm run watch

# Development server with auto-reload
npm run dev

# Clean build artifacts
npm run clean
```

## Production Deployment

1. Build the plugin:
```bash
npm run build
```

2. Set production environment variables

3. Initialize database:
```bash
node dist/cli.js init
```

4. Start server:
```bash
NODE_ENV=production node dist/server.js
```

Or use the CLI:
```bash
NODE_ENV=production node dist/cli.js server
```

## License

Source-Available License

## Support

For issues and feature requests, visit the [nself-plugins repository](https://github.com/acamarata/nself-plugins).
