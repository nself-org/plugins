# Invitations Plugin

Invitation management system with email/SMS delivery, tracking, bulk sends, templates, and acceptance workflows for nself applications.

## Overview

The Invitations plugin provides a comprehensive invitation system for onboarding users, referrals, and access management. It supports customizable templates, bulk sending, expiration management, and detailed tracking of invitation lifecycle.

### Key Features

- **Invitation Management**: Create, send, and track invitations
- **Email/SMS Delivery**: Send invitations via email or SMS
- **Customizable Templates**: Create reusable invitation templates
- **Bulk Sending**: Send invitations to multiple recipients
- **Expiration Control**: Set custom expiration periods
- **Acceptance Tracking**: Track who accepts invitations
- **Unique Codes**: Generate unique invitation codes
- **Resend Support**: Resend expired or undelivered invitations
- **Analytics**: Track acceptance rates and conversion
- **Multi-App Support**: Isolated invitations per source account
- **Webhook Integration**: Notify external systems of invitation events

### Use Cases

- **User Onboarding**: Invite new users to join platform
- **Team Invitations**: Invite team members to workspaces
- **Referral Programs**: User referral incentive programs
- **Event Management**: Send event invitations
- **Beta Programs**: Invite users to beta programs
- **Access Control**: Grant access via invitation-only
- **Collaboration**: Invite collaborators to projects
- **Family Sharing**: Invite family members to subscriptions

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install invitations

# Initialize database schema
nself invitations init

# Start the server
nself invitations server
```

### Basic Usage

```bash
# Create and send invitation
curl -X POST http://localhost:3402/v1/invitations \
  -H "Content-Type: application/json" \
  -d '{
    "email": "newuser@example.com",
    "invited_by": "user123",
    "type": "user_signup",
    "expires_hours": 168,
    "metadata": {
      "role": "member",
      "team": "engineering"
    }
  }'

# Validate invitation code
curl -X POST http://localhost:3402/v1/invitations/validate \
  -H "Content-Type: application/json" \
  -d '{
    "code": "ABC123XYZ"
  }'

# Accept invitation
curl -X POST http://localhost:3402/v1/invitations/ABC123XYZ/accept \
  -H "Content-Type: application/json" \
  -d '{
    "accepted_by": "newuser@example.com"
  }'

# Check status
nself invitations status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `INVITATIONS_PLUGIN_PORT` | No | `3402` | HTTP server port |
| `INVITATIONS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `INVITATIONS_DEFAULT_EXPIRY_HOURS` | No | `168` | Default expiry (7 days) |
| `INVITATIONS_CODE_LENGTH` | No | `32` | Invitation code length |
| `INVITATIONS_MAX_BULK_SIZE` | No | `500` | Max bulk send size |
| `INVITATIONS_ACCEPT_URL_TEMPLATE` | No | See config | URL template with {{code}} |
| `INVITATIONS_API_KEY` | No | - | API key for authentication |
| `INVITATIONS_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `INVITATIONS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level |

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
INVITATIONS_PLUGIN_PORT=3402
INVITATIONS_DEFAULT_EXPIRY_HOURS=336
INVITATIONS_CODE_LENGTH=16
INVITATIONS_MAX_BULK_SIZE=1000
INVITATIONS_ACCEPT_URL_TEMPLATE=https://app.example.com/invite/{{code}}
INVITATIONS_API_KEY=your-secret-key
```

---

## CLI Commands

### `init`
Initialize database schema.

```bash
nself invitations init
```

### `server`
Start the HTTP API server.

```bash
nself invitations server [options]

Options:
  -p, --port <port>    Server port (default: 3402)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

### `create`
Create new invitation.

```bash
nself invitations create <email> [options]

Options:
  --type <type>           Invitation type
  --invited-by <userId>   Inviter user ID
  --expires <hours>       Expiration hours
  --metadata <json>       Metadata (JSON)
```

**Example:**
```bash
nself invitations create newuser@example.com \
  --type user_signup \
  --invited-by user123 \
  --expires 168 \
  --metadata '{"role":"member"}'
```

### `validate`
Validate invitation code.

```bash
nself invitations validate <code>
```

### `accept`
Accept invitation.

```bash
nself invitations accept <code> [options]

Options:
  --accepted-by <email>   Acceptor email/ID
```

### `decline`
Decline invitation.

```bash
nself invitations decline <code>
```

### `templates`
Manage invitation templates.

```bash
nself invitations templates [command]

Commands:
  list                List all templates
  create              Create template
  update <id>         Update template
  delete <id>         Delete template
```

### `bulk`
Send bulk invitations.

```bash
nself invitations bulk <filePath> [options]

Options:
  --template <id>     Template ID
  --type <type>       Invitation type
```

### `stats`
View invitation statistics.

```bash
nself invitations stats
```

**Output:**
```
Invitation Statistics
=====================
Total Invitations:     1523
Sent:                  1498
Delivered:             1456
Accepted:              892
Declined:              34
Expired:               198
Pending:               332
Acceptance Rate:       61.2%
Average Time to Accept: 2.3 days
```

---

## REST API

### Health & Status

#### `GET /health`
Basic health check.

#### `GET /v1/status`
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "invitations",
  "version": "1.0.0",
  "status": "running",
  "config": {
    "defaultExpiryHours": 168,
    "codeLength": 32,
    "maxBulkSize": 500
  },
  "stats": {
    "totalInvitations": 1523,
    "sentInvitations": 1498,
    "deliveredInvitations": 1456,
    "acceptedInvitations": 892,
    "declinedInvitations": 34,
    "expiredInvitations": 198,
    "pendingInvitations": 332,
    "acceptanceRate": 61.2,
    "avgTimeToAcceptHours": 55.2,
    "invitationsByType": {
      "user_signup": 892,
      "team_member": 345,
      "referral": 286
    }
  }
}
```

### Invitation Management

#### `POST /v1/invitations`
Create and send invitation.

**Request:**
```json
{
  "email": "newuser@example.com",
  "phone": "+1234567890",
  "invited_by": "user123",
  "type": "user_signup",
  "template_id": "template-uuid",
  "expires_hours": 168,
  "max_uses": 1,
  "metadata": {
    "role": "member",
    "team_id": "team456",
    "permissions": ["read", "write"]
  },
  "custom_message": "Join our awesome platform!",
  "send_immediately": true
}
```

**Response:**
```json
{
  "id": "invitation-uuid",
  "code": "ABC123XYZ789",
  "email": "newuser@example.com",
  "invited_by": "user123",
  "type": "user_signup",
  "status": "sent",
  "accept_url": "https://app.example.com/invite/ABC123XYZ789",
  "expires_at": "2026-02-18T10:30:00Z",
  "created_at": "2026-02-11T10:30:00Z",
  "sent_at": "2026-02-11T10:30:01Z"
}
```

#### `GET /v1/invitations`
List invitations.

**Query Parameters:**
- `status`: Filter by status (pending, sent, delivered, accepted, declined, expired, revoked)
- `type`: Filter by type
- `invited_by`: Filter by inviter
- `email`: Filter by recipient email
- `from`: Start date (ISO 8601)
- `to`: End date (ISO 8601)
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "id": "invitation-uuid",
      "code": "ABC123XYZ789",
      "email": "newuser@example.com",
      "invited_by": "user123",
      "type": "user_signup",
      "status": "sent",
      "expires_at": "2026-02-18T10:30:00Z",
      "created_at": "2026-02-11T10:30:00Z",
      "sent_at": "2026-02-11T10:30:01Z"
    }
  ],
  "total": 1523,
  "limit": 50,
  "offset": 0
}
```

#### `GET /v1/invitations/:id`
Get invitation details.

**Response:**
```json
{
  "id": "invitation-uuid",
  "code": "ABC123XYZ789",
  "email": "newuser@example.com",
  "phone": "+1234567890",
  "invited_by": "user123",
  "type": "user_signup",
  "status": "sent",
  "template_id": "template-uuid",
  "accept_url": "https://app.example.com/invite/ABC123XYZ789",
  "max_uses": 1,
  "use_count": 0,
  "metadata": {
    "role": "member",
    "team_id": "team456"
  },
  "delivery_status": {
    "email_sent": true,
    "email_delivered": true,
    "sms_sent": false
  },
  "expires_at": "2026-02-18T10:30:00Z",
  "created_at": "2026-02-11T10:30:00Z",
  "sent_at": "2026-02-11T10:30:01Z",
  "delivered_at": "2026-02-11T10:30:15Z"
}
```

#### `POST /v1/invitations/validate`
Validate invitation code.

**Request:**
```json
{
  "code": "ABC123XYZ789"
}
```

**Response:**
```json
{
  "valid": true,
  "invitation": {
    "id": "invitation-uuid",
    "email": "newuser@example.com",
    "type": "user_signup",
    "metadata": {...},
    "expires_at": "2026-02-18T10:30:00Z"
  }
}
```

#### `POST /v1/invitations/:code/accept`
Accept invitation.

**Request:**
```json
{
  "accepted_by": "newuser@example.com",
  "user_id": "newuser-uuid",
  "metadata": {
    "signup_source": "invitation"
  }
}
```

**Response:**
```json
{
  "success": true,
  "invitation": {
    "id": "invitation-uuid",
    "code": "ABC123XYZ789",
    "status": "accepted",
    "accepted_by": "newuser@example.com",
    "accepted_at": "2026-02-11T10:35:00Z"
  }
}
```

#### `POST /v1/invitations/:code/decline`
Decline invitation.

**Request:**
```json
{
  "reason": "Not interested"
}
```

**Response:**
```json
{
  "success": true,
  "invitation": {
    "id": "invitation-uuid",
    "status": "declined",
    "declined_at": "2026-02-11T10:35:00Z"
  }
}
```

#### `POST /v1/invitations/:id/resend`
Resend invitation.

**Response:**
```json
{
  "success": true,
  "sent_at": "2026-02-11T10:40:00Z"
}
```

#### `POST /v1/invitations/:id/revoke`
Revoke invitation.

**Request:**
```json
{
  "reason": "No longer needed"
}
```

**Response:**
```json
{
  "success": true,
  "invitation": {
    "id": "invitation-uuid",
    "status": "revoked",
    "revoked_at": "2026-02-11T10:40:00Z"
  }
}
```

#### `DELETE /v1/invitations/:id`
Delete invitation record.

**Response:**
```json
{
  "success": true
}
```

### Bulk Operations

#### `POST /v1/invitations/bulk`
Send bulk invitations.

**Request:**
```json
{
  "invitations": [
    {
      "email": "user1@example.com",
      "metadata": {"team": "engineering"}
    },
    {
      "email": "user2@example.com",
      "metadata": {"team": "marketing"}
    },
    {
      "email": "user3@example.com",
      "metadata": {"team": "sales"}
    }
  ],
  "invited_by": "admin123",
  "type": "team_member",
  "template_id": "template-uuid",
  "expires_hours": 168
}
```

**Response:**
```json
{
  "bulk_send_id": "bulk-uuid",
  "total": 3,
  "created": 3,
  "failed": 0,
  "invitations": [
    {
      "email": "user1@example.com",
      "code": "CODE1",
      "status": "sent"
    },
    {
      "email": "user2@example.com",
      "code": "CODE2",
      "status": "sent"
    },
    {
      "email": "user3@example.com",
      "code": "CODE3",
      "status": "sent"
    }
  ],
  "started_at": "2026-02-11T10:30:00Z",
  "completed_at": "2026-02-11T10:30:15Z"
}
```

#### `GET /v1/bulk-sends/:id`
Get bulk send status.

**Response:**
```json
{
  "bulk_send_id": "bulk-uuid",
  "status": "completed",
  "total": 3,
  "sent": 3,
  "delivered": 3,
  "failed": 0,
  "started_at": "2026-02-11T10:30:00Z",
  "completed_at": "2026-02-11T10:30:15Z"
}
```

#### `GET /v1/bulk-sends`
List bulk sends.

**Response:**
```json
{
  "data": [
    {
      "bulk_send_id": "bulk-uuid",
      "total": 3,
      "sent": 3,
      "status": "completed",
      "started_at": "2026-02-11T10:30:00Z"
    }
  ],
  "total": 45
}
```

### Templates

#### `POST /v1/templates`
Create invitation template.

**Request:**
```json
{
  "name": "Team Invitation",
  "description": "Template for inviting team members",
  "subject": "You're invited to join {{team_name}}",
  "body_html": "<h1>Welcome!</h1><p>{{inviter_name}} invited you to join {{team_name}}.</p><a href=\"{{accept_url}}\">Accept Invitation</a>",
  "body_text": "{{inviter_name}} invited you to join {{team_name}}. Click here to accept: {{accept_url}}",
  "variables": ["team_name", "inviter_name", "accept_url"]
}
```

**Response:**
```json
{
  "id": "template-uuid",
  "name": "Team Invitation",
  "description": "Template for inviting team members",
  "subject": "You're invited to join {{team_name}}",
  "body_html": "...",
  "body_text": "...",
  "variables": ["team_name", "inviter_name", "accept_url"],
  "usage_count": 0,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/templates`
List templates.

**Response:**
```json
{
  "data": [
    {
      "id": "template-uuid",
      "name": "Team Invitation",
      "description": "Template for inviting team members",
      "usage_count": 234,
      "created_at": "2026-02-10T08:00:00Z"
    }
  ],
  "total": 12
}
```

#### `GET /v1/templates/:id`
Get template details.

**Response:**
```json
{
  "id": "template-uuid",
  "name": "Team Invitation",
  "description": "Template for inviting team members",
  "subject": "You're invited to join {{team_name}}",
  "body_html": "...",
  "body_text": "...",
  "variables": ["team_name", "inviter_name", "accept_url"],
  "usage_count": 234,
  "created_at": "2026-02-10T08:00:00Z",
  "updated_at": "2026-02-11T09:00:00Z"
}
```

#### `PUT /v1/templates/:id`
Update template.

**Request:**
```json
{
  "subject": "Updated subject",
  "body_html": "Updated HTML body"
}
```

**Response:**
```json
{
  "id": "template-uuid",
  "name": "Team Invitation",
  "updated_at": "2026-02-11T10:35:00Z"
}
```

#### `DELETE /v1/templates/:id`
Delete template.

**Response:**
```json
{
  "success": true
}
```

### Statistics

#### `GET /v1/stats`
Get comprehensive statistics.

**Response:**
```json
{
  "totalInvitations": 1523,
  "sentInvitations": 1498,
  "deliveredInvitations": 1456,
  "acceptedInvitations": 892,
  "declinedInvitations": 34,
  "expiredInvitations": 198,
  "revokedInvitations": 25,
  "pendingInvitations": 332,
  "acceptanceRate": 61.2,
  "declineRate": 2.3,
  "expirationRate": 13.6,
  "avgTimeToAcceptHours": 55.2,
  "invitationsByType": {
    "user_signup": 892,
    "team_member": 345,
    "referral": 286
  },
  "invitationsByStatus": {
    "pending": 332,
    "sent": 274,
    "accepted": 892,
    "declined": 34,
    "expired": 198
  },
  "topInviters": [
    {"user_id": "user123", "count": 234},
    {"user_id": "user456", "count": 189}
  ]
}
```

#### `GET /v1/stats/user/:userId`
Get user-specific invitation statistics.

**Response:**
```json
{
  "user_id": "user123",
  "invitations_sent": 234,
  "invitations_accepted": 156,
  "invitations_declined": 12,
  "invitations_pending": 45,
  "acceptance_rate": 66.7,
  "avg_time_to_accept_hours": 48.3
}
```

---

## Webhook Events

### `invitation.created`
Triggered when invitation is created.

### `invitation.sent`
Triggered when invitation is sent.

### `invitation.delivered`
Triggered when invitation is delivered.

### `invitation.accepted`
Triggered when invitation is accepted.

### `invitation.declined`
Triggered when invitation is declined.

### `invitation.expired`
Triggered when invitation expires.

### `invitation.revoked`
Triggered when invitation is revoked.

### `bulk.started`
Triggered when bulk send starts.

### `bulk.completed`
Triggered when bulk send completes.

### `template.created`
Triggered when template is created.

### `template.updated`
Triggered when template is updated.

---

## Database Schema

### `np_invites_invitations`
```sql
CREATE TABLE np_invites_invitations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  code VARCHAR(64) NOT NULL UNIQUE,
  email VARCHAR(255),
  phone VARCHAR(32),
  invited_by VARCHAR(255) NOT NULL,
  type VARCHAR(64) NOT NULL,
  status VARCHAR(32) DEFAULT 'pending',
  template_id UUID,
  accept_url TEXT,
  max_uses INTEGER DEFAULT 1,
  use_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}',
  custom_message TEXT,
  accepted_by VARCHAR(255),
  accepted_at TIMESTAMP WITH TIME ZONE,
  declined_at TIMESTAMP WITH TIME ZONE,
  revoked_at TIMESTAMP WITH TIME ZONE,
  expires_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  sent_at TIMESTAMP WITH TIME ZONE,
  delivered_at TIMESTAMP WITH TIME ZONE,
  CHECK (status IN ('pending', 'sent', 'delivered', 'accepted', 'declined', 'expired', 'revoked'))
);
```

### `np_invites_templates`
```sql
CREATE TABLE np_invites_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  subject VARCHAR(500),
  body_html TEXT,
  body_text TEXT,
  variables TEXT[] DEFAULT '{}',
  usage_count INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### `np_invites_bulk_sends`
```sql
CREATE TABLE np_invites_bulk_sends (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  invited_by VARCHAR(255) NOT NULL,
  type VARCHAR(64),
  template_id UUID,
  status VARCHAR(32) DEFAULT 'pending',
  total INTEGER DEFAULT 0,
  sent INTEGER DEFAULT 0,
  delivered INTEGER DEFAULT 0,
  failed INTEGER DEFAULT 0,
  started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  completed_at TIMESTAMP WITH TIME ZONE,
  CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);
```

### `np_invites_webhook_events`
```sql
CREATE TABLE np_invites_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Examples

### Example 1: Simple User Invitation

```bash
# Send invitation
curl -X POST http://localhost:3402/v1/invitations \
  -H "Content-Type: application/json" \
  -d '{
    "email": "newuser@example.com",
    "invited_by": "admin",
    "type": "user_signup",
    "metadata": {"source": "admin_panel"}
  }'

# User validates code
curl -X POST http://localhost:3402/v1/invitations/validate \
  -H "Content-Type: application/json" \
  -d '{"code": "ABC123"}'

# User accepts
curl -X POST http://localhost:3402/v1/invitations/ABC123/accept \
  -H "Content-Type: application/json" \
  -d '{"accepted_by": "newuser@example.com"}'
```

### Example 2: Team Member Invitations

```bash
# Bulk invite team
curl -X POST http://localhost:3402/v1/invitations/bulk \
  -H "Content-Type: application/json" \
  -d '{
    "invitations": [
      {"email": "dev1@example.com", "metadata": {"role": "developer"}},
      {"email": "dev2@example.com", "metadata": {"role": "developer"}},
      {"email": "pm@example.com", "metadata": {"role": "manager"}}
    ],
    "invited_by": "admin",
    "type": "team_member",
    "expires_hours": 336
  }'
```

### Example 3: Custom Template

```bash
# Create template
curl -X POST http://localhost:3402/v1/templates \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Referral Invitation",
    "subject": "{{friend_name}} invited you!",
    "body_html": "<p>Your friend {{friend_name}} thinks you would love our product!</p>",
    "variables": ["friend_name", "accept_url"]
  }'

# Use template
curl -X POST http://localhost:3402/v1/invitations \
  -H "Content-Type: application/json" \
  -d '{
    "email": "friend@example.com",
    "invited_by": "user123",
    "type": "referral",
    "template_id": "template-uuid",
    "metadata": {"friend_name": "John"}
  }'
```

### Example 4: Resend Expired Invitation

```bash
# Check if expired
curl http://localhost:3402/v1/invitations/invitation-uuid

# Resend
curl -X POST http://localhost:3402/v1/invitations/invitation-uuid/resend
```

### Example 5: Track Acceptance Rate

```bash
# Get overall stats
curl http://localhost:3402/v1/stats

# Get user stats
curl http://localhost:3402/v1/stats/user/user123
```

---

## Troubleshooting

### Invitations Not Delivering

**Solution:**
- Verify email service configuration
- Check spam folders
- Review delivery logs
- Validate email addresses
- Check rate limits

### Low Acceptance Rate

**Solution:**
- Review invitation copy and messaging
- Shorten expiration periods
- Add value proposition
- Personalize invitations
- Follow up with reminders

### Expired Invitations

**Solution:**
- Increase `INVITATIONS_DEFAULT_EXPIRY_HOURS`
- Implement automatic reminders
- Allow invitation extensions
- Resend before expiration

### Code Conflicts

**Solution:**
- Increase `INVITATIONS_CODE_LENGTH`
- Use more entropy in code generation
- Check for duplicates before insert

---

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/invitations
