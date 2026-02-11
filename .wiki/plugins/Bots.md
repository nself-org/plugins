# Bots

Bot framework for nself-chat - commands, subscriptions, marketplace, API keys, reviews.

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

The Bots plugin provides a comprehensive bot framework for nself-chat applications. It enables developers to create, deploy, and manage chat bots with slash commands, event subscriptions, webhook integrations, and a full marketplace for bot discovery and installation.

This plugin is essential for extending chat functionality with automation, integrations, and interactive features that enhance user productivity and engagement.

### Key Features

- **Bot Creation & Management**: Full lifecycle management for custom and public bots
- **Slash Commands**: Register and execute slash commands with parameters and validation
- **Event Subscriptions**: Subscribe to workspace/channel events with webhook delivery
- **Marketplace**: Discover, install, and review bots from a central marketplace
- **OAuth Integration**: Secure bot authorization with OAuth 2.0 flow
- **API Key Management**: Generate and manage bot API keys with scoped permissions
- **Rate Limiting**: Per-bot rate limits for commands and API calls
- **Message Interactions**: Track and respond to button clicks, menu selections
- **Bot Reviews**: User ratings and reviews for marketplace bots
- **Webhook Delivery**: Reliable webhook delivery with retries and error tracking
- **Multi-Account Isolation**: Full support for multi-tenant workspaces

### Supported Bot Types

- **Custom Bots**: Private workspace-specific bots
- **Public Bots**: Marketplace bots available for all workspaces
- **Integration Bots**: Third-party service integrations (Slack, GitHub, etc.)
- **Workflow Bots**: Automation and workflow orchestration
- **Notification Bots**: Alert and notification delivery
- **AI Bots**: AI-powered conversational bots

### Use Cases

1. **Productivity Tools**: Todo lists, reminders, meeting schedulers
2. **DevOps Integration**: GitHub, GitLab, CI/CD notifications
3. **Customer Support**: Helpdesk automation, ticket creation
4. **Analytics**: Report generation, metrics dashboards
5. **Onboarding**: New user guides, interactive tutorials
6. **Games & Entertainment**: Trivia, polls, interactive games

## Quick Start

```bash
# Install the plugin
nself plugin install bots

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export BOTS_PLUGIN_PORT=3708

# Initialize database schema
nself plugin bots init

# Start the bots plugin server
nself plugin bots server

# Create your first bot
nself plugin bots bots:create "My Bot" mybot --description "My first bot"

# Check status
nself plugin bots status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `BOTS_PLUGIN_PORT` | No | `3708` | HTTP server port |
| `BOTS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | ` ` (empty) | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `BOT_WEBHOOK_TIMEOUT` | No | `30` | Webhook request timeout (seconds) |
| `BOT_WEBHOOK_RETRY_COUNT` | No | `3` | Number of webhook delivery retries |
| `BOT_WEBHOOK_RETRY_DELAY` | No | `5` | Delay between retries (seconds) |
| `BOT_DEFAULT_RATE_LIMIT_PER_MINUTE` | No | `60` | Default commands per minute |
| `BOT_DEFAULT_RATE_LIMIT_PER_HOUR` | No | `1000` | Default commands per hour |
| `BOT_DEFAULT_RATE_LIMIT_PER_DAY` | No | `10000` | Default commands per day |
| `BOT_OAUTH_ENABLED` | No | `false` | Enable OAuth 2.0 flow |
| `BOT_OAUTH_CALLBACK_URL` | No | - | OAuth callback URL |
| `BOT_MARKETPLACE_ENABLED` | No | `true` | Enable bot marketplace |
| `BOT_MARKETPLACE_MODERATION` | No | `true` | Enable marketplace moderation |
| `BOT_TOKEN_EXPIRY_DAYS` | No | `365` | Bot token expiration (days) |
| `BOT_WEBHOOK_SIGNATURE_ALGORITHM` | No | `sha256` | Webhook signature algorithm |
| `BOT_MAX_MESSAGE_SIZE` | No | `10000` | Maximum message size (characters) |
| `BOT_EVENT_QUEUE_SIZE` | No | `10000` | Event queue capacity |
| `BOT_EVENT_WORKER_COUNT` | No | `5` | Concurrent event workers |
| `BOT_COMMAND_TIMEOUT` | No | `10` | Command execution timeout (seconds) |
| `BOTS_API_KEY` | No | - | API key for authenticated requests |
| `BOTS_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `BOTS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
POSTGRES_SSL=false

# Server Configuration
BOTS_PLUGIN_PORT=3708
BOTS_PLUGIN_HOST=0.0.0.0

# Webhook Configuration
BOT_WEBHOOK_TIMEOUT=30
BOT_WEBHOOK_RETRY_COUNT=3
BOT_WEBHOOK_RETRY_DELAY=5
BOT_WEBHOOK_SIGNATURE_ALGORITHM=sha256

# Rate Limiting
BOT_DEFAULT_RATE_LIMIT_PER_MINUTE=60
BOT_DEFAULT_RATE_LIMIT_PER_HOUR=1000
BOT_DEFAULT_RATE_LIMIT_PER_DAY=10000

# OAuth Configuration
BOT_OAUTH_ENABLED=true
BOT_OAUTH_CALLBACK_URL=https://yourdomain.com/oauth/callback

# Marketplace Configuration
BOT_MARKETPLACE_ENABLED=true
BOT_MARKETPLACE_MODERATION=true

# Security
BOT_TOKEN_EXPIRY_DAYS=365
BOT_MAX_MESSAGE_SIZE=10000

# Performance
BOT_EVENT_QUEUE_SIZE=10000
BOT_EVENT_WORKER_COUNT=5
BOT_COMMAND_TIMEOUT=10

# API Security
BOTS_API_KEY=your-secret-api-key-here
BOTS_RATE_LIMIT_MAX=100
BOTS_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

## CLI Commands

### Global Commands

#### `init`
Initialize the bots plugin database schema.

```bash
nself plugin bots init
```

Creates all required tables, indexes, and constraints for bots, commands, subscriptions, installations, messages, interactions, reviews, and API keys.

#### `server`
Start the bots plugin HTTP server.

```bash
nself plugin bots server
nself plugin bots server --port 3708
```

**Options:**
- `-p, --port <port>` - Server port (default: 3708)

#### `status`
Display current bots plugin status and statistics.

```bash
nself plugin bots status
```

Shows:
- Marketplace and OAuth configuration status
- Total/enabled/public/verified bot counts
- Total commands, subscriptions, installations
- API key statistics

**Example output:**
```
Bots Plugin Status
===================
Marketplace:         enabled
OAuth:               enabled
Total Bots:          127
Enabled Bots:        112
Public Bots:         45
Verified Bots:       23
Total Commands:      384
Subscriptions:       256
Installations:       892 (743 active)
API Keys:            156
```

### Bot Management

#### `bots:create`
Create a new bot.

```bash
nself plugin bots bots:create <name> <username> [options]
nself plugin bots bots:create "Todo Bot" todobot --description "Manage your tasks"
nself plugin bots bots:create "GitHub Bot" github --owner user-id-123
```

**Arguments:**
- `<name>` - Bot display name
- `<username>` - Bot username (unique, alphanumeric + underscore)

**Options:**
- `-d, --description <description>` - Bot description
- `-o, --owner <ownerId>` - Owner user ID (default: 'system')

**Output:**
```
Bot created successfully!
ID:       550e8400-e29b-41d4-a716-446655440000
Name:     Todo Bot
Username: todobot
Token:    nbot_abc123xyz...

Save the token - it will not be shown again.
```

#### `bots:list`
List bots with optional filters.

```bash
nself plugin bots bots:list
nself plugin bots bots:list --owner user-id-123
nself plugin bots bots:list --public --limit 50
```

**Options:**
- `-o, --owner <ownerId>` - Filter by owner
- `--public` - Show only public bots
- `-l, --limit <limit>` - Result limit (default: 20)

**Example output:**
```
Bots (15):
===========
- Todo Bot (@todobot) [enabled, private]
  ID: 550e8400-... | Installs: 45 | Rating: 4.5/5
- GitHub Bot (@github) [enabled, public, verified]
  ID: 660e8400-... | Installs: 892 | Rating: 4.8/5
```

#### `bots:info`
Get detailed information about a bot.

```bash
nself plugin bots bots:info <botId>
```

**Example output:**
```
Bot: Todo Bot (@todobot)
================================
ID:          550e8400-e29b-41d4-a716-446655440000
Type:        custom
Description: Manage your tasks and projects
Enabled:     true
Public:      false
Verified:    false
Category:    productivity
Tags:        todo, tasks, productivity
Installs:    45
Messages:    1,234
Commands:    12
Rating:      4.5/5 (23 reviews)
```

#### `bots:delete`
Delete a bot (soft delete, marks as disabled).

```bash
nself plugin bots bots:delete <botId>
```

### Command Management

#### `bots:commands:register`
Register a slash command for a bot.

```bash
nself plugin bots bots:commands:register <botId> <command> <description>
nself plugin bots bots:commands:register BOT_ID todo "Create a new todo item"
```

**Arguments:**
- `<botId>` - Bot ID
- `<command>` - Command name (without /)
- `<description>` - Command description

#### `bots:commands:list`
List commands for a bot.

```bash
nself plugin bots bots:commands:list <botId>
```

**Example output:**
```
Commands for Todo Bot (5):
===========================
- /todo - Create a new todo item [used 234 times]
- /list - List all todos [used 189 times]
- /done - Mark todo as complete [used 156 times]
```

### API Key Management

#### `bots:token:generate`
Generate a new API key for a bot.

```bash
nself plugin bots bots:token:generate <botId> [options]
nself plugin bots bots:token:generate BOT_ID --name "Production Key"
```

**Options:**
- `-n, --name <name>` - Key name
- `-e, --expires <days>` - Expiration in days

**Output:**
```
API Key generated successfully!
Key: sk_live_abc123xyz...
Prefix: sk_live
Expires: 2025-02-10

Save the key - it will not be shown again.
```

### Marketplace

#### `marketplace:search`
Search marketplace for bots.

```bash
nself plugin bots marketplace:search --category productivity
nself plugin bots marketplace:search --verified
```

**Options:**
- `-c, --category <category>` - Filter by category
- `--verified` - Show only verified bots
- `-q, --query <query>` - Search query

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

If `BOTS_API_KEY` is set, include the API key in the `Authorization` header:

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
  "plugin": "bots",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `GET /ready`
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "bots",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `GET /live`
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "bots",
  "version": "1.0.0",
  "uptime": 86400,
  "memory": {
    "rss": 134217728,
    "heapTotal": 67108864,
    "heapUsed": 45088768
  },
  "stats": {
    "totalBots": 127,
    "enabledBots": 112,
    "publicBots": 45,
    "verifiedBots": 23,
    "totalCommands": 384,
    "totalSubscriptions": 256,
    "totalInstallations": 892,
    "activeInstallations": 743,
    "totalApiKeys": 156
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
  "plugin": "bots",
  "version": "1.0.0",
  "status": "running",
  "marketplaceEnabled": true,
  "oauthEnabled": true,
  "stats": {
    "totalBots": 127,
    "enabledBots": 112
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Bot Management

#### `POST /api/bots`
Create a new bot.

**Request:**
```json
{
  "name": "Todo Bot",
  "username": "todobot",
  "description": "Manage your tasks",
  "ownerId": "550e8400-e29b-41d4-a716-446655440000",
  "botType": "custom",
  "isPublic": false,
  "category": "productivity",
  "tags": ["todo", "tasks"],
  "rateLimitPerMinute": 60,
  "rateLimitPerHour": 1000
}
```

**Response:**
```json
{
  "success": true,
  "bot": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Todo Bot",
    "username": "todobot",
    "token": "nbot_abc123xyz..."
  }
}
```

#### `GET /api/bots/:botId`
Get bot details.

**Response:**
```json
{
  "success": true,
  "bot": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Todo Bot",
    "username": "todobot",
    "description": "Manage your tasks",
    "bot_type": "custom",
    "is_enabled": true,
    "is_public": false,
    "install_count": 45,
    "rating_avg": 4.5,
    "rating_count": 23
  }
}
```

#### `GET /api/bots`
List bots.

**Query Parameters:**
- `ownerId` - Filter by owner
- `isPublic` - Filter by public status (true/false)
- `limit` - Result limit (default: 50)
- `offset` - Result offset (default: 0)

**Response:**
```json
{
  "success": true,
  "bots": [...],
  "count": 15
}
```

#### `PATCH /api/bots/:botId`
Update bot configuration.

**Request:**
```json
{
  "description": "Updated description",
  "isPublic": true,
  "category": "productivity",
  "tags": ["todo", "tasks", "productivity"]
}
```

**Response:**
```json
{
  "success": true,
  "bot": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "description": "Updated description",
    "is_public": true
  }
}
```

#### `DELETE /api/bots/:botId`
Delete a bot.

**Response:**
```json
{
  "success": true,
  "deleted": true
}
```

### Command Management

#### `POST /api/bots/:botId/commands`
Register a command.

**Request:**
```json
{
  "command": "todo",
  "description": "Create a new todo item",
  "usageHint": "/todo [title] [due date]",
  "commandType": "message",
  "scope": "all",
  "parameters": [
    {
      "name": "title",
      "type": "string",
      "required": true,
      "description": "Todo title"
    }
  ]
}
```

**Response:**
```json
{
  "success": true,
  "command": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "bot_id": "550e8400-e29b-41d4-a716-446655440001",
    "command": "todo",
    "description": "Create a new todo item",
    "is_enabled": true
  }
}
```

#### `GET /api/bots/:botId/commands`
List bot commands.

**Response:**
```json
{
  "success": true,
  "commands": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "command": "todo",
      "description": "Create a new todo item",
      "usage_count": 234,
      "is_enabled": true
    }
  ],
  "count": 5
}
```

#### `DELETE /api/bots/:botId/commands/:commandId`
Delete a command.

**Response:**
```json
{
  "success": true,
  "deleted": true
}
```

### Event Subscriptions

#### `POST /api/bots/:botId/subscriptions`
Create event subscription.

**Request:**
```json
{
  "workspaceId": "550e8400-e29b-41d4-a716-446655440003",
  "channelId": "550e8400-e29b-41d4-a716-446655440004",
  "eventType": "message.created",
  "webhookUrl": "https://mybot.com/webhook",
  "webhookSecret": "webhook-secret-abc123",
  "filters": {
    "channelTypes": ["public", "private"]
  }
}
```

**Response:**
```json
{
  "success": true,
  "subscription": {
    "id": "550e8400-e29b-41d4-a716-446655440005",
    "bot_id": "550e8400-e29b-41d4-a716-446655440001",
    "event_type": "message.created",
    "is_active": true
  }
}
```

#### `GET /api/bots/:botId/subscriptions`
List subscriptions.

**Response:**
```json
{
  "success": true,
  "subscriptions": [...],
  "count": 8
}
```

#### `DELETE /api/bots/:botId/subscriptions/:subscriptionId`
Delete subscription.

**Response:**
```json
{
  "success": true,
  "deleted": true
}
```

### Bot Installation

#### `POST /api/workspaces/:workspaceId/bots/:botId/install`
Install bot in workspace.

**Request:**
```json
{
  "installedBy": "550e8400-e29b-41d4-a716-446655440006",
  "grantedPermissions": 7,
  "scope": "workspace",
  "config": {
    "enableNotifications": true
  }
}
```

**Response:**
```json
{
  "success": true,
  "installation": {
    "id": "550e8400-e29b-41d4-a716-446655440007",
    "bot_id": "550e8400-e29b-41d4-a716-446655440001",
    "workspace_id": "550e8400-e29b-41d4-a716-446655440003",
    "is_active": true,
    "installed_at": "2024-02-10T10:30:00Z"
  }
}
```

#### `DELETE /api/workspaces/:workspaceId/bots/:botId/uninstall`
Uninstall bot from workspace.

**Request:**
```json
{
  "uninstalledBy": "550e8400-e29b-41d4-a716-446655440006"
}
```

**Response:**
```json
{
  "success": true,
  "uninstalled": true
}
```

#### `GET /api/workspaces/:workspaceId/bots`
List installed bots in workspace.

**Response:**
```json
{
  "success": true,
  "installations": [...],
  "count": 12
}
```

### Marketplace

#### `GET /api/marketplace/bots`
Search marketplace.

**Query Parameters:**
- `category` - Filter by category
- `verified` - Show only verified bots
- `search` - Search query
- `sort` - Sort by (installs, rating, recent)
- `limit` - Result limit (default: 50)
- `offset` - Result offset (default: 0)

**Response:**
```json
{
  "success": true,
  "bots": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "name": "GitHub Bot",
      "username": "github",
      "description": "GitHub integration",
      "category": "devops",
      "is_verified": true,
      "install_count": 892,
      "rating_avg": 4.8,
      "rating_count": 234
    }
  ],
  "count": 45
}
```

#### `GET /api/marketplace/bots/:botId`
Get marketplace bot details.

**Response:**
```json
{
  "success": true,
  "bot": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "GitHub Bot",
    "description": "Complete GitHub integration",
    "category": "devops",
    "tags": ["github", "git", "devops"],
    "website_url": "https://github-bot.com",
    "support_url": "https://github-bot.com/support",
    "is_verified": true,
    "install_count": 892,
    "rating_avg": 4.8
  }
}
```

#### `POST /api/marketplace/bots/:botId/reviews`
Submit bot review.

**Request:**
```json
{
  "userId": "550e8400-e29b-41d4-a716-446655440006",
  "rating": 5,
  "title": "Excellent bot!",
  "comment": "Works perfectly for our team."
}
```

**Response:**
```json
{
  "success": true,
  "review": {
    "id": "550e8400-e29b-41d4-a716-446655440008",
    "bot_id": "550e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440006",
    "rating": 5,
    "title": "Excellent bot!",
    "is_published": true,
    "created_at": "2024-02-10T10:30:00Z"
  }
}
```

### API Key Management

#### `POST /api/bots/:botId/api-keys`
Generate API key.

**Request:**
```json
{
  "keyName": "Production Key",
  "permissions": 7,
  "scopes": ["messages:write", "channels:read"],
  "expiresAt": "2025-02-10T10:30:00Z"
}
```

**Response:**
```json
{
  "success": true,
  "apiKey": {
    "id": "550e8400-e29b-41d4-a716-446655440009",
    "key": "sk_live_abc123xyz...",
    "key_prefix": "sk_live",
    "key_name": "Production Key",
    "expires_at": "2025-02-10T10:30:00Z"
  }
}
```

#### `GET /api/bots/:botId/api-keys`
List API keys.

**Response:**
```json
{
  "success": true,
  "keys": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440009",
      "key_name": "Production Key",
      "key_prefix": "sk_live",
      "is_active": true,
      "last_used_at": "2024-02-10T09:00:00Z",
      "use_count": 1234
    }
  ],
  "count": 3
}
```

#### `DELETE /api/bots/:botId/api-keys/:keyId`
Revoke API key.

**Request:**
```json
{
  "revokedBy": "550e8400-e29b-41d4-a716-446655440006",
  "reason": "Key compromised"
}
```

**Response:**
```json
{
  "success": true,
  "revoked": true
}
```

### Webhook Endpoint

#### `POST /webhook`
Receive webhook events.

**Request:**
```json
{
  "type": "bot.created",
  "bot": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Todo Bot",
    "username": "todobot"
  }
}
```

**Response:**
```json
{
  "received": true,
  "type": "bot.created"
}
```

## Webhook Events

The Bots plugin emits the following webhook events:

### Bot Events

#### `bot.created`
New bot created.

**Payload:**
```json
{
  "type": "bot.created",
  "bot": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Todo Bot",
    "username": "todobot",
    "bot_type": "custom"
  },
  "owner_id": "550e8400-e29b-41d4-a716-446655440006",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `bot.updated`
Bot configuration updated.

**Payload:**
```json
{
  "type": "bot.updated",
  "bot": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Todo Bot"
  },
  "changes": {
    "description": "New description",
    "is_public": true
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `bot.deleted`
Bot deleted.

**Payload:**
```json
{
  "type": "bot.deleted",
  "bot_id": "550e8400-e29b-41d4-a716-446655440001",
  "username": "todobot",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Installation Events

#### `bot.installed`
Bot installed in workspace.

**Payload:**
```json
{
  "type": "bot.installed",
  "bot_id": "550e8400-e29b-41d4-a716-446655440001",
  "workspace_id": "550e8400-e29b-41d4-a716-446655440003",
  "installed_by": "550e8400-e29b-41d4-a716-446655440006",
  "granted_permissions": 7,
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `bot.uninstalled`
Bot uninstalled from workspace.

**Payload:**
```json
{
  "type": "bot.uninstalled",
  "bot_id": "550e8400-e29b-41d4-a716-446655440001",
  "workspace_id": "550e8400-e29b-41d4-a716-446655440003",
  "uninstalled_by": "550e8400-e29b-41d4-a716-446655440006",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Command Events

#### `command.registered`
Bot command registered.

**Payload:**
```json
{
  "type": "command.registered",
  "bot_id": "550e8400-e29b-41d4-a716-446655440001",
  "command": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "command": "todo",
    "description": "Create a new todo item"
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `command.executed`
Bot command executed.

**Payload:**
```json
{
  "type": "command.executed",
  "bot_id": "550e8400-e29b-41d4-a716-446655440001",
  "command": "todo",
  "user_id": "550e8400-e29b-41d4-a716-446655440006",
  "workspace_id": "550e8400-e29b-41d4-a716-446655440003",
  "channel_id": "550e8400-e29b-41d4-a716-446655440004",
  "parameters": {
    "title": "Buy groceries"
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Interaction Events

#### `interaction.created`
User interacted with bot message.

**Payload:**
```json
{
  "type": "interaction.created",
  "bot_id": "550e8400-e29b-41d4-a716-446655440001",
  "message_id": "550e8400-e29b-41d4-a716-446655440010",
  "user_id": "550e8400-e29b-41d4-a716-446655440006",
  "interaction_type": "button_click",
  "interaction_id": "btn_complete",
  "interaction_value": {"todo_id": "123"},
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Review Events

#### `review.submitted`
Bot review submitted.

**Payload:**
```json
{
  "type": "review.submitted",
  "bot_id": "550e8400-e29b-41d4-a716-446655440001",
  "review": {
    "id": "550e8400-e29b-41d4-a716-446655440008",
    "user_id": "550e8400-e29b-41d4-a716-446655440006",
    "rating": 5,
    "title": "Excellent bot!"
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

## Database Schema

### nchat_bots

Stores bot configurations.

```sql
CREATE TABLE nchat_bots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(100) NOT NULL,
  username VARCHAR(50) NOT NULL,
  description TEXT,
  avatar_url TEXT,
  bot_type VARCHAR(50) NOT NULL DEFAULT 'custom',
  owner_id UUID NOT NULL,
  workspace_id UUID,
  token_hash TEXT NOT NULL,
  oauth_client_id VARCHAR(255),
  oauth_client_secret_encrypted TEXT,
  permissions BIGINT NOT NULL DEFAULT 0,
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  is_verified BOOLEAN NOT NULL DEFAULT false,
  is_public BOOLEAN NOT NULL DEFAULT false,
  category VARCHAR(50),
  tags TEXT[] DEFAULT '{}',
  website_url TEXT,
  support_url TEXT,
  privacy_policy_url TEXT,
  terms_of_service_url TEXT,
  install_count INTEGER DEFAULT 0,
  message_count INTEGER DEFAULT 0,
  command_count INTEGER DEFAULT 0,
  rating_avg DECIMAL(3,2) DEFAULT 0.0,
  rating_count INTEGER DEFAULT 0,
  last_active_at TIMESTAMPTZ,
  last_message_at TIMESTAMPTZ,
  rate_limit_per_minute INTEGER DEFAULT 60,
  rate_limit_per_hour INTEGER DEFAULT 1000,
  rate_limit_per_day INTEGER DEFAULT 10000,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, username)
);

CREATE INDEX idx_bots_account ON nchat_bots(source_account_id);
CREATE INDEX idx_bots_username ON nchat_bots(username);
CREATE INDEX idx_bots_owner ON nchat_bots(owner_id);
CREATE INDEX idx_bots_workspace ON nchat_bots(workspace_id);
CREATE INDEX idx_bots_public ON nchat_bots(is_public) WHERE is_public = true;
CREATE INDEX idx_bots_verified ON nchat_bots(is_verified) WHERE is_verified = true;
CREATE INDEX idx_bots_category ON nchat_bots(category) WHERE category IS NOT NULL;
CREATE INDEX idx_bots_tags ON nchat_bots USING GIN(tags);
```

**Bot types:** `custom`, `integration`, `workflow`, `notification`, `ai`

**Categories:** `productivity`, `devops`, `support`, `analytics`, `games`, `other`

### nchat_bot_commands

Stores slash commands.

```sql
CREATE TABLE nchat_bot_commands (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
  command VARCHAR(50) NOT NULL,
  description TEXT NOT NULL,
  usage_hint TEXT,
  command_type VARCHAR(50) NOT NULL DEFAULT 'message',
  scope VARCHAR(50) NOT NULL DEFAULT 'all',
  parameters JSONB DEFAULT '[]'::jsonb,
  required_permissions BIGINT DEFAULT 0,
  rate_limit_per_minute INTEGER,
  rate_limit_per_hour INTEGER,
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  usage_count INTEGER DEFAULT 0,
  last_used_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, bot_id, command)
);

CREATE INDEX idx_bot_commands_account ON nchat_bot_commands(source_account_id);
CREATE INDEX idx_bot_commands_bot ON nchat_bot_commands(bot_id);
CREATE INDEX idx_bot_commands_command ON nchat_bot_commands(command);
CREATE INDEX idx_bot_commands_enabled ON nchat_bot_commands(is_enabled) WHERE is_enabled = true;
```

**Command types:** `message`, `user`, `shortcut`

**Scopes:** `all`, `workspace`, `channel`, `dm`

### nchat_bot_subscriptions

Event subscriptions for webhooks.

```sql
CREATE TABLE nchat_bot_subscriptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
  workspace_id UUID,
  channel_id UUID,
  event_type VARCHAR(100) NOT NULL,
  filters JSONB DEFAULT '{}'::jsonb,
  delivery_mode VARCHAR(50) NOT NULL DEFAULT 'webhook',
  webhook_url TEXT,
  webhook_secret VARCHAR(255),
  is_active BOOLEAN NOT NULL DEFAULT true,
  event_count INTEGER DEFAULT 0,
  last_event_at TIMESTAMPTZ,
  failed_delivery_count INTEGER DEFAULT 0,
  last_failure_at TIMESTAMPTZ,
  last_error_message TEXT,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, bot_id, workspace_id, channel_id, event_type)
);

CREATE INDEX idx_bot_subscriptions_account ON nchat_bot_subscriptions(source_account_id);
CREATE INDEX idx_bot_subscriptions_bot ON nchat_bot_subscriptions(bot_id);
CREATE INDEX idx_bot_subscriptions_workspace ON nchat_bot_subscriptions(workspace_id);
CREATE INDEX idx_bot_subscriptions_channel ON nchat_bot_subscriptions(channel_id);
CREATE INDEX idx_bot_subscriptions_event ON nchat_bot_subscriptions(event_type);
CREATE INDEX idx_bot_subscriptions_active ON nchat_bot_subscriptions(is_active) WHERE is_active = true;
```

**Event types:** `message.created`, `message.updated`, `channel.created`, `user.joined`, etc.

### nchat_bot_installations

Bot installations in workspaces.

```sql
CREATE TABLE nchat_bot_installations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
  workspace_id UUID NOT NULL,
  installed_by UUID NOT NULL,
  scope VARCHAR(50) NOT NULL DEFAULT 'workspace',
  channel_id UUID,
  config JSONB DEFAULT '{}'::jsonb,
  granted_permissions BIGINT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  oauth_access_token_encrypted TEXT,
  oauth_refresh_token_encrypted TEXT,
  oauth_expires_at TIMESTAMPTZ,
  oauth_scope TEXT,
  message_count INTEGER DEFAULT 0,
  command_count INTEGER DEFAULT 0,
  last_used_at TIMESTAMPTZ,
  installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  uninstalled_at TIMESTAMPTZ,
  uninstalled_by UUID,
  UNIQUE(source_account_id, bot_id, workspace_id, channel_id)
);

CREATE INDEX idx_bot_installations_account ON nchat_bot_installations(source_account_id);
CREATE INDEX idx_bot_installations_bot ON nchat_bot_installations(bot_id);
CREATE INDEX idx_bot_installations_workspace ON nchat_bot_installations(workspace_id);
CREATE INDEX idx_bot_installations_channel ON nchat_bot_installations(channel_id);
CREATE INDEX idx_bot_installations_active ON nchat_bot_installations(is_active) WHERE is_active = true;
```

### nchat_bot_messages

Tracks bot messages.

```sql
CREATE TABLE nchat_bot_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
  message_id UUID NOT NULL,
  channel_id UUID NOT NULL,
  message_type VARCHAR(50) NOT NULL,
  interaction_count INTEGER DEFAULT 0,
  last_interaction_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bot_messages_account ON nchat_bot_messages(source_account_id);
CREATE INDEX idx_bot_messages_bot ON nchat_bot_messages(bot_id);
CREATE INDEX idx_bot_messages_message ON nchat_bot_messages(message_id);
CREATE INDEX idx_bot_messages_channel ON nchat_bot_messages(channel_id);
CREATE INDEX idx_bot_messages_created ON nchat_bot_messages(created_at DESC);
```

### nchat_bot_interactions

User interactions with bot messages.

```sql
CREATE TABLE nchat_bot_interactions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
  message_id UUID NOT NULL,
  user_id UUID NOT NULL,
  interaction_type VARCHAR(50) NOT NULL,
  interaction_id VARCHAR(255) NOT NULL,
  interaction_value JSONB,
  response_sent BOOLEAN NOT NULL DEFAULT false,
  response_message_id UUID,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bot_interactions_account ON nchat_bot_interactions(source_account_id);
CREATE INDEX idx_bot_interactions_bot ON nchat_bot_interactions(bot_id);
CREATE INDEX idx_bot_interactions_message ON nchat_bot_interactions(message_id);
CREATE INDEX idx_bot_interactions_user ON nchat_bot_interactions(user_id);
CREATE INDEX idx_bot_interactions_type ON nchat_bot_interactions(interaction_type);
CREATE INDEX idx_bot_interactions_created ON nchat_bot_interactions(created_at DESC);
```

**Interaction types:** `button_click`, `menu_select`, `modal_submit`, `overflow_select`

### nchat_bot_reviews

Bot reviews and ratings.

```sql
CREATE TABLE nchat_bot_reviews (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
  title VARCHAR(200),
  comment TEXT,
  is_published BOOLEAN NOT NULL DEFAULT true,
  is_flagged BOOLEAN NOT NULL DEFAULT false,
  moderated_at TIMESTAMPTZ,
  moderated_by UUID,
  moderation_reason TEXT,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, bot_id, user_id)
);

CREATE INDEX idx_bot_reviews_account ON nchat_bot_reviews(source_account_id);
CREATE INDEX idx_bot_reviews_bot ON nchat_bot_reviews(bot_id);
CREATE INDEX idx_bot_reviews_user ON nchat_bot_reviews(user_id);
CREATE INDEX idx_bot_reviews_rating ON nchat_bot_reviews(rating);
CREATE INDEX idx_bot_reviews_published ON nchat_bot_reviews(is_published) WHERE is_published = true;
```

### nchat_bot_api_keys

API keys for bot authentication.

```sql
CREATE TABLE nchat_bot_api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
  key_name VARCHAR(100) NOT NULL,
  key_hash VARCHAR(255) NOT NULL,
  key_prefix VARCHAR(20) NOT NULL,
  permissions BIGINT NOT NULL,
  scopes TEXT[] DEFAULT '{}',
  is_active BOOLEAN NOT NULL DEFAULT true,
  rate_limit_per_minute INTEGER,
  rate_limit_per_hour INTEGER,
  expires_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  use_count INTEGER DEFAULT 0,
  revoked_at TIMESTAMPTZ,
  revoked_by UUID,
  revoke_reason TEXT,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, key_hash)
);

CREATE INDEX idx_bot_api_keys_account ON nchat_bot_api_keys(source_account_id);
CREATE INDEX idx_bot_api_keys_bot ON nchat_bot_api_keys(bot_id);
CREATE INDEX idx_bot_api_keys_hash ON nchat_bot_api_keys(key_hash);
CREATE INDEX idx_bot_api_keys_active ON nchat_bot_api_keys(is_active) WHERE is_active = true;
CREATE INDEX idx_bot_api_keys_expires ON nchat_bot_api_keys(expires_at) WHERE expires_at IS NOT NULL;
```

**Key prefixes:** `sk_live_`, `sk_test_`

### nchat_bots_webhook_events

Webhook events log.

```sql
CREATE TABLE nchat_bots_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(128),
  payload JSONB,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMPTZ,
  error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bots_webhook_events_account ON nchat_bots_webhook_events(source_account_id);
CREATE INDEX idx_bots_webhook_events_type ON nchat_bots_webhook_events(event_type);
CREATE INDEX idx_bots_webhook_events_processed ON nchat_bots_webhook_events(processed);
```

## Examples

### Example 1: Create Bot with Commands

```bash
# Create a bot
curl -X POST http://localhost:3708/api/bots \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Todo Bot",
    "username": "todobot",
    "description": "Manage your tasks",
    "ownerId": "550e8400-e29b-41d4-a716-446655440000",
    "isPublic": false,
    "category": "productivity",
    "tags": ["todo", "tasks"]
  }'

# Save the bot token from response!

# Register commands
curl -X POST http://localhost:3708/api/bots/BOT_ID/commands \
  -H "Content-Type: application/json" \
  -d '{
    "command": "todo",
    "description": "Create a new todo item",
    "parameters": [
      {"name": "title", "type": "string", "required": true}
    ]
  }'
```

### Example 2: Install Bot in Workspace

```bash
# Install bot
curl -X POST http://localhost:3708/api/workspaces/WORKSPACE_ID/bots/BOT_ID/install \
  -H "Content-Type: application/json" \
  -d '{
    "installedBy": "USER_ID",
    "grantedPermissions": 7,
    "config": {
      "enableNotifications": true
    }
  }'

# List installed bots
curl http://localhost:3708/api/workspaces/WORKSPACE_ID/bots
```

### Example 3: Subscribe to Events

```bash
# Create subscription
curl -X POST http://localhost:3708/api/bots/BOT_ID/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "workspaceId": "WORKSPACE_ID",
    "eventType": "message.created",
    "webhookUrl": "https://mybot.com/webhook",
    "webhookSecret": "secret-abc123",
    "filters": {
      "channelTypes": ["public"]
    }
  }'
```

### Example 4: Generate API Key

```bash
# Generate key
curl -X POST http://localhost:3708/api/bots/BOT_ID/api-keys \
  -H "Content-Type: application/json" \
  -d '{
    "keyName": "Production Key",
    "permissions": 7,
    "scopes": ["messages:write", "channels:read"]
  }'

# Save the key from response!
```

### Example 5: Marketplace Search

```bash
# Search marketplace
curl "http://localhost:3708/api/marketplace/bots?category=productivity&sort=rating"

# Submit review
curl -X POST http://localhost:3708/api/marketplace/bots/BOT_ID/reviews \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "USER_ID",
    "rating": 5,
    "title": "Excellent bot!",
    "comment": "Very useful for our team."
  }'
```

## Troubleshooting

### Bot Not Receiving Events

**Problem:** Bot subscriptions not receiving webhook events

**Solutions:**
1. Verify subscription is active:
   ```bash
   curl http://localhost:3708/api/bots/BOT_ID/subscriptions
   ```

2. Check webhook URL is accessible:
   ```bash
   curl -X POST https://your-bot.com/webhook -d '{"test": true}'
   ```

3. Review failed deliveries:
   ```sql
   SELECT * FROM nchat_bot_subscriptions
   WHERE bot_id = 'BOT_ID'
   AND failed_delivery_count > 0;
   ```

4. Verify webhook signature validation matches algorithm

### Command Not Working

**Problem:** Slash commands not executing

**Solutions:**
1. Verify command is registered and enabled:
   ```bash
   curl http://localhost:3708/api/bots/BOT_ID/commands
   ```

2. Check rate limits:
   ```sql
   SELECT usage_count, last_used_at, rate_limit_per_minute
   FROM nchat_bot_commands
   WHERE bot_id = 'BOT_ID' AND command = 'your-command';
   ```

3. Ensure bot is installed in workspace:
   ```bash
   curl http://localhost:3708/api/workspaces/WORKSPACE_ID/bots
   ```

### Authentication Errors

**Problem:** "Invalid token" or "Unauthorized" errors

**Solutions:**
1. Verify bot token is correct (check token_hash in database)
2. Ensure token hasn't expired (check created_at vs TOKEN_EXPIRY_DAYS)
3. Use correct header format: `Authorization: Bot YOUR_TOKEN`

### Marketplace Issues

**Problem:** Bot not appearing in marketplace

**Solutions:**
1. Verify bot is marked as public:
   ```sql
   UPDATE nchat_bots
   SET is_public = true
   WHERE id = 'BOT_ID';
   ```

2. Check marketplace is enabled:
   ```bash
   echo $BOT_MARKETPLACE_ENABLED  # Should be 'true'
   ```

3. Ensure moderation is passed if enabled

---

**Version:** 1.0.0
**Last Updated:** February 2024
**Support:** https://github.com/acamarata/nself-plugins/issues
