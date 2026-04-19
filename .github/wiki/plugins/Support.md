# Support

Helpdesk and customer support for nself-chat - ticketing, SLA, canned responses, knowledge base, analytics.

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

The Support plugin provides a complete helpdesk and customer support system for nself-chat applications. It includes ticketing, SLA management, team routing, canned responses, knowledge base articles, satisfaction surveys, and comprehensive analytics for support operations.

This plugin is essential for applications requiring structured customer support with SLA tracking, agent performance monitoring, and knowledge management.

### Key Features

- **Ticketing System**: Complete ticket lifecycle from creation to resolution with auto-numbering
- **SLA Management**: Priority-based SLA policies with first response and resolution tracking
- **Team Routing**: Support teams with auto-assignment (round robin, least busy, skill-based)
- **Agent Dashboard**: Real-time agent status, workload, and performance metrics
- **Canned Responses**: Pre-written responses with shortcuts for faster support
- **Knowledge Base**: Searchable help articles with versioning and analytics
- **Satisfaction Surveys**: CSAT and NPS tracking with automated surveys
- **Email Integration**: SMTP support for ticket creation and notifications
- **Business Hours**: SLA calculations respect configured business hours
- **Escalation**: Automatic escalation based on SLA breaches
- **Audit Trail**: Complete ticket history with field-level change tracking
- **Multi-Account Isolation**: Full support for multi-tenant applications

### Supported Features

- **Ticket Statuses**: new, open, pending, resolved, closed, reopened
- **Priority Levels**: low, medium, high, urgent with customizable SLA targets
- **Ticket Sources**: chat, email, api, web_form, phone
- **Assignment Methods**: round_robin, least_busy, skill_based, manual
- **Response Types**: Internal notes, customer-visible replies, system messages
- **Satisfaction**: 5-star CSAT ratings with comments, NPS scores

### Use Cases

1. **Customer Support**: Multi-channel support with SLA tracking
2. **IT Helpdesk**: Internal IT support with ticket prioritization
3. **Technical Support**: Product support with knowledge base integration
4. **Account Management**: Customer success team coordination
5. **Bug Tracking**: Issue tracking with team assignment

## Quick Start

```bash
# Install the plugin
nself plugin install support

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export SUPPORT_PLUGIN_PORT=3709
export SUPPORT_EMAIL="support@example.com"

# Initialize database schema
nself plugin support init

# Start the support plugin server
nself plugin support server

# Create a ticket
nself plugin support ticket:create "Website login issue" \
  --description "Cannot login to account" \
  --priority high

# Check status
nself plugin support status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `SUPPORT_PLUGIN_PORT` | No | `3709` | HTTP server port |
| `SUPPORT_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | ` ` (empty) | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `SUPPORT_EMAIL` | No | `support@example.com` | Support email address |
| `SUPPORT_EMAIL_SMTP_HOST` | No | - | SMTP server hostname |
| `SUPPORT_EMAIL_SMTP_PORT` | No | `587` | SMTP server port |
| `SUPPORT_EMAIL_FROM_NAME` | No | `Support Team` | Email sender name |
| `SUPPORT_DEFAULT_FIRST_RESPONSE_MINUTES` | No | `60` | Default first response SLA (minutes) |
| `SUPPORT_DEFAULT_RESOLUTION_MINUTES` | No | `1440` | Default resolution SLA (minutes, 24hrs) |
| `SUPPORT_BUSINESS_HOURS_START` | No | `09:00` | Business hours start time (HH:MM) |
| `SUPPORT_BUSINESS_HOURS_END` | No | `17:00` | Business hours end time (HH:MM) |
| `SUPPORT_TIMEZONE` | No | `UTC` | Timezone for business hours |
| `SUPPORT_AUTO_ASSIGNMENT` | No | `true` | Enable automatic ticket assignment |
| `SUPPORT_ASSIGNMENT_METHOD` | No | `round_robin` | Assignment method (round_robin, least_busy, skill_based) |
| `SUPPORT_MAX_TICKETS_PER_AGENT` | No | `10` | Maximum tickets per agent |
| `SUPPORT_CSAT_ENABLED` | No | `true` | Enable CSAT surveys |
| `SUPPORT_CSAT_SEND_DELAY_HOURS` | No | `24` | Hours after resolution to send CSAT |
| `SUPPORT_NPS_ENABLED` | No | `false` | Enable NPS surveys |
| `SUPPORT_NPS_SEND_INTERVAL_DAYS` | No | `90` | Days between NPS surveys |
| `SUPPORT_KB_ENABLED` | No | `true` | Enable knowledge base |
| `SUPPORT_KB_PUBLIC_ACCESS` | No | `true` | Allow public KB access |
| `SUPPORT_KB_SUGGEST_ARTICLES` | No | `true` | Suggest articles when creating tickets |
| `SUPPORT_NOTIFY_ON_NEW_TICKET` | No | `true` | Notify agents on new tickets |
| `SUPPORT_NOTIFY_ON_ASSIGNMENT` | No | `true` | Notify agent on ticket assignment |
| `SUPPORT_NOTIFY_ON_SLA_BREACH` | No | `true` | Notify on SLA breaches |
| `SUPPORT_NOTIFY_ON_CUSTOMER_REPLY` | No | `true` | Notify agent on customer replies |
| `SUPPORT_API_KEY` | No | - | API key for authenticated requests |
| `SUPPORT_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `SUPPORT_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
POSTGRES_SSL=false

# Server Configuration
SUPPORT_PLUGIN_PORT=3709
SUPPORT_PLUGIN_HOST=0.0.0.0

# Email Configuration
SUPPORT_EMAIL=support@example.com
SUPPORT_EMAIL_SMTP_HOST=smtp.example.com
SUPPORT_EMAIL_SMTP_PORT=587
SUPPORT_EMAIL_FROM_NAME=Our Support Team

# SLA Configuration
SUPPORT_DEFAULT_FIRST_RESPONSE_MINUTES=60
SUPPORT_DEFAULT_RESOLUTION_MINUTES=1440
SUPPORT_BUSINESS_HOURS_START=09:00
SUPPORT_BUSINESS_HOURS_END=17:00
SUPPORT_TIMEZONE=America/New_York

# Assignment Configuration
SUPPORT_AUTO_ASSIGNMENT=true
SUPPORT_ASSIGNMENT_METHOD=round_robin
SUPPORT_MAX_TICKETS_PER_AGENT=10

# Satisfaction Surveys
SUPPORT_CSAT_ENABLED=true
SUPPORT_CSAT_SEND_DELAY_HOURS=24
SUPPORT_NPS_ENABLED=true
SUPPORT_NPS_SEND_INTERVAL_DAYS=90

# Knowledge Base
SUPPORT_KB_ENABLED=true
SUPPORT_KB_PUBLIC_ACCESS=true
SUPPORT_KB_SUGGEST_ARTICLES=true

# Notifications
SUPPORT_NOTIFY_ON_NEW_TICKET=true
SUPPORT_NOTIFY_ON_ASSIGNMENT=true
SUPPORT_NOTIFY_ON_SLA_BREACH=true
SUPPORT_NOTIFY_ON_CUSTOMER_REPLY=true

# Security
SUPPORT_API_KEY=your-secret-api-key-here
SUPPORT_RATE_LIMIT_MAX=100
SUPPORT_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

## CLI Commands

### Global Commands

#### `init`
Initialize the support plugin database schema.

```bash
nself plugin support init
```

Creates all required tables, indexes, and constraints for tickets, teams, SLA policies, canned responses, KB articles, and audit trails.

#### `server`
Start the support plugin HTTP server.

```bash
nself plugin support server
nself plugin support server --port 3709
```

**Options:**
- `-p, --port <port>` - Server port (default: 3709)

#### `status`
Display current support plugin status and statistics.

```bash
nself plugin support status
```

Shows:
- Configuration status (CSAT, KB, auto-assignment)
- Ticket statistics (total, open, pending, resolved)
- Team and agent counts
- SLA policies, canned responses, KB articles

**Example output:**
```
Support Plugin Status
=======================
Port:               3709
CSAT Enabled:       true
KB Enabled:         true
Auto Assignment:    true
Assignment Method:  round_robin

Ticket Statistics
-----------------
Total Tickets:      1,234
Open:               45
Pending:            12
Resolved:           1,156

Resources
---------
Teams:              5
Agents:             23
SLA Policies:       4
Canned Responses:   28
KB Articles:        156 (142 published)
```

### Ticket Management

#### `ticket:create`
Create a new support ticket.

```bash
nself plugin support ticket:create <subject> [options]
nself plugin support ticket:create "Website login issue" \
  --description "User cannot login to their account" \
  --priority high \
  --team TEAM_ID \
  --category authentication
```

**Arguments:**
- `<subject>` - Ticket subject

**Options:**
- `--description <description>` - Ticket description
- `--priority <priority>` - Priority: low, medium, high, urgent (default: medium)
- `--assign <userId>` - Assign to specific agent
- `--team <teamId>` - Assign to team
- `--category <category>` - Ticket category
- `--source <source>` - Source: chat, email, api, web_form (default: chat)

**Output:**
```
✓ Ticket created: TKT-00123
  ID:                 550e8400-...
  Status:             new
  Priority:           high
  First response due: 2024-02-10T11:00:00Z
  Resolution due:     2024-02-10T18:00:00Z
```

#### `tickets:list`
List support tickets with filters.

```bash
nself plugin support tickets:list
nself plugin support tickets:list --status open --priority high
nself plugin support tickets:list --assigned USER_ID --limit 50
```

**Options:**
- `-s, --status <status>` - Filter by status
- `-p, --priority <priority>` - Filter by priority
- `--assigned <userId>` - Filter by assigned agent
- `--team <teamId>` - Filter by team
- `-l, --limit <limit>` - Result limit (default: 20)

#### `ticket:info`
Get detailed ticket information.

```bash
nself plugin support ticket:info <ticketId>
nself plugin support ticket:info TKT-00123
```

**Output:**
```
Ticket: TKT-00123
================================
Subject:          Website login issue
Status:           open
Priority:         high
Assigned to:      John Doe (Agent)
Team:             Technical Support
Created:          2024-02-10T10:00:00Z
First Response:   2024-02-10T10:15:00Z (15 min)
Resolution Due:   2024-02-10T18:00:00Z

SLA Status:       On track
Customer:         customer@example.com
Source:           chat

Messages: 3
```

#### `ticket:update`
Update ticket properties.

```bash
nself plugin support ticket:update <ticketId> [options]
nself plugin support ticket:update TKT-00123 --status resolved --priority medium
```

**Options:**
- `--status <status>` - Update status
- `--priority <priority>` - Update priority
- `--assign <userId>` - Reassign ticket
- `--team <teamId>` - Change team

#### `ticket:comment`
Add comment to ticket.

```bash
nself plugin support ticket:comment <ticketId> <message>
nself plugin support ticket:comment TKT-00123 "Issue resolved by resetting password"
```

#### `ticket:close`
Close a ticket.

```bash
nself plugin support ticket:close <ticketId>
```

### Team Management

#### `team:create`
Create a support team.

```bash
nself plugin support team:create <name> [options]
nself plugin support team:create "Technical Support" \
  --description "Technical issues and bug reports" \
  --email tech@example.com
```

**Options:**
- `--description <description>` - Team description
- `--email <email>` - Team email address

#### `teams:list`
List support teams.

```bash
nself plugin support teams:list
```

#### `team:add-member`
Add agent to team.

```bash
nself plugin support team:add-member <teamId> <userId> [options]
nself plugin support team:add-member TEAM_ID USER_ID --role agent --max-tickets 10
```

**Options:**
- `--role <role>` - Role: agent, lead, supervisor (default: agent)
- `--max-tickets <num>` - Maximum concurrent tickets (default: 10)
- `--skills <skills>` - Comma-separated skills

### SLA Management

#### `sla:create`
Create SLA policy.

```bash
nself plugin support sla:create <name> [options]
nself plugin support sla:create "Standard SLA" \
  --urgent-first 15 --urgent-res 240 \
  --high-first 60 --high-res 480
```

**Options:**
- `--urgent-first <min>` - Urgent first response time (minutes)
- `--urgent-res <min>` - Urgent resolution time (minutes)
- `--high-first <min>` - High first response time (minutes)
- `--high-res <min>` - High resolution time (minutes)
- `--business-hours` - Apply only during business hours

#### `sla:list`
List SLA policies.

```bash
nself plugin support sla:list
```

### Canned Responses

#### `canned:create`
Create canned response.

```bash
nself plugin support canned:create <title> [options]
nself plugin support canned:create "Password Reset" \
  --shortcut "/pwd" \
  --content "Follow these steps to reset your password..."
```

**Options:**
- `--shortcut <shortcut>` - Quick access shortcut (e.g., /pwd)
- `--content <content>` - Response content
- `--category <category>` - Category

#### `canned:list`
List canned responses.

```bash
nself plugin support canned:list
nself plugin support canned:list --category authentication
```

### Knowledge Base

#### `kb:create`
Create knowledge base article.

```bash
nself plugin support kb:create <title> [options]
nself plugin support kb:create "How to reset your password" \
  --content "..." \
  --category authentication \
  --public
```

**Options:**
- `--content <content>` - Article content (Markdown)
- `--category <category>` - Category
- `--tags <tags>` - Comma-separated tags
- `--public` - Make publicly accessible

#### `kb:list`
List KB articles.

```bash
nself plugin support kb:list
nself plugin support kb:list --published --category authentication
```

**Options:**
- `--published` - Show only published articles
- `--category <category>` - Filter by category

#### `kb:publish`
Publish KB article.

```bash
nself plugin support kb:publish <articleId>
```

### Analytics

#### `dashboard`
View support dashboard with key metrics.

```bash
nself plugin support dashboard
```

Shows:
- Ticket volume trends
- Average response/resolution times
- SLA compliance rate
- Agent performance
- CSAT scores

## REST API

All endpoints return JSON responses with the following structure:
```json
{
  "success": true,
  "data": { ... }
}
```

Error responses:
```json
{
  "error": "Error message"
}
```

### Authentication

If `SUPPORT_API_KEY` is set, include the API key in the `Authorization` header:

```
Authorization: Bearer your-api-key-here
```

### Health Endpoints

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "support",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `GET /ready`
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "support",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `GET /live`
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "support",
  "version": "1.0.0",
  "uptime": 86400,
  "memory": {
    "rss": 134217728,
    "heapTotal": 67108864,
    "heapUsed": 45088768
  },
  "stats": {
    "totalTickets": 1234,
    "openTickets": 45,
    "pendingTickets": 12,
    "resolvedTickets": 1156,
    "totalTeams": 5,
    "totalAgents": 23,
    "totalSlaPolicies": 4,
    "totalCannedResponses": 28,
    "totalKbArticles": 156,
    "publishedKbArticles": 142
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Status Endpoint

#### `GET /v1/status`
Get plugin status and configuration.

**Response:**
```json
{
  "plugin": "support",
  "version": "1.0.0",
  "status": "running",
  "csatEnabled": true,
  "kbEnabled": true,
  "autoAssignment": true,
  "stats": {
    "totalTickets": 1234,
    "openTickets": 45
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Ticket Management

#### `POST /api/support/tickets`
Create a new ticket.

**Request:**
```json
{
  "customerId": "550e8400-e29b-41d4-a716-446655440000",
  "customerName": "John Doe",
  "customerEmail": "customer@example.com",
  "subject": "Website login issue",
  "description": "Cannot login to my account",
  "priority": "high",
  "source": "chat",
  "category": "authentication",
  "tags": ["login", "password"],
  "teamId": "550e8400-e29b-41d4-a716-446655440001",
  "assignedTo": "550e8400-e29b-41d4-a716-446655440002"
}
```

**Response:**
```json
{
  "success": true,
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticketNumber": "TKT-00123",
    "status": "new",
    "priority": "high",
    "firstResponseDueAt": "2024-02-10T11:00:00Z",
    "resolutionDueAt": "2024-02-10T18:00:00Z"
  }
}
```

#### `GET /api/support/tickets/:ticketId`
Get ticket details.

**Response:**
```json
{
  "success": true,
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123",
    "subject": "Website login issue",
    "status": "open",
    "priority": "high",
    "assigned_to": "550e8400-e29b-41d4-a716-446655440002",
    "first_response_due_at": "2024-02-10T11:00:00Z",
    "resolution_due_at": "2024-02-10T18:00:00Z",
    "first_response_breached": false,
    "resolution_breached": false
  }
}
```

#### `GET /api/support/tickets`
List tickets with filters.

**Query Parameters:**
- `status` - Filter by status
- `priority` - Filter by priority
- `assignedTo` - Filter by assigned agent
- `teamId` - Filter by team
- `customerId` - Filter by customer
- `tags` - Comma-separated tags
- `search` - Search query
- `sort` - Sort field (created_at, priority, status)
- `limit` - Result limit (default: 50)
- `offset` - Result offset (default: 0)

**Response:**
```json
{
  "success": true,
  "tickets": [...],
  "count": 45
}
```

#### `PATCH /api/support/tickets/:ticketId`
Update ticket.

**Request:**
```json
{
  "status": "resolved",
  "priority": "medium",
  "assignedTo": "550e8400-e29b-41d4-a716-446655440002",
  "userId": "550e8400-e29b-41d4-a716-446655440004"
}
```

**Response:**
```json
{
  "success": true,
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "status": "resolved",
    "resolved_at": "2024-02-10T15:30:00Z"
  }
}
```

#### `POST /api/support/tickets/:ticketId/satisfaction`
Submit satisfaction rating.

**Request:**
```json
{
  "rating": 5,
  "comment": "Very helpful, issue resolved quickly!"
}
```

**Response:**
```json
{
  "success": true,
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "satisfaction_rating": 5,
    "satisfaction_submitted_at": "2024-02-10T16:00:00Z"
  }
}
```

### Ticket Messages

#### `POST /api/support/tickets/:ticketId/messages`
Add message to ticket.

**Request:**
```json
{
  "userId": "550e8400-e29b-41d4-a716-446655440002",
  "content": "I've reset your password. Please check your email.",
  "isInternal": false,
  "attachments": [
    {
      "name": "screenshot.png",
      "url": "https://example.com/files/screenshot.png",
      "size": 102400
    }
  ]
}
```

**Response:**
```json
{
  "success": true,
  "message": {
    "id": "550e8400-e29b-41d4-a716-446655440005",
    "ticket_id": "550e8400-e29b-41d4-a716-446655440003",
    "content": "I've reset your password...",
    "created_at": "2024-02-10T10:45:00Z"
  }
}
```

#### `GET /api/support/tickets/:ticketId/messages`
List ticket messages.

**Query Parameters:**
- `includeInternal` - Include internal notes (default: true)

**Response:**
```json
{
  "success": true,
  "messages": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440005",
      "user_id": "550e8400-e29b-41d4-a716-446655440002",
      "content": "I've reset your password...",
      "is_internal": false,
      "created_at": "2024-02-10T10:45:00Z"
    }
  ],
  "count": 3
}
```

### Team Management

#### `POST /api/support/teams`
Create support team.

**Request:**
```json
{
  "name": "Technical Support",
  "description": "Handles technical issues and bug reports",
  "email": "tech@example.com",
  "businessHours": {
    "monday": {"start": "09:00", "end": "17:00"},
    "tuesday": {"start": "09:00", "end": "17:00"}
  },
  "timezone": "America/New_York",
  "autoAssignmentEnabled": true,
  "assignmentMethod": "round_robin"
}
```

**Response:**
```json
{
  "success": true,
  "team": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Technical Support",
    "is_active": true,
    "member_count": 0
  }
}
```

#### `GET /api/support/teams`
List teams.

**Response:**
```json
{
  "success": true,
  "teams": [...],
  "count": 5
}
```

#### `POST /api/support/teams/:teamId/members`
Add member to team.

**Request:**
```json
{
  "userId": "550e8400-e29b-41d4-a716-446655440002",
  "role": "agent",
  "skills": ["authentication", "billing"],
  "skillLevel": 3,
  "maxConcurrentTickets": 10
}
```

**Response:**
```json
{
  "success": true,
  "member": {
    "id": "550e8400-e29b-41d4-a716-446655440006",
    "team_id": "550e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440002",
    "role": "agent",
    "is_active": true
  }
}
```

### SLA Policies

#### `POST /api/support/sla-policies`
Create SLA policy.

**Request:**
```json
{
  "name": "Standard SLA",
  "description": "Default SLA for all tickets",
  "urgentFirstResponseMinutes": 15,
  "urgentResolutionMinutes": 240,
  "highFirstResponseMinutes": 60,
  "highResolutionMinutes": 480,
  "mediumFirstResponseMinutes": 240,
  "mediumResolutionMinutes": 1440,
  "lowFirstResponseMinutes": 480,
  "lowResolutionMinutes": 2880,
  "appliesDuringBusinessHoursOnly": true,
  "escalationEnabled": true,
  "escalationThresholdMinutes": 30,
  "isDefault": true
}
```

**Response:**
```json
{
  "success": true,
  "policy": {
    "id": "550e8400-e29b-41d4-a716-446655440007",
    "name": "Standard SLA",
    "is_active": true,
    "is_default": true
  }
}
```

#### `GET /api/support/sla-policies`
List SLA policies.

**Response:**
```json
{
  "success": true,
  "policies": [...],
  "count": 4
}
```

### Canned Responses

#### `POST /api/support/canned-responses`
Create canned response.

**Request:**
```json
{
  "title": "Password Reset Instructions",
  "shortcut": "/pwd",
  "content": "To reset your password:\n1. Visit the login page\n2. Click 'Forgot Password'...",
  "category": "authentication",
  "tags": ["password", "login"],
  "visibility": "team",
  "teamId": "550e8400-e29b-41d4-a716-446655440001",
  "createdBy": "550e8400-e29b-41d4-a716-446655440002"
}
```

**Response:**
```json
{
  "success": true,
  "response": {
    "id": "550e8400-e29b-41d4-a716-446655440008",
    "title": "Password Reset Instructions",
    "shortcut": "/pwd",
    "is_active": true
  }
}
```

#### `GET /api/support/canned-responses`
List canned responses.

**Query Parameters:**
- `category` - Filter by category
- `teamId` - Filter by team
- `search` - Search query

**Response:**
```json
{
  "success": true,
  "responses": [...],
  "count": 28
}
```

### Knowledge Base

#### `POST /api/support/kb/articles`
Create KB article.

**Request:**
```json
{
  "title": "How to Reset Your Password",
  "slug": "reset-password",
  "content": "# Password Reset\n\nFollow these steps...",
  "summary": "Step-by-step guide to reset your password",
  "authorId": "550e8400-e29b-41d4-a716-446655440002",
  "category": "authentication",
  "tags": ["password", "login", "security"],
  "isPublic": true,
  "metaTitle": "Password Reset Guide",
  "metaDescription": "Learn how to reset your password"
}
```

**Response:**
```json
{
  "success": true,
  "article": {
    "id": "550e8400-e29b-41d4-a716-446655440009",
    "title": "How to Reset Your Password",
    "slug": "reset-password",
    "is_published": false,
    "version": 1
  }
}
```

#### `GET /api/support/kb/articles`
List KB articles.

**Query Parameters:**
- `category` - Filter by category
- `tags` - Comma-separated tags
- `search` - Search query
- `published` - Show only published (default: false)
- `public` - Show only public articles

**Response:**
```json
{
  "success": true,
  "articles": [...],
  "count": 156
}
```

#### `GET /api/support/kb/articles/:articleId`
Get article details.

**Response:**
```json
{
  "success": true,
  "article": {
    "id": "550e8400-e29b-41d4-a716-446655440009",
    "title": "How to Reset Your Password",
    "content": "# Password Reset...",
    "view_count": 1234,
    "helpful_count": 89,
    "not_helpful_count": 5
  }
}
```

#### `POST /api/support/kb/articles/:articleId/publish`
Publish KB article.

**Response:**
```json
{
  "success": true,
  "article": {
    "id": "550e8400-e29b-41d4-a716-446655440009",
    "is_published": true,
    "published_at": "2024-02-10T12:00:00Z"
  }
}
```

### Analytics

#### `GET /api/support/analytics`
Get support analytics.

**Query Parameters:**
- `startDate` - Start date (ISO 8601)
- `endDate` - End date (ISO 8601)
- `teamId` - Filter by team
- `agentId` - Filter by agent

**Response:**
```json
{
  "success": true,
  "analytics": {
    "ticketVolume": {
      "total": 1234,
      "new": 234,
      "resolved": 189,
      "closed": 156
    },
    "responseTime": {
      "avgFirstResponseSeconds": 1800,
      "avgResolutionSeconds": 28800
    },
    "slaCompliance": {
      "firstResponseRate": 0.95,
      "resolutionRate": 0.89
    },
    "satisfaction": {
      "csatAvg": 4.5,
      "csatCount": 234,
      "npsScore": 67
    },
    "agents": [
      {
        "userId": "550e8400-e29b-41d4-a716-446655440002",
        "ticketsHandled": 89,
        "avgResolutionSeconds": 25200,
        "csatAvg": 4.7
      }
    ]
  }
}
```

### Webhook Endpoint

#### `POST /webhook`
Receive webhook events.

**Request:**
```json
{
  "type": "ticket.created",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123",
    "subject": "Website login issue"
  }
}
```

**Response:**
```json
{
  "received": true,
  "type": "ticket.created"
}
```

## Webhook Events

The Support plugin emits the following webhook events:

### Ticket Events

#### `ticket.created`
New ticket created.

**Payload:**
```json
{
  "type": "ticket.created",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123",
    "subject": "Website login issue",
    "priority": "high",
    "status": "new"
  },
  "customer": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "customer@example.com"
  },
  "timestamp": "2024-02-10T10:00:00Z"
}
```

#### `ticket.updated`
Ticket updated.

**Payload:**
```json
{
  "type": "ticket.updated",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123"
  },
  "changes": {
    "status": {"old": "new", "new": "open"},
    "priority": {"old": "medium", "new": "high"}
  },
  "updated_by": "550e8400-e29b-41d4-a716-446655440002",
  "timestamp": "2024-02-10T10:15:00Z"
}
```

#### `ticket.resolved`
Ticket resolved.

**Payload:**
```json
{
  "type": "ticket.resolved",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123"
  },
  "resolved_by": "550e8400-e29b-41d4-a716-446655440002",
  "resolution_time_seconds": 18000,
  "timestamp": "2024-02-10T15:00:00Z"
}
```

#### `ticket.closed`
Ticket closed.

#### `ticket.reopened`
Ticket reopened.

#### `ticket.assigned`
Ticket assigned to agent.

**Payload:**
```json
{
  "type": "ticket.assigned",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123"
  },
  "assigned_to": "550e8400-e29b-41d4-a716-446655440002",
  "assigned_by": "550e8400-e29b-41d4-a716-446655440004",
  "timestamp": "2024-02-10T10:05:00Z"
}
```

#### `ticket.escalated`
Ticket escalated via SLA breach.

**Payload:**
```json
{
  "type": "ticket.escalated",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123"
  },
  "escalation_reason": "first_response_sla_breach",
  "escalated_to_team": "550e8400-e29b-41d4-a716-446655440010",
  "timestamp": "2024-02-10T11:30:00Z"
}
```

### Message Events

#### `message.created`
New ticket message.

**Payload:**
```json
{
  "type": "message.created",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123"
  },
  "message": {
    "id": "550e8400-e29b-41d4-a716-446655440005",
    "user_id": "550e8400-e29b-41d4-a716-446655440002",
    "content": "I've reset your password...",
    "is_internal": false
  },
  "timestamp": "2024-02-10T10:45:00Z"
}
```

### SLA Events

#### `sla.breach.first_response`
First response SLA breached.

**Payload:**
```json
{
  "type": "sla.breach.first_response",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123"
  },
  "sla_policy": {
    "id": "550e8400-e29b-41d4-a716-446655440007",
    "name": "Standard SLA"
  },
  "due_at": "2024-02-10T11:00:00Z",
  "breach_time_seconds": 300,
  "timestamp": "2024-02-10T11:05:00Z"
}
```

#### `sla.breach.resolution`
Resolution SLA breached.

### Satisfaction Events

#### `satisfaction.submitted`
Customer satisfaction survey submitted.

**Payload:**
```json
{
  "type": "satisfaction.submitted",
  "ticket": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "ticket_number": "TKT-00123"
  },
  "rating": 5,
  "comment": "Very helpful, issue resolved quickly!",
  "timestamp": "2024-02-10T16:00:00Z"
}
```

### Team Events

#### `team.created`
Support team created.

#### `team.member.added`
Member added to team.

**Payload:**
```json
{
  "type": "team.member.added",
  "team": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Technical Support"
  },
  "member": {
    "user_id": "550e8400-e29b-41d4-a716-446655440002",
    "role": "agent"
  },
  "timestamp": "2024-02-10T09:00:00Z"
}
```

### Knowledge Base Events

#### `kb.article.published`
Knowledge base article published.

**Payload:**
```json
{
  "type": "kb.article.published",
  "article": {
    "id": "550e8400-e29b-41d4-a716-446655440009",
    "title": "How to Reset Your Password",
    "slug": "reset-password"
  },
  "author_id": "550e8400-e29b-41d4-a716-446655440002",
  "timestamp": "2024-02-10T12:00:00Z"
}
```

## Database Schema

### np_support_sla_policies

SLA policies with priority-based targets.

```sql
CREATE TABLE np_support_sla_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(100) NOT NULL,
  description TEXT,
  urgent_first_response_minutes INTEGER DEFAULT 15,
  urgent_resolution_minutes INTEGER DEFAULT 240,
  high_first_response_minutes INTEGER DEFAULT 60,
  high_resolution_minutes INTEGER DEFAULT 480,
  medium_first_response_minutes INTEGER DEFAULT 240,
  medium_resolution_minutes INTEGER DEFAULT 1440,
  low_first_response_minutes INTEGER DEFAULT 480,
  low_resolution_minutes INTEGER DEFAULT 2880,
  applies_during_business_hours_only BOOLEAN NOT NULL DEFAULT true,
  business_hours JSONB,
  timezone VARCHAR(50) DEFAULT 'UTC',
  escalation_enabled BOOLEAN NOT NULL DEFAULT true,
  escalation_threshold_minutes INTEGER DEFAULT 30,
  escalate_to_team_id UUID,
  is_active BOOLEAN NOT NULL DEFAULT true,
  is_default BOOLEAN NOT NULL DEFAULT false,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sla_policies_account ON np_support_sla_policies(source_account_id);
CREATE INDEX idx_sla_policies_active ON np_support_sla_policies(is_active) WHERE is_active = true;
CREATE INDEX idx_sla_policies_default ON np_support_sla_policies(is_default) WHERE is_default = true;
```

### np_support_teams

Support teams with routing configuration.

```sql
CREATE TABLE np_support_teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(100) NOT NULL,
  description TEXT,
  email VARCHAR(255),
  is_active BOOLEAN NOT NULL DEFAULT true,
  business_hours JSONB,
  timezone VARCHAR(50) DEFAULT 'UTC',
  auto_assignment_enabled BOOLEAN NOT NULL DEFAULT true,
  assignment_method VARCHAR(50) DEFAULT 'round_robin',
  default_sla_policy_id UUID REFERENCES np_support_sla_policies(id) ON DELETE SET NULL,
  open_tickets_count INTEGER DEFAULT 0,
  member_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX idx_support_teams_account ON np_support_teams(source_account_id);
CREATE INDEX idx_support_teams_active ON np_support_teams(is_active) WHERE is_active = true;
```

**Assignment methods:** `round_robin`, `least_busy`, `skill_based`, `manual`

### np_support_team_members

Team members with skills and performance tracking.

```sql
CREATE TABLE np_support_team_members (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  team_id UUID NOT NULL REFERENCES np_support_teams(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  role VARCHAR(50) NOT NULL DEFAULT 'agent',
  skills TEXT[] DEFAULT '{}',
  skill_level INTEGER DEFAULT 1 CHECK (skill_level >= 1 AND skill_level <= 5),
  max_concurrent_tickets INTEGER DEFAULT 10,
  current_ticket_count INTEGER DEFAULT 0,
  is_active BOOLEAN NOT NULL DEFAULT true,
  is_available BOOLEAN NOT NULL DEFAULT true,
  availability_status VARCHAR(50) DEFAULT 'available',
  total_tickets_handled INTEGER DEFAULT 0,
  avg_first_response_time_seconds INTEGER,
  avg_resolution_time_seconds INTEGER,
  customer_satisfaction_avg DECIMAL(3,2),
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, team_id, user_id)
);

CREATE INDEX idx_support_team_members_account ON np_support_team_members(source_account_id);
CREATE INDEX idx_support_team_members_team ON np_support_team_members(team_id);
CREATE INDEX idx_support_team_members_user ON np_support_team_members(user_id);
CREATE INDEX idx_support_team_members_active ON np_support_team_members(is_active) WHERE is_active = true;
CREATE INDEX idx_support_team_members_available ON np_support_team_members(is_available) WHERE is_available = true;
```

**Roles:** `agent`, `lead`, `supervisor`, `manager`

**Availability statuses:** `available`, `busy`, `away`, `offline`

### np_support_tickets

Support tickets with SLA tracking.

```sql
CREATE TABLE np_support_tickets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  ticket_number VARCHAR(50) NOT NULL,
  customer_id UUID,
  customer_name VARCHAR(255),
  customer_email VARCHAR(255),
  customer_phone VARCHAR(50),
  subject VARCHAR(500) NOT NULL,
  description TEXT NOT NULL,
  status VARCHAR(50) NOT NULL DEFAULT 'new',
  priority VARCHAR(50) NOT NULL DEFAULT 'medium',
  assigned_to UUID,
  assigned_at TIMESTAMPTZ,
  team_id UUID REFERENCES np_support_teams(id) ON DELETE SET NULL,
  channel_id UUID,
  source VARCHAR(50) NOT NULL DEFAULT 'chat',
  category VARCHAR(100),
  tags TEXT[] DEFAULT '{}',
  sla_policy_id UUID REFERENCES np_support_sla_policies(id) ON DELETE SET NULL,
  first_response_due_at TIMESTAMPTZ,
  first_response_at TIMESTAMPTZ,
  resolution_due_at TIMESTAMPTZ,
  resolved_at TIMESTAMPTZ,
  first_response_breached BOOLEAN NOT NULL DEFAULT false,
  resolution_breached BOOLEAN NOT NULL DEFAULT false,
  satisfaction_rating INTEGER,
  satisfaction_comment TEXT,
  satisfaction_submitted_at TIMESTAMPTZ,
  custom_fields JSONB DEFAULT '{}'::jsonb,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  closed_at TIMESTAMPTZ,
  UNIQUE(source_account_id, ticket_number)
);

CREATE INDEX idx_support_tickets_account ON np_support_tickets(source_account_id);
CREATE INDEX idx_support_tickets_number ON np_support_tickets(ticket_number);
CREATE INDEX idx_support_tickets_customer ON np_support_tickets(customer_id);
CREATE INDEX idx_support_tickets_assigned ON np_support_tickets(assigned_to);
CREATE INDEX idx_support_tickets_team ON np_support_tickets(team_id);
CREATE INDEX idx_support_tickets_status ON np_support_tickets(status);
CREATE INDEX idx_support_tickets_priority ON np_support_tickets(priority);
CREATE INDEX idx_support_tickets_created ON np_support_tickets(created_at DESC);
CREATE INDEX idx_support_tickets_tags ON np_support_tickets USING GIN(tags);
CREATE INDEX idx_support_tickets_sla_breach ON np_support_tickets(first_response_breached, resolution_breached);
```

**Statuses:** `new`, `open`, `pending`, `resolved`, `closed`, `reopened`

**Priorities:** `low`, `medium`, `high`, `urgent`

**Sources:** `chat`, `email`, `api`, `web_form`, `phone`

### np_support_ticket_messages

Ticket messages and communications.

```sql
CREATE TABLE np_support_ticket_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  ticket_id UUID NOT NULL REFERENCES np_support_tickets(id) ON DELETE CASCADE,
  user_id UUID,
  content TEXT NOT NULL,
  is_internal BOOLEAN NOT NULL DEFAULT false,
  is_system BOOLEAN NOT NULL DEFAULT false,
  attachments JSONB DEFAULT '[]'::jsonb,
  email_message_id VARCHAR(255),
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ticket_messages_account ON np_support_ticket_messages(source_account_id);
CREATE INDEX idx_ticket_messages_ticket ON np_support_ticket_messages(ticket_id);
CREATE INDEX idx_ticket_messages_user ON np_support_ticket_messages(user_id);
CREATE INDEX idx_ticket_messages_created ON np_support_ticket_messages(created_at DESC);
```

### np_support_ticket_events

Audit trail for ticket changes.

```sql
CREATE TABLE np_support_ticket_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  ticket_id UUID NOT NULL REFERENCES np_support_tickets(id) ON DELETE CASCADE,
  user_id UUID,
  event_type VARCHAR(100) NOT NULL,
  field_name VARCHAR(100),
  old_value TEXT,
  new_value TEXT,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ticket_events_account ON np_support_ticket_events(source_account_id);
CREATE INDEX idx_ticket_events_ticket ON np_support_ticket_events(ticket_id);
CREATE INDEX idx_ticket_events_type ON np_support_ticket_events(event_type);
CREATE INDEX idx_ticket_events_created ON np_support_ticket_events(created_at DESC);
```

**Event types:** `created`, `updated`, `assigned`, `status_changed`, `priority_changed`, `resolved`, `closed`, `reopened`

### np_support_canned_responses

Pre-written responses.

```sql
CREATE TABLE np_support_canned_responses (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  title VARCHAR(200) NOT NULL,
  shortcut VARCHAR(50),
  content TEXT NOT NULL,
  category VARCHAR(100),
  tags TEXT[] DEFAULT '{}',
  visibility VARCHAR(50) NOT NULL DEFAULT 'team',
  team_id UUID REFERENCES np_support_teams(id) ON DELETE CASCADE,
  created_by UUID NOT NULL,
  attachments JSONB DEFAULT '[]'::jsonb,
  usage_count INTEGER DEFAULT 0,
  last_used_at TIMESTAMPTZ,
  is_active BOOLEAN NOT NULL DEFAULT true,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, shortcut)
);

CREATE INDEX idx_canned_responses_account ON np_support_canned_responses(source_account_id);
CREATE INDEX idx_canned_responses_shortcut ON np_support_canned_responses(shortcut);
CREATE INDEX idx_canned_responses_category ON np_support_canned_responses(category);
CREATE INDEX idx_canned_responses_team ON np_support_canned_responses(team_id);
CREATE INDEX idx_canned_responses_active ON np_support_canned_responses(is_active) WHERE is_active = true;
```

**Visibility:** `private`, `team`, `public`

### np_support_kb_articles

Knowledge base articles.

```sql
CREATE TABLE np_support_kb_articles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  title VARCHAR(500) NOT NULL,
  slug VARCHAR(500) NOT NULL,
  content TEXT NOT NULL,
  summary TEXT,
  author_id UUID NOT NULL,
  category VARCHAR(100),
  tags TEXT[] DEFAULT '{}',
  is_published BOOLEAN NOT NULL DEFAULT false,
  is_public BOOLEAN NOT NULL DEFAULT true,
  meta_title VARCHAR(200),
  meta_description VARCHAR(500),
  attachments JSONB DEFAULT '[]'::jsonb,
  related_articles UUID[] DEFAULT '{}',
  view_count INTEGER DEFAULT 0,
  helpful_count INTEGER DEFAULT 0,
  not_helpful_count INTEGER DEFAULT 0,
  version INTEGER DEFAULT 1,
  previous_version_id UUID,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  published_at TIMESTAMPTZ,
  UNIQUE(source_account_id, slug)
);

CREATE INDEX idx_kb_articles_account ON np_support_kb_articles(source_account_id);
CREATE INDEX idx_kb_articles_slug ON np_support_kb_articles(slug);
CREATE INDEX idx_kb_articles_author ON np_support_kb_articles(author_id);
CREATE INDEX idx_kb_articles_category ON np_support_kb_articles(category);
CREATE INDEX idx_kb_articles_tags ON np_support_kb_articles USING GIN(tags);
CREATE INDEX idx_kb_articles_published ON np_support_kb_articles(is_published) WHERE is_published = true;
```

### np_support_webhook_events

Webhook events log.

```sql
CREATE TABLE np_support_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(128),
  payload JSONB,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMPTZ,
  error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_support_webhook_events_account ON np_support_webhook_events(source_account_id);
CREATE INDEX idx_support_webhook_events_type ON np_support_webhook_events(event_type);
CREATE INDEX idx_support_webhook_events_processed ON np_support_webhook_events(processed);
```

## Examples

### Example 1: Create Ticket with SLA

```bash
# Create high-priority ticket
curl -X POST http://localhost:3709/api/support/tickets \
  -H "Content-Type: application/json" \
  -d '{
    "customerEmail": "customer@example.com",
    "subject": "Cannot access account",
    "description": "Getting error 500 when trying to login",
    "priority": "high",
    "source": "chat",
    "category": "authentication"
  }'

# Response includes SLA deadlines
{
  "success": true,
  "ticket": {
    "ticketNumber": "TKT-00123",
    "firstResponseDueAt": "2024-02-10T11:00:00Z",
    "resolutionDueAt": "2024-02-10T18:00:00Z"
  }
}
```

### Example 2: Agent Workflow

```bash
# Agent lists assigned tickets
curl "http://localhost:3709/api/support/tickets?assignedTo=AGENT_ID&status=open"

# Agent adds response
curl -X POST http://localhost:3709/api/support/tickets/TICKET_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "AGENT_ID",
    "content": "I have reset your password. Check your email."
  }'

# Agent resolves ticket
curl -X PATCH http://localhost:3709/api/support/tickets/TICKET_ID \
  -H "Content-Type: application/json" \
  -d '{
    "status": "resolved",
    "userId": "AGENT_ID"
  }'
```

### Example 3: Team Setup

```bash
# Create team
curl -X POST http://localhost:3709/api/support/teams \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Technical Support",
    "autoAssignmentEnabled": true,
    "assignmentMethod": "round_robin"
  }'

# Add agents
curl -X POST http://localhost:3709/api/support/teams/TEAM_ID/members \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "AGENT_1",
    "role": "agent",
    "maxConcurrentTickets": 10
  }'
```

### Example 4: Knowledge Base

```bash
# Create article
curl -X POST http://localhost:3709/api/support/kb/articles \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Password Reset Guide",
    "slug": "password-reset",
    "content": "# How to Reset\n\n1. Click Forgot Password...",
    "authorId": "USER_ID",
    "category": "authentication",
    "isPublic": true
  }'

# Publish article
curl -X POST http://localhost:3709/api/support/kb/articles/ARTICLE_ID/publish
```

### Example 5: Analytics Dashboard

```bash
# Get support metrics
curl "http://localhost:3709/api/support/analytics?startDate=2024-02-01&endDate=2024-02-10"

# Response includes:
{
  "analytics": {
    "ticketVolume": {"total": 1234, "new": 234},
    "responseTime": {"avgFirstResponseSeconds": 1800},
    "slaCompliance": {"firstResponseRate": 0.95},
    "satisfaction": {"csatAvg": 4.5}
  }
}
```

## Troubleshooting

### SLA Issues

**Problem:** SLA times incorrect or not calculating

**Solutions:**
1. Verify SLA policy is active and set as default:
   ```sql
   SELECT * FROM np_support_sla_policies
   WHERE is_default = true AND is_active = true;
   ```

2. Check business hours configuration matches timezone:
   ```bash
   echo $SUPPORT_BUSINESS_HOURS_START
   echo $SUPPORT_TIMEZONE
   ```

3. Ensure priority-specific SLA targets are set correctly

### Assignment Problems

**Problem:** Tickets not auto-assigning to agents

**Solutions:**
1. Verify auto-assignment is enabled:
   ```bash
   echo $SUPPORT_AUTO_ASSIGNMENT  # Should be 'true'
   ```

2. Check agents are available:
   ```sql
   SELECT * FROM np_support_team_members
   WHERE team_id = 'TEAM_ID'
   AND is_active = true
   AND is_available = true
   AND current_ticket_count < max_concurrent_tickets;
   ```

3. Review assignment method configuration

### Email Integration

**Problem:** Email notifications not sending

**Solutions:**
1. Verify SMTP configuration:
   ```bash
   echo $SUPPORT_EMAIL_SMTP_HOST
   echo $SUPPORT_EMAIL_SMTP_PORT
   ```

2. Test SMTP connectivity:
   ```bash
   telnet smtp.example.com 587
   ```

3. Check notification settings are enabled

### Performance Issues

**Problem:** Slow ticket queries

**Solutions:**
1. Review slow queries:
   ```sql
   SELECT query, mean_exec_time
   FROM pg_stat_statements
   WHERE query LIKE '%np_support_tickets%'
   ORDER BY mean_exec_time DESC;
   ```

2. Ensure indexes are present:
   ```sql
   SELECT indexname FROM pg_indexes
   WHERE tablename = 'np_support_tickets';
   ```

3. Consider partitioning for large ticket volumes

---

**Version:** 1.0.0
**Last Updated:** February 2024
**Support:** https://github.com/acamarata/nself-plugins/issues
