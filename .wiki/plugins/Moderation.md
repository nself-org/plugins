# Moderation Plugin

Unified content moderation platform with profanity filtering, toxicity detection, AI-powered review, rule-based policies, automated actions, manual review workflows, user strikes, and appeals management.

---

## Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [Database Schema](#database-schema)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Moderation Workflows](#moderation-workflows)
- [Policy Engine](#policy-engine)
- [AI-Powered Moderation](#ai-powered-moderation)
- [User Strikes & Bans](#user-strikes--bans)
- [Appeals Management](#appeals-management)
- [Query Examples](#query-examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Moderation plugin provides a comprehensive, production-ready content moderation solution that combines multiple detection methods, automated decision-making, and manual review workflows. It syncs all moderation data to PostgreSQL and provides real-time webhook notifications.

This is a **unified moderation platform** that consolidates:
- **Profanity filtering** and toxicity detection
- **AI-powered moderation** using OpenAI, Google Vision, and AWS Rekognition
- **Automated content review** with configurable auto-approve/reject thresholds
- **Manual review queue** with SLA tracking and workflow management
- **User strikes and ban management** with automatic threshold enforcement
- **Appeals management** with time-limited appeal windows
- **Rule-based policy engine** for custom content policies
- **Word list management** for profanity and allowed terms
- **Policy evaluation** with detailed violation tracking
- **Automated actions** (warn, mute, ban) based on severity

### Core Capabilities

| Capability | Description |
|------------|-------------|
| **Profanity Detection** | Real-time profanity checking against customizable word lists |
| **Toxicity Analysis** | AI-powered toxicity scoring with configurable thresholds |
| **Content Review** | Automated and manual review workflows with queue management |
| **Policy Enforcement** | Rule-based policy engine with custom rules and actions |
| **User Management** | Strike tracking, automatic bans, and appeal workflows |
| **Audit Trail** | Complete audit log of all moderation actions and decisions |
| **Multi-Provider AI** | Support for OpenAI, Google Vision, and AWS Rekognition |
| **Real-time Webhooks** | Instant notifications for all moderation events |

### Moderation Tables

The plugin creates **17 comprehensive tables**:

| Table | Purpose |
|-------|---------|
| `np_moderation_rules` | Custom moderation rules with conditions and actions |
| `np_moderation_wordlists` | Profanity and allowed word lists |
| `np_moderation_actions` | All moderation actions (warn, mute, ban) taken against users |
| `np_moderation_flags` | Content flagged for review with severity and status |
| `np_moderation_appeals` | User appeals against moderation actions |
| `np_moderation_reports` | User-submitted reports of violating content |
| `np_moderation_toxicity_scores` | AI toxicity analysis results |
| `np_moderation_user_stats` | Per-user violation and action statistics |
| `np_moderation_audit_log` | Complete audit trail of all moderation activities |
| `mod_reviews` | Automated and manual content review records |
| `mod_policies` | Content policies with configured thresholds |
| `mod_user_strikes` | User strike tracking with expiration |
| `cp_policies` | Content policy definitions |
| `cp_rules` | Individual policy rules with matching conditions |
| `cp_evaluations` | Policy evaluation results for content |
| `cp_word_lists` | Word lists for policy matching |
| `cp_overrides` | Manual policy overrides for specific content |
| `np_moderation_webhook_events` | Received webhook event log |

---

## Key Features

### 1. Multi-Layer Detection

- **Profanity Filtering**: Regex-based word list matching with severity levels
- **Toxicity Detection**: AI-powered toxicity scoring (0.0 - 1.0 scale)
- **Image Moderation**: Unsafe content detection in images via Google Vision or AWS Rekognition
- **Text Moderation**: Hate speech, harassment, and self-harm detection via OpenAI
- **Custom Rules**: Create rules based on keywords, patterns, user attributes, or content metadata

### 2. Automated Decision Making

- **Auto-Approve**: Content below configured safety threshold auto-approved
- **Auto-Flag**: Content above threshold flagged for manual review
- **Auto-Reject**: Content above high threshold automatically rejected
- **Smart Routing**: Borderline content routed to manual review queue

### 3. Manual Review Workflows

- **Review Queue**: Centralized queue of flagged content requiring human review
- **Priority Sorting**: Sort by severity, age, report count, or user reputation
- **SLA Tracking**: Monitor review times against configured SLA
- **Batch Actions**: Approve or reject multiple items at once
- **Reviewer Assignment**: Assign specific reviewers to content
- **Review History**: Full audit trail of all review decisions

### 4. User Strike System

- **Strike Accumulation**: Automatic strike assignment on policy violations
- **Strike Expiry**: Strikes expire after configured period (default 30 days)
- **Threshold Actions**: Auto-warn, auto-mute, or auto-ban at strike thresholds
- **Strike History**: Complete history of strikes per user
- **Manual Strikes**: Moderators can manually add or remove strikes

### 5. Appeals Management

- **Appeal Submission**: Users can appeal moderation actions
- **Time-Limited**: Appeals must be submitted within configured window (default 7 days)
- **Appeal Review**: Moderators review and approve/deny appeals
- **Action Reversal**: Approved appeals automatically reverse original action
- **Appeal History**: Track all appeals and outcomes

### 6. Policy Engine

- **Custom Policies**: Define policies with multiple rules and conditions
- **Rule Matching**: Rules match on content, metadata, user attributes, or context
- **Action Assignment**: Policies trigger specific actions (approve, reject, warn, ban)
- **Policy Priority**: Policies evaluated in priority order
- **Override Support**: Manual overrides for specific content or users

### 7. Comprehensive Reporting

- **User Statistics**: Per-user violation counts, strikes, and actions
- **Content Statistics**: Aggregate statistics by content type, severity, and outcome
- **Moderator Performance**: Track review times, decision distribution, and accuracy
- **Trend Analysis**: Identify patterns in violations over time
- **Export Support**: Export reports to CSV or JSON

---

## Quick Start

```bash
# Install the plugin
nself plugin install moderation

# Configure environment
cat >> .env <<EOF
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
MODERATION_PLUGIN_PORT=3704
MODERATION_TOXICITY_ENABLED=true
MODERATION_TOXICITY_PROVIDER=openai
MOD_OPENAI_API_KEY=sk-xxx
MOD_AUTO_APPROVE_BELOW=0.3
MOD_AUTO_REJECT_ABOVE=0.9
EOF

# Initialize database schema
nself plugin moderation init

# Start moderation server
nself plugin moderation server --port 3704

# Analyze content
nself plugin moderation analyze --text "Sample content to moderate"

# Check profanity
nself plugin moderation check-profanity "Test this content"

# View review queue
nself plugin moderation queue list

# Check user status
nself plugin moderation user-status user_12345
```

---

## Installation

### Prerequisites

- **nself CLI** version 0.4.8 or later
- **PostgreSQL** 12+ running and accessible
- **Node.js** 18+ (for TypeScript implementation)
- **API Keys** (optional): OpenAI, Google Cloud, or AWS credentials for AI moderation

### Install Command

```bash
# Install from nself registry
nself plugin install moderation

# Verify installation
nself plugin list | grep moderation
```

### Manual Installation

```bash
# Clone repository
git clone https://github.com/acamarata/nself-plugins.git
cd nself-plugins/plugins/moderation

# Install dependencies (if using TypeScript implementation)
cd ts
npm install

# Build TypeScript
npm run build
```

### Initialize Database

```bash
# Create all moderation tables, indexes, and views
nself plugin moderation init

# Verify tables created
nself plugin moderation status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `MODERATION_PLUGIN_PORT` | No | `3704` | HTTP server port for API and webhooks |
| `MODERATION_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `MODERATION_APP_IDS` | No | - | Comma-separated app IDs for multi-app isolation |
| `MODERATION_LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |
| `MODERATION_PROVIDER` | No | `internal` | Primary moderation provider (internal, openai, google, aws) |
| `MODERATION_TOXICITY_ENABLED` | No | `false` | Enable toxicity detection |
| `MODERATION_TOXICITY_PROVIDER` | No | `openai` | Toxicity analysis provider (openai, perspective) |
| `MODERATION_TOXICITY_THRESHOLD` | No | `0.8` | Toxicity score threshold for flagging (0.0-1.0) |
| `MODERATION_AUTO_DELETE_ENABLED` | No | `true` | Auto-delete content above threshold |
| `MODERATION_AUTO_DELETE_THRESHOLD` | No | `0.95` | Auto-delete toxicity threshold (0.0-1.0) |
| `MODERATION_AUTO_MUTE_ENABLED` | No | `false` | Auto-mute users on repeated violations |
| `MODERATION_AUTO_MUTE_VIOLATIONS` | No | `3` | Number of violations before auto-mute |
| `MODERATION_APPEALS_ENABLED` | No | `true` | Enable appeals system |
| `MODERATION_APPEALS_TIME_LIMIT_DAYS` | No | `7` | Days allowed to submit appeal |
| `MODERATION_CLEANUP_ENABLED` | No | `false` | Enable automatic cleanup of old records |
| `MODERATION_CLEANUP_INTERVAL_MINUTES` | No | `1440` | Cleanup interval in minutes (default 24h) |
| `MODERATION_API_KEY` | No | - | API key for authenticating webhook/API requests |
| `MODERATION_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `MODERATION_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |
| `MOD_OPENAI_API_KEY` | No | - | OpenAI API key for content moderation |
| `MOD_GOOGLE_VISION_KEY` | No | - | Google Cloud Vision API key |
| `MOD_AWS_REKOGNITION_KEY` | No | - | AWS access key for Rekognition |
| `MOD_AWS_REKOGNITION_SECRET` | No | - | AWS secret key for Rekognition |
| `MOD_AWS_REKOGNITION_REGION` | No | `us-east-1` | AWS region for Rekognition |
| `MOD_AUTO_APPROVE_BELOW` | No | `0.3` | Auto-approve content below this score (0.0-1.0) |
| `MOD_AUTO_REJECT_ABOVE` | No | `0.9` | Auto-reject content above this score (0.0-1.0) |
| `MOD_FLAG_THRESHOLD` | No | `0.5` | Flag content for review above this score (0.0-1.0) |
| `MOD_STRIKE_WARN_THRESHOLD` | No | `1` | Strikes to trigger warning |
| `MOD_STRIKE_BAN_THRESHOLD` | No | `3` | Strikes to trigger automatic ban |
| `MOD_STRIKE_EXPIRY_DAYS` | No | `30` | Days until strikes expire |
| `MOD_REVIEW_SLA_HOURS` | No | `24` | SLA for manual review completion (hours) |
| `MOD_QUEUE_WORKER_CONCURRENCY` | No | `5` | Concurrent workers for processing queue |
| `CP_DEFAULT_ACTION` | No | `flag` | Default action for policy violations (approve, flag, reject) |
| `CP_PROFANITY_ENABLED` | No | `true` | Enable profanity filtering in policies |
| `CP_MAX_CONTENT_LENGTH` | No | `10000` | Max content length to evaluate (chars) |
| `CP_EVALUATION_LOG_ENABLED` | No | `true` | Log all policy evaluations |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Server
MODERATION_PLUGIN_PORT=3704
MODERATION_LOG_LEVEL=info
MODERATION_API_KEY=your-secure-api-key-here

# Moderation Configuration
MODERATION_PROVIDER=openai
MODERATION_TOXICITY_ENABLED=true
MODERATION_TOXICITY_PROVIDER=openai
MODERATION_TOXICITY_THRESHOLD=0.75

# AI Provider Keys
MOD_OPENAI_API_KEY=sk-your-openai-key
MOD_GOOGLE_VISION_KEY=your-google-vision-key
MOD_AWS_REKOGNITION_KEY=AKIAIOSFODNN7EXAMPLE
MOD_AWS_REKOGNITION_SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
MOD_AWS_REKOGNITION_REGION=us-east-1

# Automated Thresholds
MOD_AUTO_APPROVE_BELOW=0.3
MOD_AUTO_REJECT_ABOVE=0.9
MOD_FLAG_THRESHOLD=0.5

# Strike System
MOD_STRIKE_WARN_THRESHOLD=1
MOD_STRIKE_BAN_THRESHOLD=3
MOD_STRIKE_EXPIRY_DAYS=30

# Appeals
MODERATION_APPEALS_ENABLED=true
MODERATION_APPEALS_TIME_LIMIT_DAYS=7

# Policy Engine
CP_DEFAULT_ACTION=flag
CP_PROFANITY_ENABLED=true
CP_MAX_CONTENT_LENGTH=10000

# Cleanup
MODERATION_CLEANUP_ENABLED=true
MODERATION_CLEANUP_INTERVAL_MINUTES=1440
```

### Multi-App Configuration

The moderation plugin supports multi-app isolation via the `source_account_id` column:

```bash
# Configure multiple app IDs
MODERATION_APP_IDS=app1,app2,app3

# Each moderation action/flag/review will be tagged with source_account_id
# Filter queries by app: WHERE source_account_id = 'app1'
```

### API Provider Setup

#### OpenAI Setup

1. Create API key at https://platform.openai.com/api-keys
2. Add to `.env`: `MOD_OPENAI_API_KEY=sk-xxx`
3. Set provider: `MODERATION_TOXICITY_PROVIDER=openai`

#### Google Vision Setup

1. Create project at https://console.cloud.google.com
2. Enable Cloud Vision API
3. Create service account and download JSON key
4. Add to `.env`: `MOD_GOOGLE_VISION_KEY=your-key-json`

#### AWS Rekognition Setup

1. Create IAM user with Rekognition permissions
2. Generate access key and secret
3. Add to `.env`:
   ```bash
   MOD_AWS_REKOGNITION_KEY=AKIAIOSFODNN7EXAMPLE
   MOD_AWS_REKOGNITION_SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCY
   MOD_AWS_REKOGNITION_REGION=us-east-1
   ```

---

## Database Schema

### Complete Table Overview

The Moderation plugin creates 17 tables for comprehensive content moderation:

1. **np_moderation_rules** - Custom rules with conditions and actions
2. **np_moderation_wordlists** - Profanity and allowed word lists
3. **np_moderation_actions** - Actions taken against users (warn, mute, ban)
4. **np_moderation_flags** - Content flagged for review
5. **np_moderation_appeals** - User appeals against actions
6. **np_moderation_reports** - User-submitted reports
7. **np_moderation_toxicity_scores** - AI toxicity analysis results
8. **np_moderation_user_stats** - Per-user statistics
9. **np_moderation_audit_log** - Complete audit trail
10. **mod_reviews** - Content review records
11. **mod_policies** - Content policies with thresholds
12. **mod_user_strikes** - Strike tracking with expiration
13. **cp_policies** - Content policy definitions
14. **cp_rules** - Individual policy rules
15. **cp_evaluations** - Policy evaluation results
16. **cp_word_lists** - Word lists for policies
17. **cp_overrides** - Manual policy overrides
18. **np_moderation_webhook_events** - Webhook event log

### Key Schema Details

#### np_moderation_rules

Columns: `id`, `name`, `description`, `rule_type`, `condition` (JSONB), `action`, `severity`, `enabled`, `priority`, `metadata` (JSONB), `source_account_id`, `created_at`, `updated_at`, `synced_at`

Indexes: `rule_type`, `enabled`, `priority`, `source_account_id`

#### np_moderation_actions

Columns: `id`, `user_id`, `action_type`, `reason`, `content_id`, `moderator_id`, `duration_seconds`, `expires_at`, `revoked`, `revoked_at`, `revoked_by`, `revoke_reason`, `metadata` (JSONB), `source_account_id`, `created_at`, `synced_at`

Indexes: `user_id`, `action_type`, `moderator_id`, `created_at`, `expires_at`, `source_account_id`

#### np_moderation_flags

Columns: `id`, `content_id`, `content_type`, `user_id`, `flag_reason`, `severity`, `status`, `toxicity_score`, `matched_rules` (TEXT[]), `matched_words` (TEXT[]), `reporter_id`, `reviewer_id`, `reviewed_at`, `review_decision`, `review_notes`, `metadata` (JSONB), `source_account_id`, `created_at`, `synced_at`

Indexes: `content_id`, `user_id`, `status`, `severity`, `reviewer_id`, `created_at`, `source_account_id`

#### np_moderation_user_stats

Columns: `user_id`, `total_flags`, `total_violations`, `total_warnings`, `total_mutes`, `total_bans`, `active_strikes`, `total_strikes`, `last_violation_at`, `last_action_at`, `reputation_score`, `is_currently_muted`, `is_currently_banned`, `metadata` (JSONB), `source_account_id`, `updated_at`, `synced_at`

Indexes: `total_violations`, `active_strikes`, `reputation_score`, `is_currently_muted`, `is_currently_banned`, `source_account_id`

#### mod_user_strikes

Columns: `id`, `user_id`, `reason`, `severity`, `content_id`, `action_id`, `issued_by`, `expires_at`, `expired`, `revoked`, `revoked_at`, `revoked_by`, `metadata` (JSONB), `source_account_id`, `created_at`, `synced_at`

Indexes: `user_id`, `expires_at`, `expired`, `severity`, `created_at`, `source_account_id`

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin moderation init

# Check plugin status
nself plugin moderation status

# View moderation statistics
nself plugin moderation stats
```

### Server Management

```bash
# Start moderation server
nself plugin moderation server --port 3704

# Start with custom host
nself plugin moderation server --host 127.0.0.1 --port 3704

# Start in production mode
NODE_ENV=production nself plugin moderation server
```

### Content Analysis

```bash
# Analyze text content
nself plugin moderation analyze --text "Content to check"

# Analyze with custom threshold
nself plugin moderation analyze --text "Content" --threshold 0.7

# Analyze image URL
nself plugin moderation analyze --image-url "https://example.com/image.jpg"

# Check profanity only
nself plugin moderation check-profanity "Text to check for profanity"

# Evaluate against policies
nself plugin moderation evaluate --content-id "post_123" --text "Content"
```

### Review Queue Management

```bash
# List pending reviews
nself plugin moderation queue list

# List with filters
nself plugin moderation queue list --status pending --severity high

# List sorted by age
nself plugin moderation queue list --sort age

# View specific flag details
nself plugin moderation queue show flag_123

# Approve flagged content
nself plugin moderation review flag_123 approve --notes "Content is acceptable"

# Reject flagged content
nself plugin moderation review flag_123 reject --notes "Violates policy X"
```

### Moderation Actions

```bash
# Warn user
nself plugin moderation actions warn user_123 --reason "First violation of policy X"

# Mute user temporarily
nself plugin moderation actions mute user_123 --duration 3600 --reason "Repeated spam"

# Ban user permanently
nself plugin moderation actions ban user_123 --reason "Severe ToS violation"

# Temporary ban
nself plugin moderation actions ban user_123 --duration 86400 --reason "7-day suspension"

# Unmute user
nself plugin moderation actions unmute user_123

# Unban user
nself plugin moderation actions unban user_123 --reason "Appeal approved"

# Revoke action
nself plugin moderation actions revoke action_123 --reason "Mistaken action"

# List actions for user
nself plugin moderation actions list --user user_123
```

### Appeals Management

```bash
# List pending appeals
nself plugin moderation appeals list --status pending

# View appeal details
nself plugin moderation appeals show appeal_123

# Approve appeal
nself plugin moderation appeals review appeal_123 approve --notes "Valid appeal"

# Deny appeal
nself plugin moderation appeals review appeal_123 deny --notes "Violation stands"
```

### Rules Management

```bash
# List all rules
nself plugin moderation rules list

# Create keyword rule
nself plugin moderation rules create \
  --name "spam-keywords" \
  --type keyword \
  --condition '{"keywords": ["spam", "scam"]}' \
  --action flag \
  --severity medium

# Update rule
nself plugin moderation rules update rule_123 --enabled false

# Delete rule
nself plugin moderation rules delete rule_123
```

### Policy Management

```bash
# List all policies
nself plugin moderation policies list

# Create policy
nself plugin moderation policies create \
  --name "Community Guidelines" \
  --auto-approve-below 0.3 \
  --auto-reject-above 0.9

# Update policy thresholds
nself plugin moderation policies update policy_123 \
  --auto-approve-below 0.2 \
  --flag-threshold 0.6
```

### Word List Management

```bash
# List word lists
nself plugin moderation word-lists list

# Create word list
nself plugin moderation word-lists create \
  --name "profanity-tier1" \
  --type profanity \
  --severity high \
  --words "word1,word2,word3"

# Add words to list
nself plugin moderation word-lists add wordlist_123 "newword1,newword2"

# Remove words from list
nself plugin moderation word-lists remove wordlist_123 "word1"
```

### User Status

```bash
# Check user moderation status
nself plugin moderation user-status user_123

# View user strikes
nself plugin moderation user-status user_123 --strikes

# View user action history
nself plugin moderation user-status user_123 --actions
```

---

## REST API

The moderation server exposes a comprehensive REST API on port 3704 (configurable).

### Base URL

```
http://localhost:3704
```

### Authentication

Include API key in header (if `MODERATION_API_KEY` is set):

```
Authorization: Bearer your-api-key-here
```

### Health Check

**GET** `/health`

```bash
curl http://localhost:3704/health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2026-02-11T10:00:00Z",
  "uptime": 3600
}
```

### Content Analysis

**POST** `/api/analyze`

Analyze content for moderation issues.

```bash
curl -X POST http://localhost:3704/api/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "content_id": "post_123",
    "content_type": "post",
    "text": "Content to analyze",
    "user_id": "user_456"
  }'
```

Response:
```json
{
  "content_id": "post_123",
  "status": "approved",
  "toxicity_score": 0.15,
  "confidence": 0.92,
  "matched_rules": [],
  "action": "approve"
}
```

### Policy Evaluation

**POST** `/api/evaluate`

Evaluate content against policies.

```bash
curl -X POST http://localhost:3704/api/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "content_id": "post_123",
    "text": "Content to evaluate",
    "user_id": "user_456"
  }'
```

Response:
```json
{
  "content_id": "post_123",
  "matched_policies": ["policy_1"],
  "violations": [
    {
      "policy_id": "policy_1",
      "rule_id": "rule_5",
      "severity": "medium",
      "reason": "Keyword match: spam"
    }
  ],
  "recommended_action": "flag"
}
```

### Review Queue

**GET** `/api/queue`

Get review queue items. Query params: `status`, `severity`, `limit`, `offset`.

```bash
curl "http://localhost:3704/api/queue?status=pending&severity=high"
```

**GET** `/api/queue/:id`

Get specific flag details.

```bash
curl http://localhost:3704/api/queue/flag_123
```

### Review Decision

**POST** `/api/review/:id`

Make review decision.

```bash
curl -X POST http://localhost:3704/api/review/flag_123 \
  -H "Content-Type: application/json" \
  -d '{
    "decision": "approved",
    "reviewer_id": "mod_456",
    "notes": "Content is acceptable"
  }'
```

### Moderation Actions

**POST** `/api/actions`

Create moderation action.

```bash
curl -X POST http://localhost:3704/api/actions \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_123",
    "action_type": "warn",
    "reason": "Policy violation",
    "moderator_id": "mod_789"
  }'
```

**GET** `/api/actions`

List actions. Query params: `user_id`, `action_type`, `moderator_id`, `active`.

```bash
curl "http://localhost:3704/api/actions?user_id=user_123"
```

### User Status

**GET** `/api/users/:id/status`

Get user moderation status.

```bash
curl http://localhost:3704/api/users/user_123/status
```

Response:
```json
{
  "user_id": "user_123",
  "active_strikes": 2,
  "total_strikes": 5,
  "total_violations": 7,
  "is_currently_muted": false,
  "is_currently_banned": false,
  "reputation_score": 65.5
}
```

---

## Webhook Events

The moderation plugin sends webhooks for all significant events.

### Webhook Configuration

```bash
MODERATION_WEBHOOK_URL=https://your-app.com/webhooks/moderation
```

### Event Types

All webhooks follow this format:

```json
{
  "id": "evt_123",
  "type": "action.created",
  "created": 1707650400,
  "data": {
    "object": { ... }
  }
}
```

### Action Events

- **action.created** - Moderation action taken
- **action.revoked** - Action revoked

### Flag Events

- **flag.created** - Content flagged
- **flag.reviewed** - Content reviewed

### Appeal Events

- **appeal.created** - Appeal submitted
- **appeal.reviewed** - Appeal reviewed

### Report Events

- **report.created** - Report submitted

### Rule Events

- **rule.created** - Rule created
- **rule.updated** - Rule updated

### Review Events

- **review.approved** - Content auto-approved
- **review.flagged** - Content flagged
- **review.rejected** - Content rejected
- **review.manual.completed** - Manual review completed

### Strike Events

- **user.strike.added** - Strike added
- **user.strike.threshold** - Strike threshold reached

---

## Moderation Workflows

### Automated Review Workflow

1. Content Submission → Analysis
2. Score Calculation → Toxicity/policy scores
3. Threshold Check:
   - Score < AUTO_APPROVE_BELOW → Auto-approved
   - Score > AUTO_REJECT_ABOVE → Auto-rejected
   - Between thresholds → Flagged for review
4. Queue Addition → Manual review
5. Moderator Review → Decision
6. Action Execution → Applied

### Strike System Workflow

1. Violation Detected
2. Strike Assignment
3. Threshold Check:
   - 1 strike → Warning
   - 3 strikes → Temporary mute
   - 5 strikes → Permanent ban
4. Strike Expiry (30 days default)
5. Appeal Process

---

## Policy Engine

### Policy Structure

Policies consist of:
- Name and description
- Priority (lower = first)
- Thresholds (auto-approve, flag, reject)
- Rules (matching conditions)
- Actions (what to do on match)

### Rule Types

- `keyword` - Match keywords
- `regex` - Match patterns
- `wordlist` - Match against word list
- `ai` - AI-based detection
- `metadata` - Match content metadata

---

## AI-Powered Moderation

### OpenAI Moderation

Categories: hate, harassment, self-harm, sexual, violence

```bash
MOD_OPENAI_API_KEY=sk-your-key
MODERATION_TOXICITY_PROVIDER=openai
```

### Google Cloud Vision

Image detection: adult, violence, racy content

```bash
MOD_GOOGLE_VISION_KEY=your-key
MODERATION_PROVIDER=google
```

### AWS Rekognition

Content detection: nudity, violence, drugs, hate symbols

```bash
MOD_AWS_REKOGNITION_KEY=AKIAIOSFODNN7EXAMPLE
MOD_AWS_REKOGNITION_SECRET=wJalrXUtnFEMI
MOD_AWS_REKOGNITION_REGION=us-east-1
```

---

## User Strikes & Bans

### Strike Levels

- 1 strike → Warning
- 2 strikes → Final warning
- 3 strikes → 24h mute
- 5 strikes → Permanent ban

### Configuration

```bash
MOD_STRIKE_WARN_THRESHOLD=1
MOD_STRIKE_BAN_THRESHOLD=3
MOD_STRIKE_EXPIRY_DAYS=30
```

### Ban Types

**Temporary:**
```bash
nself plugin moderation actions ban user_123 --duration 604800
```

**Permanent:**
```bash
nself plugin moderation actions ban user_123 --reason "Severe violation"
```

---

## Appeals Management

### Appeal Window

```bash
MODERATION_APPEALS_ENABLED=true
MODERATION_APPEALS_TIME_LIMIT_DAYS=7
```

### Submit Appeal

```bash
curl -X POST http://localhost:3704/api/appeals \
  -d '{
    "action_id": "action_123",
    "user_id": "user_456",
    "appeal_reason": "I believe this was a mistake..."
  }'
```

### Review Appeal

```bash
nself plugin moderation appeals review appeal_123 approve \
  --notes "Valid appeal, reversing action"
```

---

## Query Examples

### Pending Reviews

```sql
SELECT id, content_id, severity, toxicity_score, created_at
FROM np_moderation_flags
WHERE status = 'pending'
ORDER BY severity DESC, created_at ASC;
```

### Users with Multiple Strikes

```sql
SELECT user_id, active_strikes, total_violations, reputation_score
FROM np_moderation_user_stats
WHERE active_strikes >= 2
ORDER BY active_strikes DESC;
```

### Review Queue Age

```sql
SELECT
  severity,
  COUNT(*) as count,
  AVG(EXTRACT(EPOCH FROM (NOW() - created_at))/3600) as avg_age_hours
FROM np_moderation_flags
WHERE status = 'pending'
GROUP BY severity;
```

### Top Violated Rules

```sql
SELECT r.name, COUNT(*) as violation_count
FROM np_moderation_flags f
CROSS JOIN LATERAL unnest(f.matched_rules) AS rule_id
JOIN np_moderation_rules r ON r.id = rule_id
WHERE f.created_at >= NOW() - INTERVAL '30 days'
GROUP BY r.id, r.name
ORDER BY violation_count DESC
LIMIT 20;
```

---

## Troubleshooting

### Plugin Won't Start

**Error:** Cannot connect to database

**Solution:**
```bash
# Verify DATABASE_URL
echo $DATABASE_URL

# Test connection
psql $DATABASE_URL -c "SELECT 1"

# Reinitialize
nself plugin moderation init
```

### High False Positives

**Solution:**
```bash
# Increase threshold
MODERATION_TOXICITY_THRESHOLD=0.9

# Add to allowed list
nself plugin moderation word-lists create \
  --name "false-positives" \
  --type allowed \
  --words "word1,word2"
```

### Review Queue Growing

**Solution:**
```bash
# Adjust thresholds
MOD_AUTO_APPROVE_BELOW=0.4
MOD_AUTO_REJECT_ABOVE=0.85

# Increase workers
MOD_QUEUE_WORKER_CONCURRENCY=10
```

### Webhook Not Received

**Solution:**
```bash
# Check webhook log
SELECT * FROM np_moderation_webhook_events
WHERE processed = false
ORDER BY created_at DESC;
```

---

**Last Updated**: February 11, 2026
**Plugin Version**: 1.0.0
**Minimum nself Version**: 0.4.8
