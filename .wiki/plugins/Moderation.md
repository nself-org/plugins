# Moderation

Comprehensive content moderation with profanity filtering, toxicity detection, automated actions, and manual review workflows.

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

The Moderation plugin provides comprehensive content moderation capabilities for nself applications. It combines automated filtering with manual review workflows to keep communities safe and enforce content policies. The plugin supports profanity detection, toxicity scoring, automated moderation actions, appeals processes, and detailed user risk tracking.

This plugin is essential for any platform with user-generated content, providing the tools needed to enforce community guidelines while maintaining transparency and fairness through appeals and audit trails.

### Key Features

- **Profanity Filtering**: Customizable wordlists with regex support and multi-language detection
- **Toxicity Detection**: Optional AI-powered toxicity scoring via Perspective API, OpenAI, or local models
- **Automated Moderation**: Rule-based automatic actions (warn, mute, ban, delete content)
- **Manual Review Queue**: Flag content for human review with severity-based prioritization
- **User Reporting**: Community-driven content reporting with spam prevention
- **Appeals System**: Allow users to contest moderation actions with review workflows
- **Risk Scoring**: Track user behavior patterns and calculate risk levels
- **User Statistics**: Comprehensive per-user moderation history and metrics
- **Audit Trail**: Complete log of all moderation actions for accountability
- **Multi-Account Isolation**: Full support for multi-tenant applications

### Supported Moderation Actions

- **Warn**: Issue a warning to a user
- **Mute**: Temporarily or permanently mute a user
- **Kick**: Remove user from channel/server
- **Ban**: Temporarily or permanently ban a user
- **Delete**: Remove offensive content
- **Flag**: Mark content for review

### Toxicity Detection Providers

- **local**: Built-in keyword-based detection (default, no API required)
- **perspective_api**: Google's Perspective API for advanced toxicity scoring
- **openai**: OpenAI's moderation API

### Use Cases

1. **Chat Platforms**: Real-time content moderation for messaging apps
2. **Forums & Communities**: Enforce community guidelines and handle user reports
3. **Social Networks**: Automated moderation at scale with human oversight
4. **Gaming Platforms**: Toxicity detection and player behavior tracking
5. **E-commerce**: Review moderation and spam prevention

## Quick Start

```bash
# Install the plugin
nself plugin install moderation

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export MODERATION_PLUGIN_PORT=3704

# Initialize database schema
nself plugin moderation init

# Start the moderation server
nself plugin moderation server

# Check status
nself plugin moderation status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `MODERATION_PLUGIN_PORT` | No | `3704` | HTTP server port |
| `MODERATION_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `` (empty) | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `MODERATION_TOXICITY_ENABLED` | No | `false` | Enable AI toxicity detection |
| `MODERATION_TOXICITY_PROVIDER` | No | `local` | Toxicity provider: local, perspective_api, openai |
| `MODERATION_TOXICITY_THRESHOLD` | No | `0.8` | Toxicity score threshold (0.0-1.0) |
| `MODERATION_AUTO_DELETE_ENABLED` | No | `true` | Automatically delete highly toxic content |
| `MODERATION_AUTO_DELETE_THRESHOLD` | No | `0.95` | Threshold for auto-deletion (0.0-1.0) |
| `MODERATION_AUTO_MUTE_ENABLED` | No | `false` | Automatically mute repeat offenders |
| `MODERATION_AUTO_MUTE_VIOLATIONS` | No | `3` | Number of violations before auto-mute |
| `MODERATION_AUTO_BAN_ENABLED` | No | `false` | Automatically ban severe repeat offenders |
| `MODERATION_APPEALS_ENABLED` | No | `true` | Allow users to appeal moderation actions |
| `MODERATION_APPEALS_TIME_LIMIT_DAYS` | No | `7` | Days users have to submit appeals |
| `MODERATION_CLEANUP_ENABLED` | No | `true` | Automatically clean up expired actions |
| `MODERATION_CLEANUP_INTERVAL_MINUTES` | No | `60` | Cleanup interval in minutes |
| `MODERATION_MAX_REPORTS_PER_USER_PER_DAY` | No | `10` | Maximum reports per user per day (spam prevention) |
| `MODERATION_API_KEY` | No | - | API key for authenticated requests |
| `MODERATION_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `MODERATION_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
POSTGRES_SSL=false

# Server Configuration
MODERATION_PLUGIN_PORT=3704
MODERATION_PLUGIN_HOST=0.0.0.0

# Toxicity Detection (Optional - requires API keys)
MODERATION_TOXICITY_ENABLED=false
MODERATION_TOXICITY_PROVIDER=local
MODERATION_TOXICITY_THRESHOLD=0.8

# Automated Moderation
MODERATION_AUTO_DELETE_ENABLED=true
MODERATION_AUTO_DELETE_THRESHOLD=0.95
MODERATION_AUTO_MUTE_ENABLED=false
MODERATION_AUTO_MUTE_VIOLATIONS=3
MODERATION_AUTO_BAN_ENABLED=false

# Appeals System
MODERATION_APPEALS_ENABLED=true
MODERATION_APPEALS_TIME_LIMIT_DAYS=7

# Maintenance
MODERATION_CLEANUP_ENABLED=true
MODERATION_CLEANUP_INTERVAL_MINUTES=60

# Rate Limiting
MODERATION_MAX_REPORTS_PER_USER_PER_DAY=10

# Security
MODERATION_API_KEY=your-secret-api-key-here
MODERATION_RATE_LIMIT_MAX=100
MODERATION_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

## CLI Commands

### Global Commands

#### `init`
Initialize the moderation plugin database schema.

```bash
nself plugin moderation init
```

Creates all required tables, indexes, and constraints.

#### `server`
Start the moderation plugin HTTP server.

```bash
nself plugin moderation server
nself plugin moderation server --port 3704
```

**Options:**
- `-p, --port <port>` - Server port (default: 3704)

#### `status`
Display current moderation plugin status and statistics.

```bash
nself plugin moderation status
```

Shows configuration, rules, wordlists, and 30-day statistics.

### Content Analysis Commands

#### `analyze`
Analyze content for moderation issues.

```bash
nself plugin moderation analyze --content "This is test content"
nself plugin moderation analyze --content "Bad word here" --type message
```

**Options:**
- `--content <content>` - Content to analyze (required)
- `--type <type>` - Content type (default: message)

### Queue Management Commands

#### `queue`
View the moderation review queue.

```bash
nself plugin moderation queue
nself plugin moderation queue --status pending --severity high --limit 50
```

**Options:**
- `--status <status>` - Filter by status (default: pending)
- `--severity <severity>` - Filter by severity (low, medium, high, critical)
- `--limit <limit>` - Result limit (default: 20)

### Action Commands

#### `action`
Take a moderation action against a user.

```bash
nself plugin moderation action \
  --user user_123 \
  --type warn \
  --reason "Violated community guidelines"

nself plugin moderation action \
  --user user_456 \
  --type mute \
  --reason "Spam" \
  --duration 1440
```

**Options:**
- `--user <user_id>` - Target user ID (required)
- `--type <type>` - Action type: warn, mute, kick, ban (required)
- `--reason <reason>` - Reason for action (required)
- `--duration <minutes>` - Duration in minutes (for mute/ban)

### Statistics Commands

#### `stats`
View moderation statistics.

```bash
# Overview stats
nself plugin moderation stats
nself plugin moderation stats --days 7

# User-specific stats
nself plugin moderation stats --user user_123
```

**Options:**
- `--user <user_id>` - View stats for a specific user
- `--days <days>` - Number of days for overview (default: 30)

### Maintenance Commands

#### `cleanup`
Clean up expired moderation actions.

```bash
nself plugin moderation cleanup
```

Expires and deactivates temporary mutes and bans that have reached their expiration time.

## REST API

### Health Check Endpoints

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "moderation",
  "timestamp": "2024-02-11T10:00:00Z"
}
```

#### `GET /ready`
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "moderation",
  "timestamp": "2024-02-11T10:00:00Z"
}
```

### Content Analysis Endpoints

#### `POST /api/moderation/analyze`
Analyze content for moderation issues.

**Request:**
```json
{
  "content": "This is some content to check",
  "content_type": "message",
  "channel_id": "channel_123"
}
```

**Response:**
```json
{
  "is_safe": false,
  "toxicity_score": 0.85,
  "matched_rules": [
    {
      "rule_id": "rule-uuid",
      "rule_name": "Profanity Filter",
      "severity": "high",
      "matched_words": ["badword1", "badword2"]
    }
  ],
  "suggested_actions": [
    {
      "type": "delete",
      "reason": "Matched rule: Profanity Filter"
    },
    {
      "type": "warn",
      "reason": "Matched rule: Profanity Filter"
    }
  ]
}
```

#### `POST /api/moderation/check-profanity`
Check content for profanity matches.

**Request:**
```json
{
  "content": "Text to check for profanity",
  "language": "en"
}
```

**Response:**
```json
{
  "contains_profanity": true,
  "matched_words": ["badword1", "badword2"],
  "severity": "high"
}
```

### Action Endpoints

#### `POST /api/moderation/actions`
Create a moderation action.

**Request:**
```json
{
  "user_id": "user_123",
  "action_type": "mute",
  "reason": "Spamming chat",
  "severity": "medium",
  "duration_minutes": 1440,
  "target_message_id": "msg_456",
  "target_channel_id": "channel_789",
  "moderator_id": "mod_111",
  "moderator_notes": "First offense, temporary mute",
  "is_automated": false
}
```

**Response (201):**
```json
{
  "action_id": "action-uuid",
  "expires_at": "2024-02-12T10:00:00Z"
}
```

#### `GET /api/moderation/actions/:user_id`
List all actions for a user.

**Response:**
```json
{
  "actions": [
    {
      "id": "action-uuid",
      "action_type": "mute",
      "severity": "medium",
      "reason": "Spamming chat",
      "created_at": "2024-02-11T10:00:00Z",
      "expires_at": "2024-02-12T10:00:00Z",
      "is_active": true,
      "is_automated": false,
      "moderator_id": "mod_111"
    }
  ]
}
```

#### `DELETE /api/moderation/actions/:action_id`
Revoke a moderation action.

**Request:**
```json
{
  "revoke_reason": "Appeal approved",
  "revoked_by": "mod_222"
}
```

**Response:**
```json
{
  "success": true
}
```

### Flag & Queue Endpoints

#### `POST /api/moderation/flags`
Create a flag for content review.

**Request:**
```json
{
  "content_type": "message",
  "content_id": "msg_123",
  "content_snapshot": {
    "text": "Flagged message content",
    "author_id": "user_456"
  },
  "flag_reason": "Contains hate speech",
  "flag_category": "hate_speech",
  "severity": "high",
  "flagged_by_user_id": "user_789",
  "is_automated": false
}
```

**Response (201):**
```json
{
  "flag_id": "flag-uuid"
}
```

#### `GET /api/moderation/queue`
Get the moderation review queue.

**Query Parameters:**
- `status` - Filter by status (default: pending)
- `severity` - Filter by severity
- `limit` - Result limit (default: 50)
- `offset` - Offset for pagination (default: 0)

**Response:**
```json
{
  "flags": [
    {
      "id": "flag-uuid",
      "content_type": "message",
      "content_id": "msg_123",
      "flag_reason": "Contains hate speech",
      "flag_category": "hate_speech",
      "severity": "high",
      "status": "pending",
      "is_automated": false,
      "created_at": "2024-02-11T10:00:00Z"
    }
  ],
  "total": 42
}
```

#### `POST /api/moderation/flags/:flag_id/review`
Review a flagged item.

**Request:**
```json
{
  "status": "approved",
  "reviewed_by": "mod_123",
  "review_notes": "Confirmed violation, taking action",
  "action": {
    "type": "warn",
    "duration_minutes": null
  }
}
```

**Response:**
```json
{
  "success": true,
  "action_id": "action-uuid"
}
```

### Report Endpoints

#### `POST /api/moderation/reports`
Submit a user report.

**Request:**
```json
{
  "reporter_id": "user_123",
  "content_type": "message",
  "content_id": "msg_456",
  "report_category": "harassment",
  "report_reason": "User is harassing me in chat",
  "additional_context": "This has been happening for several days"
}
```

**Response (201):**
```json
{
  "report_id": "report-uuid"
}
```

#### `GET /api/moderation/reports`
List user reports.

**Query Parameters:**
- `status` - Filter by status
- `limit` - Result limit (default: 50)
- `offset` - Offset for pagination (default: 0)

**Response:**
```json
{
  "reports": [
    {
      "id": "report-uuid",
      "reporter_id": "user_123",
      "content_type": "message",
      "content_id": "msg_456",
      "report_category": "harassment",
      "report_reason": "User is harassing me in chat",
      "status": "pending",
      "created_at": "2024-02-11T10:00:00Z"
    }
  ],
  "total": 15
}
```

### Appeal Endpoints

#### `POST /api/moderation/appeals`
Submit an appeal for a moderation action.

**Request:**
```json
{
  "action_id": "action-uuid",
  "appellant_user_id": "user_123",
  "appeal_reason": "I believe this action was a mistake. I was not spamming, just excited about the news.",
  "supporting_evidence": {
    "context": "Previous messages show legitimate conversation",
    "screenshots": ["url1", "url2"]
  }
}
```

**Response (201):**
```json
{
  "appeal_id": "appeal-uuid"
}
```

#### `GET /api/moderation/appeals`
List appeals.

**Query Parameters:**
- `status` - Filter by status (pending, approved, rejected)
- `limit` - Result limit (default: 50)

**Response:**
```json
{
  "appeals": [
    {
      "id": "appeal-uuid",
      "action_id": "action-uuid",
      "appellant_user_id": "user_123",
      "appeal_reason": "I believe this action was a mistake...",
      "status": "pending",
      "created_at": "2024-02-11T10:00:00Z"
    }
  ]
}
```

#### `POST /api/moderation/appeals/:appeal_id/review`
Review an appeal.

**Request:**
```json
{
  "status": "approved",
  "reviewed_by": "mod_456",
  "review_decision": "Appeal granted. User's explanation is reasonable and previous behavior shows good standing."
}
```

**Response:**
```json
{
  "success": true,
  "action_revoked": true
}
```

### Rule & Wordlist Endpoints

#### `GET /api/moderation/rules`
List all moderation rules.

**Response:**
```json
{
  "rules": [
    {
      "id": "rule-uuid",
      "name": "Profanity Filter",
      "description": "Blocks messages containing profanity",
      "filter_type": "profanity",
      "severity": "high",
      "is_enabled": true,
      "conditions": {
        "wordlist_ids": ["wordlist-uuid"]
      },
      "actions": [
        {"type": "delete"},
        {"type": "warn"}
      ]
    }
  ]
}
```

#### `POST /api/moderation/rules`
Create a moderation rule.

**Request:**
```json
{
  "name": "Spam Filter",
  "description": "Detects and removes spam messages",
  "filter_type": "spam",
  "severity": "medium",
  "conditions": {
    "pattern": "http.*(viagra|cialis)",
    "regex": true
  },
  "actions": [
    {"type": "delete"},
    {"type": "mute", "duration_minutes": 60}
  ],
  "threshold_config": {
    "min_score": 0.7
  }
}
```

**Response (201):**
```json
{
  "rule_id": "rule-uuid"
}
```

#### `PATCH /api/moderation/rules/:rule_id`
Update a moderation rule.

**Request:**
```json
{
  "is_enabled": false,
  "severity": "low"
}
```

**Response:**
```json
{
  "success": true
}
```

#### `DELETE /api/moderation/rules/:rule_id`
Delete a moderation rule.

**Response:**
```json
{
  "deleted": true
}
```

#### `GET /api/moderation/wordlists`
List all wordlists.

**Response:**
```json
{
  "wordlists": [
    {
      "id": "wordlist-uuid",
      "name": "English Profanity",
      "description": "Common English profanity words",
      "language": "en",
      "category": "profanity",
      "words": ["word1", "word2", "word3"],
      "is_regex": false,
      "case_sensitive": false,
      "is_enabled": true,
      "severity": "high"
    }
  ]
}
```

#### `POST /api/moderation/wordlists`
Create a wordlist.

**Request:**
```json
{
  "name": "Custom Banned Words",
  "description": "Community-specific banned words",
  "language": "en",
  "category": "profanity",
  "words": ["badword1", "badword2", "badword3"],
  "is_regex": false,
  "case_sensitive": false,
  "severity": "high"
}
```

**Response (201):**
```json
{
  "wordlist_id": "wordlist-uuid"
}
```

#### `PATCH /api/moderation/wordlists/:wordlist_id`
Update a wordlist.

**Request:**
```json
{
  "words": ["word1", "word2", "word3", "word4"],
  "is_enabled": true
}
```

**Response:**
```json
{
  "success": true
}
```

#### `DELETE /api/moderation/wordlists/:wordlist_id`
Delete a wordlist.

**Response:**
```json
{
  "deleted": true
}
```

### Statistics Endpoints

#### `GET /api/moderation/stats/user/:user_id`
Get moderation stats for a specific user.

**Response:**
```json
{
  "total_warnings": 2,
  "total_mutes": 1,
  "total_bans": 0,
  "total_flags": 3,
  "risk_level": "medium",
  "risk_score": 35.5,
  "average_toxicity_score": 0.65,
  "is_muted": false,
  "muted_until": null,
  "is_banned": false,
  "banned_until": null
}
```

#### `GET /api/moderation/stats/overview`
Get overview statistics.

**Query Parameters:**
- `timeframe` - Time period: day, week, month (default: month)

**Response:**
```json
{
  "total_actions": 156,
  "total_flags": 42,
  "total_reports": 28,
  "actions_by_type": {
    "warn": 89,
    "mute": 45,
    "ban": 12,
    "kick": 10
  },
  "flags_by_severity": {
    "low": 8,
    "medium": 20,
    "high": 10,
    "critical": 4
  },
  "average_toxicity_score": 0.42
}
```

### Audit Log Endpoint

#### `GET /api/moderation/audit-log`
List audit log entries.

**Query Parameters:**
- `event_type` - Filter by event type
- `event_category` - Filter by category (action, flag, appeal, config)
- `actor_id` - Filter by actor
- `limit` - Result limit (default: 50)
- `offset` - Offset for pagination (default: 0)

**Response:**
```json
{
  "logs": [
    {
      "id": "log-uuid",
      "event_type": "action.created",
      "event_category": "action",
      "actor_id": "mod_123",
      "actor_type": "user",
      "target_type": "user",
      "target_id": "user_456",
      "details": {
        "action_type": "mute",
        "reason": "Spamming",
        "action_id": "action-uuid"
      },
      "created_at": "2024-02-11T10:00:00Z"
    }
  ],
  "total": 1500
}
```

### Maintenance Endpoint

#### `POST /api/moderation/cleanup/expired`
Manually trigger cleanup of expired actions.

**Response:**
```json
{
  "expired_count": 15
}
```

## Webhook Events

The Moderation plugin emits webhook events for all moderation activities:

### Action Events

| Event | Description | Payload |
|-------|-------------|---------|
| `action.created` | New moderation action taken | `{action_id, user_id, action_type, reason, expires_at}` |
| `action.revoked` | Moderation action revoked | `{action_id, revoke_reason, revoked_by}` |

### Flag Events

| Event | Description | Payload |
|-------|-------------|---------|
| `flag.created` | Content flagged for review | `{flag_id, content_type, content_id, severity}` |
| `flag.reviewed` | Flagged content reviewed | `{flag_id, status, reviewed_by, action_taken}` |

### Appeal Events

| Event | Description | Payload |
|-------|-------------|---------|
| `appeal.created` | New appeal submitted | `{appeal_id, action_id, appellant_user_id}` |
| `appeal.reviewed` | Appeal reviewed | `{appeal_id, status, was_successful}` |

### Report Events

| Event | Description | Payload |
|-------|-------------|---------|
| `report.created` | New user report submitted | `{report_id, reporter_id, content_type, content_id}` |

### Rule Events

| Event | Description | Payload |
|-------|-------------|---------|
| `rule.created` | New moderation rule created | `{rule_id, name, filter_type}` |
| `rule.updated` | Moderation rule updated | `{rule_id, changes}` |

## Database Schema

### moderation_rules

Moderation rules for automated content filtering.

```sql
CREATE TABLE IF NOT EXISTS moderation_rules (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  filter_type VARCHAR(50) NOT NULL,
  severity VARCHAR(20) NOT NULL DEFAULT 'medium',
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  conditions JSONB NOT NULL DEFAULT '{}',
  actions JSONB NOT NULL DEFAULT '[]',
  threshold_config JSONB DEFAULT '{}',
  channel_id VARCHAR(255),
  created_by VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_moderation_rules_account ON moderation_rules(source_account_id);
CREATE INDEX IF NOT EXISTS idx_moderation_rules_enabled ON moderation_rules(source_account_id, is_enabled) WHERE is_enabled = true;
CREATE INDEX IF NOT EXISTS idx_moderation_rules_type ON moderation_rules(source_account_id, filter_type, severity);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| name | VARCHAR(255) | No | - | Rule name |
| description | TEXT | Yes | - | Rule description |
| filter_type | VARCHAR(50) | No | - | Type: profanity, toxicity, spam, caps, links |
| severity | VARCHAR(20) | No | 'medium' | Severity: low, medium, high, critical |
| is_enabled | BOOLEAN | No | true | Whether rule is active |
| conditions | JSONB | No | {} | Rule conditions (patterns, thresholds, etc.) |
| actions | JSONB | No | [] | Actions to take when rule matches |
| threshold_config | JSONB | No | {} | Threshold configuration |
| channel_id | VARCHAR(255) | Yes | - | Limit rule to specific channel |
| created_by | VARCHAR(255) | Yes | - | User who created rule |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### moderation_wordlists

Profanity and banned word lists.

```sql
CREATE TABLE IF NOT EXISTS moderation_wordlists (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  language VARCHAR(10) NOT NULL DEFAULT 'en',
  category VARCHAR(100),
  words TEXT[] NOT NULL DEFAULT '{}',
  is_regex BOOLEAN NOT NULL DEFAULT false,
  case_sensitive BOOLEAN NOT NULL DEFAULT false,
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  severity VARCHAR(20) NOT NULL DEFAULT 'medium',
  created_by VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX IF NOT EXISTS idx_moderation_wordlists_account ON moderation_wordlists(source_account_id);
CREATE INDEX IF NOT EXISTS idx_moderation_wordlists_enabled ON moderation_wordlists(source_account_id, is_enabled) WHERE is_enabled = true;
CREATE INDEX IF NOT EXISTS idx_moderation_wordlists_language ON moderation_wordlists(source_account_id, language);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| name | VARCHAR(255) | No | - | Wordlist name |
| description | TEXT | Yes | - | Wordlist description |
| language | VARCHAR(10) | No | 'en' | Language code (en, es, fr, etc.) |
| category | VARCHAR(100) | Yes | - | Category: profanity, slurs, spam |
| words | TEXT[] | No | {} | Array of words/patterns |
| is_regex | BOOLEAN | No | false | Whether words are regex patterns |
| case_sensitive | BOOLEAN | No | false | Case-sensitive matching |
| is_enabled | BOOLEAN | No | true | Whether wordlist is active |
| severity | VARCHAR(20) | No | 'medium' | Severity: low, medium, high, critical |
| created_by | VARCHAR(255) | Yes | - | User who created wordlist |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### moderation_actions

Moderation actions taken against users.

```sql
CREATE TABLE IF NOT EXISTS moderation_actions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  target_user_id VARCHAR(255) NOT NULL,
  target_message_id VARCHAR(255),
  target_channel_id VARCHAR(255),
  action_type VARCHAR(50) NOT NULL,
  severity VARCHAR(20) NOT NULL,
  reason TEXT NOT NULL,
  duration_minutes INTEGER,
  expires_at TIMESTAMP WITH TIME ZONE,
  triggered_by_rule_id UUID REFERENCES moderation_rules(id) ON DELETE SET NULL,
  is_automated BOOLEAN NOT NULL DEFAULT false,
  moderator_id VARCHAR(255),
  moderator_notes TEXT,
  metadata JSONB DEFAULT '{}',
  is_active BOOLEAN NOT NULL DEFAULT true,
  revoked_at TIMESTAMP WITH TIME ZONE,
  revoked_by VARCHAR(255),
  revoke_reason TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_moderation_actions_account ON moderation_actions(source_account_id);
CREATE INDEX IF NOT EXISTS idx_moderation_actions_user ON moderation_actions(source_account_id, target_user_id);
CREATE INDEX IF NOT EXISTS idx_moderation_actions_message ON moderation_actions(target_message_id) WHERE target_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_moderation_actions_moderator ON moderation_actions(moderator_id) WHERE moderator_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_moderation_actions_expires ON moderation_actions(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_moderation_actions_active ON moderation_actions(source_account_id, is_active, created_at);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| target_user_id | VARCHAR(255) | No | - | User receiving action |
| target_message_id | VARCHAR(255) | Yes | - | Related message ID |
| target_channel_id | VARCHAR(255) | Yes | - | Related channel ID |
| action_type | VARCHAR(50) | No | - | Type: warn, mute, kick, ban, delete |
| severity | VARCHAR(20) | No | - | Severity: low, medium, high, critical |
| reason | TEXT | No | - | Reason for action |
| duration_minutes | INTEGER | Yes | - | Duration for temporary actions |
| expires_at | TIMESTAMP WITH TIME ZONE | Yes | - | Expiration timestamp |
| triggered_by_rule_id | UUID | Yes | - | Rule that triggered action (if automated) |
| is_automated | BOOLEAN | No | false | Whether action was automated |
| moderator_id | VARCHAR(255) | Yes | - | Moderator who took action |
| moderator_notes | TEXT | Yes | - | Internal moderator notes |
| metadata | JSONB | No | {} | Additional metadata |
| is_active | BOOLEAN | No | true | Whether action is still active |
| revoked_at | TIMESTAMP WITH TIME ZONE | Yes | - | When action was revoked |
| revoked_by | VARCHAR(255) | Yes | - | Who revoked the action |
| revoke_reason | TEXT | Yes | - | Reason for revoking |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Action timestamp |

### moderation_flags

Content flagged for manual review.

```sql
CREATE TABLE IF NOT EXISTS moderation_flags (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  content_type VARCHAR(50) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_snapshot JSONB,
  flag_reason VARCHAR(255) NOT NULL,
  flag_category VARCHAR(100),
  severity VARCHAR(20) NOT NULL DEFAULT 'medium',
  flagged_by_user_id VARCHAR(255),
  flagged_by_rule_id UUID REFERENCES moderation_rules(id) ON DELETE SET NULL,
  is_automated BOOLEAN NOT NULL DEFAULT false,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  reviewed_by VARCHAR(255),
  reviewed_at TIMESTAMP WITH TIME ZONE,
  review_notes TEXT,
  action_id UUID REFERENCES moderation_actions(id) ON DELETE SET NULL,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_moderation_flags_account ON moderation_flags(source_account_id);
CREATE INDEX IF NOT EXISTS idx_moderation_flags_content ON moderation_flags(source_account_id, content_type, content_id);
CREATE INDEX IF NOT EXISTS idx_moderation_flags_status ON moderation_flags(source_account_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_moderation_flags_severity ON moderation_flags(source_account_id, severity, status);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| content_type | VARCHAR(50) | No | - | Type: message, post, profile, etc. |
| content_id | VARCHAR(255) | No | - | Content identifier |
| content_snapshot | JSONB | Yes | - | Snapshot of flagged content |
| flag_reason | VARCHAR(255) | No | - | Reason for flag |
| flag_category | VARCHAR(100) | Yes | - | Category: hate_speech, harassment, spam, etc. |
| severity | VARCHAR(20) | No | 'medium' | Severity: low, medium, high, critical |
| flagged_by_user_id | VARCHAR(255) | Yes | - | User who flagged (if manual) |
| flagged_by_rule_id | UUID | Yes | - | Rule that flagged (if automated) |
| is_automated | BOOLEAN | No | false | Whether flag was automated |
| status | VARCHAR(20) | No | 'pending' | Status: pending, approved, rejected |
| reviewed_by | VARCHAR(255) | Yes | - | Moderator who reviewed |
| reviewed_at | TIMESTAMP WITH TIME ZONE | Yes | - | Review timestamp |
| review_notes | TEXT | Yes | - | Review notes |
| action_id | UUID | Yes | - | Related moderation action |
| metadata | JSONB | No | {} | Additional metadata |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Flag timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### moderation_appeals

Appeals of moderation actions.

```sql
CREATE TABLE IF NOT EXISTS moderation_appeals (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  action_id UUID NOT NULL REFERENCES moderation_actions(id) ON DELETE CASCADE,
  appellant_user_id VARCHAR(255) NOT NULL,
  appeal_reason TEXT NOT NULL,
  supporting_evidence JSONB DEFAULT '{}',
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  reviewed_by VARCHAR(255),
  reviewed_at TIMESTAMP WITH TIME ZONE,
  review_decision TEXT,
  was_successful BOOLEAN,
  new_action_id UUID REFERENCES moderation_actions(id) ON DELETE SET NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_moderation_appeals_account ON moderation_appeals(source_account_id);
CREATE INDEX IF NOT EXISTS idx_moderation_appeals_action ON moderation_appeals(action_id);
CREATE INDEX IF NOT EXISTS idx_moderation_appeals_user ON moderation_appeals(source_account_id, appellant_user_id);
CREATE INDEX IF NOT EXISTS idx_moderation_appeals_status ON moderation_appeals(source_account_id, status, created_at);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| action_id | UUID | No | - | Moderation action being appealed |
| appellant_user_id | VARCHAR(255) | No | - | User submitting appeal |
| appeal_reason | TEXT | No | - | Reason for appeal |
| supporting_evidence | JSONB | No | {} | Evidence supporting appeal |
| status | VARCHAR(20) | No | 'pending' | Status: pending, approved, rejected |
| reviewed_by | VARCHAR(255) | Yes | - | Moderator who reviewed appeal |
| reviewed_at | TIMESTAMP WITH TIME ZONE | Yes | - | Review timestamp |
| review_decision | TEXT | Yes | - | Decision explanation |
| was_successful | BOOLEAN | Yes | - | Whether appeal was granted |
| new_action_id | UUID | Yes | - | New action if modified |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Appeal timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### moderation_reports

User-submitted reports of content violations.

```sql
CREATE TABLE IF NOT EXISTS moderation_reports (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  reporter_id VARCHAR(255) NOT NULL,
  content_type VARCHAR(50) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  report_category VARCHAR(100) NOT NULL,
  report_reason TEXT NOT NULL,
  additional_context TEXT,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  assigned_to VARCHAR(255),
  flag_id UUID REFERENCES moderation_flags(id) ON DELETE SET NULL,
  action_id UUID REFERENCES moderation_actions(id) ON DELETE SET NULL,
  resolution_notes TEXT,
  resolved_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_moderation_reports_account ON moderation_reports(source_account_id);
CREATE INDEX IF NOT EXISTS idx_moderation_reports_reporter ON moderation_reports(source_account_id, reporter_id);
CREATE INDEX IF NOT EXISTS idx_moderation_reports_content ON moderation_reports(source_account_id, content_type, content_id);
CREATE INDEX IF NOT EXISTS idx_moderation_reports_status ON moderation_reports(source_account_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_moderation_reports_assigned ON moderation_reports(assigned_to) WHERE assigned_to IS NOT NULL;
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| reporter_id | VARCHAR(255) | No | - | User submitting report |
| content_type | VARCHAR(50) | No | - | Type of reported content |
| content_id | VARCHAR(255) | No | - | Content identifier |
| report_category | VARCHAR(100) | No | - | Category: harassment, spam, hate_speech, etc. |
| report_reason | TEXT | No | - | Report explanation |
| additional_context | TEXT | Yes | - | Additional context |
| status | VARCHAR(20) | No | 'pending' | Status: pending, investigating, resolved, dismissed |
| assigned_to | VARCHAR(255) | Yes | - | Assigned moderator |
| flag_id | UUID | Yes | - | Related flag if created |
| action_id | UUID | Yes | - | Related action if taken |
| resolution_notes | TEXT | Yes | - | Resolution notes |
| resolved_at | TIMESTAMP WITH TIME ZONE | Yes | - | Resolution timestamp |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Report timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### moderation_toxicity_scores

AI toxicity detection scores.

```sql
CREATE TABLE IF NOT EXISTS moderation_toxicity_scores (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  content_type VARCHAR(50) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_hash VARCHAR(64),
  overall_score DECIMAL(5,4) NOT NULL,
  category_scores JSONB DEFAULT '{}',
  provider VARCHAR(50) NOT NULL,
  model_version VARCHAR(50),
  analyzed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  metadata JSONB DEFAULT '{}',
  UNIQUE(source_account_id, content_type, content_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_toxicity_account ON moderation_toxicity_scores(source_account_id);
CREATE INDEX IF NOT EXISTS idx_toxicity_content ON moderation_toxicity_scores(source_account_id, content_type, content_id);
CREATE INDEX IF NOT EXISTS idx_toxicity_score ON moderation_toxicity_scores(source_account_id, overall_score);
CREATE INDEX IF NOT EXISTS idx_toxicity_hash ON moderation_toxicity_scores(content_hash) WHERE content_hash IS NOT NULL;
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| content_type | VARCHAR(50) | No | - | Type of content analyzed |
| content_id | VARCHAR(255) | No | - | Content identifier |
| content_hash | VARCHAR(64) | Yes | - | Hash for caching |
| overall_score | DECIMAL(5,4) | No | - | Overall toxicity score (0.0-1.0) |
| category_scores | JSONB | No | {} | Scores by category (hate, harassment, etc.) |
| provider | VARCHAR(50) | No | - | Provider: local, perspective_api, openai |
| model_version | VARCHAR(50) | Yes | - | Model version used |
| analyzed_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Analysis timestamp |
| metadata | JSONB | No | {} | Additional metadata |

### moderation_user_stats

Per-user moderation statistics and risk tracking.

```sql
CREATE TABLE IF NOT EXISTS moderation_user_stats (
  user_id VARCHAR(255) NOT NULL,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  total_warnings INTEGER NOT NULL DEFAULT 0,
  total_mutes INTEGER NOT NULL DEFAULT 0,
  total_bans INTEGER NOT NULL DEFAULT 0,
  total_flags INTEGER NOT NULL DEFAULT 0,
  total_reports_filed INTEGER NOT NULL DEFAULT 0,
  total_reports_against INTEGER NOT NULL DEFAULT 0,
  average_toxicity_score DECIMAL(5,4),
  toxicity_trend DECIMAL(5,4),
  risk_level VARCHAR(20) NOT NULL DEFAULT 'low',
  risk_score DECIMAL(5,2) DEFAULT 0.0,
  is_muted BOOLEAN NOT NULL DEFAULT false,
  muted_until TIMESTAMP WITH TIME ZONE,
  is_banned BOOLEAN NOT NULL DEFAULT false,
  banned_until TIMESTAMP WITH TIME ZONE,
  first_violation_at TIMESTAMP WITH TIME ZONE,
  last_violation_at TIMESTAMP WITH TIME ZONE,
  last_calculated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  PRIMARY KEY (source_account_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_stats_account ON moderation_user_stats(source_account_id);
CREATE INDEX IF NOT EXISTS idx_user_stats_risk ON moderation_user_stats(source_account_id, risk_level, risk_score);
CREATE INDEX IF NOT EXISTS idx_user_stats_muted ON moderation_user_stats(source_account_id, is_muted, muted_until) WHERE is_muted = true;
CREATE INDEX IF NOT EXISTS idx_user_stats_banned ON moderation_user_stats(source_account_id, is_banned, banned_until) WHERE is_banned = true;
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| user_id | VARCHAR(255) | No | - | User identifier |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| total_warnings | INTEGER | No | 0 | Total warnings received |
| total_mutes | INTEGER | No | 0 | Total mutes received |
| total_bans | INTEGER | No | 0 | Total bans received |
| total_flags | INTEGER | No | 0 | Total flags on user's content |
| total_reports_filed | INTEGER | No | 0 | Reports filed by user |
| total_reports_against | INTEGER | No | 0 | Reports filed against user |
| average_toxicity_score | DECIMAL(5,4) | Yes | - | Average toxicity score |
| toxicity_trend | DECIMAL(5,4) | Yes | - | Toxicity trend |
| risk_level | VARCHAR(20) | No | 'low' | Risk level: low, medium, high, critical |
| risk_score | DECIMAL(5,2) | No | 0.0 | Calculated risk score (0-100) |
| is_muted | BOOLEAN | No | false | Currently muted |
| muted_until | TIMESTAMP WITH TIME ZONE | Yes | - | Mute expiration |
| is_banned | BOOLEAN | No | false | Currently banned |
| banned_until | TIMESTAMP WITH TIME ZONE | Yes | - | Ban expiration |
| first_violation_at | TIMESTAMP WITH TIME ZONE | Yes | - | First violation timestamp |
| last_violation_at | TIMESTAMP WITH TIME ZONE | Yes | - | Last violation timestamp |
| last_calculated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last calculation timestamp |

### moderation_audit_log

Audit trail of all moderation activities.

```sql
CREATE TABLE IF NOT EXISTS moderation_audit_log (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(100) NOT NULL,
  event_category VARCHAR(50) NOT NULL,
  actor_id VARCHAR(255),
  actor_type VARCHAR(50) NOT NULL DEFAULT 'user',
  target_type VARCHAR(50),
  target_id VARCHAR(255),
  details JSONB NOT NULL DEFAULT '{}',
  ip_address VARCHAR(45),
  user_agent TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_account ON moderation_audit_log(source_account_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_event ON moderation_audit_log(source_account_id, event_type, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON moderation_audit_log(source_account_id, actor_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_target ON moderation_audit_log(source_account_id, target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON moderation_audit_log(source_account_id, created_at DESC);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| event_type | VARCHAR(100) | No | - | Event type (action.created, flag.reviewed, etc.) |
| event_category | VARCHAR(50) | No | - | Category: action, flag, appeal, config |
| actor_id | VARCHAR(255) | Yes | - | User/moderator performing action |
| actor_type | VARCHAR(50) | No | 'user' | Actor type: user, automation, system |
| target_type | VARCHAR(50) | Yes | - | Target type: user, message, rule, etc. |
| target_id | VARCHAR(255) | Yes | - | Target identifier |
| details | JSONB | No | {} | Event details |
| ip_address | VARCHAR(45) | Yes | - | IP address |
| user_agent | TEXT | Yes | - | User agent |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Event timestamp |

## Examples

### Example 1: Set Up Profanity Filtering

```bash
# 1. Create a wordlist
curl -X POST http://localhost:3704/api/moderation/wordlists \
  -H "Content-Type: application/json" \
  -d '{
    "name": "English Profanity",
    "language": "en",
    "category": "profanity",
    "words": ["badword1", "badword2", "badword3"],
    "severity": "high",
    "is_regex": false,
    "case_sensitive": false
  }'

# 2. Create a moderation rule
curl -X POST http://localhost:3704/api/moderation/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Profanity Filter",
    "description": "Auto-delete messages with profanity",
    "filter_type": "profanity",
    "severity": "high",
    "conditions": {
      "wordlist_ids": ["wordlist-uuid"]
    },
    "actions": [
      {"type": "delete"},
      {"type": "warn"}
    ]
  }'

# 3. Test content
nself plugin moderation analyze --content "This has a badword1 in it"
```

### Example 2: Manual Content Review Workflow

```sql
-- View pending flags in queue
SELECT
  id, content_type, content_id, flag_reason, severity, created_at
FROM moderation_flags
WHERE source_account_id = 'primary'
  AND status = 'pending'
ORDER BY severity DESC, created_at ASC
LIMIT 20;

-- Review a flag and take action
-- Via API:
curl -X POST http://localhost:3704/api/moderation/flags/{flag-uuid}/review \
  -H "Content-Type: application/json" \
  -d '{
    "status": "approved",
    "reviewed_by": "mod_123",
    "review_notes": "Confirmed harassment, muting user",
    "action": {
      "type": "mute",
      "duration_minutes": 1440
    }
  }'
```

### Example 3: User Reports and Flags

```bash
# User submits a report
curl -X POST http://localhost:3704/api/moderation/reports \
  -H "Content-Type: application/json" \
  -d '{
    "reporter_id": "user_123",
    "content_type": "message",
    "content_id": "msg_456",
    "report_category": "harassment",
    "report_reason": "This user has been harassing me repeatedly",
    "additional_context": "This is the third time this week"
  }'

# System automatically creates flag for moderator review
# Moderator reviews in queue
nself plugin moderation queue --severity high --limit 10
```

### Example 4: Appeals Process

```bash
# 1. User receives mute action
curl -X POST http://localhost:3704/api/moderation/actions \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_789",
    "action_type": "mute",
    "reason": "Spamming chat",
    "duration_minutes": 1440,
    "moderator_id": "mod_111"
  }'

# 2. User submits appeal
curl -X POST http://localhost:3704/api/moderation/appeals \
  -H "Content-Type: application/json" \
  -d '{
    "action_id": "action-uuid",
    "appellant_user_id": "user_789",
    "appeal_reason": "I was not spamming, just excited about the news. I will be more careful with message frequency.",
    "supporting_evidence": {
      "context": "Previous messages show legitimate conversation"
    }
  }'

# 3. Moderator reviews appeal
curl -X POST http://localhost:3704/api/moderation/appeals/{appeal-uuid}/review \
  -H "Content-Type: application/json" \
  -d '{
    "status": "approved",
    "reviewed_by": "mod_222",
    "review_decision": "Appeal granted. User has good history and explanation is reasonable. Revoking mute."
  }'
```

### Example 5: User Risk Tracking

```sql
-- Calculate risk scores for all users with violations
UPDATE moderation_user_stats
SET risk_score = (
  SELECT LEAST(100, (
    -- Recent violations (last 30 days)
    (SELECT COUNT(*) * 5 FROM moderation_actions
     WHERE target_user_id = moderation_user_stats.user_id
       AND created_at > NOW() - INTERVAL '30 days') +
    -- Average toxicity
    (COALESCE((SELECT AVG(overall_score) * 50 FROM moderation_toxicity_scores
              WHERE content_type = 'message'
                AND content_id IN (SELECT target_message_id FROM moderation_actions
                                  WHERE target_user_id = moderation_user_stats.user_id)), 0))
  ))
),
risk_level = CASE
  WHEN risk_score >= 75 THEN 'critical'
  WHEN risk_score >= 50 THEN 'high'
  WHEN risk_score >= 25 THEN 'medium'
  ELSE 'low'
END;

-- Find high-risk users
SELECT user_id, risk_level, risk_score,
       total_warnings, total_mutes, total_bans,
       average_toxicity_score
FROM moderation_user_stats
WHERE source_account_id = 'primary'
  AND risk_level IN ('high', 'critical')
ORDER BY risk_score DESC;
```

## Troubleshooting

### Profanity Not Being Detected

**Problem**: Content with banned words is not being flagged.

**Solution:**
```sql
-- Check wordlists are enabled
SELECT id, name, is_enabled, words
FROM moderation_wordlists
WHERE source_account_id = 'primary';

-- Check rules are enabled
SELECT id, name, is_enabled, filter_type
FROM moderation_rules
WHERE source_account_id = 'primary'
  AND filter_type = 'profanity';

-- Test profanity detection directly
```

```bash
nself plugin moderation analyze --content "test content with badword"
```

### Expired Actions Not Cleaning Up

**Problem**: Mutes and bans that should have expired are still active.

**Solution:**
```bash
# Manual cleanup
nself plugin moderation cleanup

# Check cleanup is enabled
echo $MODERATION_CLEANUP_ENABLED  # should be true

# Set up automated cleanup via cron
echo "0 * * * * nself plugin moderation cleanup" | crontab -
```

```sql
-- Manually expire old actions
UPDATE moderation_user_stats
SET is_muted = false, muted_until = NULL
WHERE is_muted = true
  AND muted_until < NOW();

UPDATE moderation_user_stats
SET is_banned = false, banned_until = NULL
WHERE is_banned = true
  AND banned_until < NOW();
```

### Too Many False Positives

**Problem**: Legitimate content is being flagged/deleted.

**Solution:**
```sql
-- Lower sensitivity thresholds
UPDATE moderation_rules
SET threshold_config = jsonb_set(threshold_config, '{min_score}', '0.9')
WHERE filter_type = 'toxicity';

-- Disable overly aggressive rules
UPDATE moderation_rules
SET is_enabled = false
WHERE name = 'Aggressive Spam Filter';

-- Review false positive patterns
SELECT flag_reason, COUNT(*) as count
FROM moderation_flags
WHERE status = 'rejected'
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY flag_reason
ORDER BY count DESC;
```

### Queue Overload

**Problem**: Moderation queue has too many pending items.

**Solution:**
```sql
-- Prioritize by severity
SELECT COUNT(*), severity
FROM moderation_flags
WHERE status = 'pending'
GROUP BY severity;

-- Auto-reject low-severity old items
UPDATE moderation_flags
SET status = 'rejected',
    review_notes = 'Auto-rejected due to age',
    reviewed_at = NOW()
WHERE status = 'pending'
  AND severity = 'low'
  AND created_at < NOW() - INTERVAL '30 days';

-- Assign moderators to handle backlog
UPDATE moderation_flags
SET assigned_to = 'mod_team'
WHERE status = 'pending'
  AND severity IN ('high', 'critical');
```

### User Stats Not Updating

**Problem**: User risk scores and statistics are stale.

**Solution:**
```bash
# Recalculate risk scores for specific user
curl -X POST http://localhost:3704/api/moderation/stats/user/user_123/calculate

# Bulk recalculation
```

```sql
-- Recalculate all user stats
UPDATE moderation_user_stats
SET
  total_warnings = (SELECT COUNT(*) FROM moderation_actions
                   WHERE target_user_id = moderation_user_stats.user_id
                     AND action_type = 'warn'),
  total_mutes = (SELECT COUNT(*) FROM moderation_actions
                WHERE target_user_id = moderation_user_stats.user_id
                  AND action_type = 'mute'),
  total_bans = (SELECT COUNT(*) FROM moderation_actions
               WHERE target_user_id = moderation_user_stats.user_id
                 AND action_type = 'ban'),
  last_calculated_at = NOW()
WHERE source_account_id = 'primary';
```

### Appeals Not Working

**Problem**: Users cannot submit appeals or appeals are not being processed.

**Solution:**
```bash
# Check appeals are enabled
echo $MODERATION_APPEALS_ENABLED  # should be true
echo $MODERATION_APPEALS_TIME_LIMIT_DAYS  # default: 7

# Check for expired appeal window
```

```sql
SELECT
  a.id as action_id,
  a.target_user_id,
  a.action_type,
  a.created_at,
  NOW() - a.created_at as age,
  CASE
    WHEN NOW() - a.created_at > INTERVAL '7 days' THEN 'Expired'
    ELSE 'Can Appeal'
  END as appeal_status
FROM moderation_actions a
LEFT JOIN moderation_appeals ap ON ap.action_id = a.id
WHERE a.is_active = true
  AND ap.id IS NULL;
```

### Database Performance Issues

**Problem**: Moderation queries are slow.

**Solution:**
```sql
-- Check index usage
SELECT schemaname, tablename, indexname, idx_scan
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
  AND tablename LIKE 'moderation_%'
ORDER BY idx_scan ASC;

-- Analyze tables
ANALYZE moderation_actions;
ANALYZE moderation_flags;
ANALYZE moderation_user_stats;

-- Archive old audit logs
DELETE FROM moderation_audit_log
WHERE created_at < NOW() - INTERVAL '1 year';

VACUUM FULL moderation_audit_log;
```

---

For additional support, consult the [nself-plugins GitHub repository](https://github.com/acamarata/nself-plugins) or file an issue.
