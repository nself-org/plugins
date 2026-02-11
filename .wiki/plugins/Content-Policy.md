# Content Policy Plugin

Content policy evaluation and moderation engine with rule-based filtering, profanity detection, and automated content moderation for nself applications.

## Overview

The Content Policy plugin provides a comprehensive content moderation system that evaluates user-generated content against configurable policies and rules. It supports multiple action types (allow, flag, block), custom word lists, policy overrides, and detailed audit logging.

### Key Features

- **Policy Management**: Create and manage content policies with multiple rules
- **Rule Engine**: Flexible rule-based content evaluation
- **Word Lists**: Custom profanity and blocked word lists
- **Multiple Actions**: Allow, flag for review, or block content
- **Context-Aware**: Evaluate content based on user, content type, and context
- **Profanity Detection**: Built-in profanity detection with customizable lists
- **Policy Overrides**: Manual overrides for specific content
- **Audit Trail**: Complete evaluation history for compliance
- **Multi-App Support**: Isolated policies per source account
- **Performance**: Fast evaluation with caching support
- **Configurable**: Adjust strictness and thresholds per policy

### Use Cases

- **Social Networks**: Moderate user posts and comments
- **Forums**: Filter inappropriate content automatically
- **E-commerce**: Review product descriptions and reviews
- **Gaming**: Monitor chat and user-generated content
- **Education**: Ensure age-appropriate content
- **News Sites**: Pre-moderate user comments
- **Marketplace**: Screen listings and descriptions
- **Community Platforms**: Enforce community guidelines

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install content-policy

# Initialize database schema
nself content-policy init

# Start the server
nself content-policy server
```

### Basic Usage

```bash
# Evaluate content
curl -X POST http://localhost:3504/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "content": "This is some user content to check",
    "content_type": "post",
    "user_id": "user123",
    "context": {"channel": "general"}
  }'

# Create a policy
curl -X POST http://localhost:3504/v1/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Default Moderation",
    "description": "Standard content moderation rules",
    "enabled": true,
    "rules": [
      {
        "type": "profanity",
        "action": "flag",
        "severity": "medium"
      }
    ]
  }'

# Check status
nself content-policy status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `CP_PLUGIN_PORT` | No | `3504` | HTTP server port |
| `CP_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `CP_DEFAULT_ACTION` | No | `flag` | Default action (allow, flag, block) |
| `CP_PROFANITY_ENABLED` | No | `true` | Enable built-in profanity detection |
| `CP_MAX_CONTENT_LENGTH` | No | `100000` | Maximum content length to evaluate |
| `CP_EVALUATION_LOG_ENABLED` | No | `true` | Log all evaluations |
| `CP_API_KEY` | No | - | API key for authentication |
| `CP_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `CP_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
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
CP_PLUGIN_PORT=3504
CP_DEFAULT_ACTION=flag
CP_PROFANITY_ENABLED=true
CP_MAX_CONTENT_LENGTH=50000
CP_EVALUATION_LOG_ENABLED=true
CP_API_KEY=your-secret-key
```

---

## CLI Commands

### `init`
Initialize the database schema.

```bash
nself content-policy init
```

### `server`
Start the HTTP API server.

```bash
nself content-policy server [options]

Options:
  -p, --port <port>    Server port (default: 3504)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

### `evaluate`
Evaluate content against policies.

```bash
nself content-policy evaluate <content> [options]

Options:
  --user <userId>        User ID
  --type <contentType>   Content type
  --context <json>       Additional context (JSON string)
```

**Example:**
```bash
nself content-policy evaluate "Check this content" \
  --user user123 \
  --type comment \
  --context '{"channel":"general"}'
```

### `policies`
Manage content policies.

```bash
nself content-policy policies [command]

Commands:
  list                List all policies
  create              Create new policy
  update <id>         Update policy
  delete <id>         Delete policy
  enable <id>         Enable policy
  disable <id>        Disable policy
```

### `word-lists`
Manage word lists.

```bash
nself content-policy word-lists [command]

Commands:
  list                List all word lists
  create              Create word list
  add <listId>        Add words to list
  remove <listId>     Remove words from list
```

### `queue`
View flagged content queue.

```bash
nself content-policy queue [options]

Options:
  -l, --limit <limit>    Number to show (default: 20)
  --action <action>      Filter by action (flag, block)
```

### `stats`
View moderation statistics.

```bash
nself content-policy stats
```

**Output:**
```
Content Policy Statistics
=========================
Total Evaluations:    15234
Allowed:              13456
Flagged:              1234
Blocked:              544
Total Policies:       5
Active Policies:      4
Total Word Lists:     3
Total Rules:          12
```

---

## REST API

All endpoints support multi-app isolation via `X-Source-Account-Id` header.

### Health & Status

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "content-policy",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/status`
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "content-policy",
  "version": "1.0.0",
  "status": "running",
  "config": {
    "defaultAction": "flag",
    "profanityEnabled": true,
    "maxContentLength": 100000,
    "evaluationLogEnabled": true
  },
  "stats": {
    "totalEvaluations": 15234,
    "allowed": 13456,
    "flagged": 1234,
    "blocked": 544,
    "totalPolicies": 5,
    "activePolicies": 4,
    "totalWordLists": 3,
    "totalRules": 12,
    "evaluationsByAction": {
      "allow": 13456,
      "flag": 1234,
      "block": 544
    }
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

### Content Evaluation

#### `POST /v1/evaluate`
Evaluate content against all active policies.

**Request:**
```json
{
  "content": "This is the content to check for policy violations",
  "content_type": "post",
  "user_id": "user123",
  "content_id": "post-abc-123",
  "context": {
    "channel": "general",
    "language": "en",
    "region": "US"
  }
}
```

**Response:**
```json
{
  "evaluation_id": "eval-uuid",
  "action": "flag",
  "reason": "Potential profanity detected",
  "confidence": 0.85,
  "matched_rules": [
    {
      "policy_id": "policy-uuid",
      "policy_name": "Default Moderation",
      "rule_type": "profanity",
      "action": "flag",
      "severity": "medium",
      "matched_terms": ["****"]
    }
  ],
  "suggestions": [
    "Review content manually",
    "Consider editing flagged terms"
  ],
  "metadata": {
    "processing_time_ms": 45,
    "policies_evaluated": 3,
    "rules_matched": 1
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `POST /v1/evaluate/batch`
Evaluate multiple content items.

**Request:**
```json
{
  "items": [
    {
      "content": "First post content",
      "content_type": "post",
      "user_id": "user123",
      "content_id": "post-1"
    },
    {
      "content": "Second comment content",
      "content_type": "comment",
      "user_id": "user456",
      "content_id": "comment-1"
    }
  ]
}
```

**Response:**
```json
{
  "results": [
    {
      "content_id": "post-1",
      "action": "allow",
      "confidence": 0.95
    },
    {
      "content_id": "comment-1",
      "action": "flag",
      "reason": "Potential spam detected",
      "confidence": 0.78
    }
  ],
  "summary": {
    "total": 2,
    "allowed": 1,
    "flagged": 1,
    "blocked": 0
  }
}
```

### Policies

#### `POST /v1/policies`
Create a new content policy.

**Request:**
```json
{
  "name": "Strict Moderation",
  "description": "Strict content moderation for public channels",
  "enabled": true,
  "priority": 1,
  "rules": [
    {
      "type": "profanity",
      "action": "block",
      "severity": "high",
      "word_list_id": "profanity-list-uuid"
    },
    {
      "type": "spam",
      "action": "flag",
      "severity": "medium",
      "threshold": 0.7
    },
    {
      "type": "length",
      "action": "flag",
      "min_length": 10,
      "max_length": 5000
    }
  ],
  "applies_to": {
    "content_types": ["post", "comment"],
    "channels": ["public"],
    "user_roles": ["user", "member"]
  }
}
```

**Response:**
```json
{
  "id": "policy-uuid",
  "name": "Strict Moderation",
  "description": "Strict content moderation for public channels",
  "enabled": true,
  "priority": 1,
  "rules": [...],
  "applies_to": {...},
  "created_at": "2026-02-11T10:30:00Z",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/policies`
List all policies.

**Query Parameters:**
- `enabled`: Filter by enabled status (true/false)

**Response:**
```json
{
  "data": [
    {
      "id": "policy-uuid",
      "name": "Strict Moderation",
      "description": "Strict content moderation",
      "enabled": true,
      "priority": 1,
      "rule_count": 3,
      "evaluation_count": 1523,
      "created_at": "2026-02-11T10:00:00Z"
    }
  ],
  "total": 5
}
```

#### `GET /v1/policies/:id`
Get policy details.

**Response:**
```json
{
  "id": "policy-uuid",
  "name": "Strict Moderation",
  "description": "Strict content moderation for public channels",
  "enabled": true,
  "priority": 1,
  "rules": [...],
  "applies_to": {...},
  "statistics": {
    "total_evaluations": 1523,
    "actions": {
      "allow": 1200,
      "flag": 250,
      "block": 73
    }
  },
  "created_at": "2026-02-11T10:00:00Z",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `PUT /v1/policies/:id`
Update a policy.

**Request:**
```json
{
  "name": "Updated Policy Name",
  "enabled": false,
  "rules": [...]
}
```

**Response:**
```json
{
  "id": "policy-uuid",
  "name": "Updated Policy Name",
  "enabled": false,
  "updated_at": "2026-02-11T10:35:00Z"
}
```

#### `DELETE /v1/policies/:id`
Delete a policy.

**Response:**
```json
{
  "success": true
}
```

### Word Lists

#### `POST /v1/word-lists`
Create a new word list.

**Request:**
```json
{
  "name": "Profanity List",
  "description": "Common profanity words",
  "type": "blocklist",
  "words": ["word1", "word2", "word3"],
  "case_sensitive": false,
  "match_partial": true
}
```

**Response:**
```json
{
  "id": "list-uuid",
  "name": "Profanity List",
  "description": "Common profanity words",
  "type": "blocklist",
  "word_count": 3,
  "case_sensitive": false,
  "match_partial": true,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/word-lists`
List all word lists.

**Response:**
```json
{
  "data": [
    {
      "id": "list-uuid",
      "name": "Profanity List",
      "type": "blocklist",
      "word_count": 245,
      "usage_count": 12,
      "created_at": "2026-02-11T10:00:00Z"
    }
  ],
  "total": 3
}
```

#### `POST /v1/word-lists/:id/words`
Add words to a list.

**Request:**
```json
{
  "words": ["newword1", "newword2"]
}
```

**Response:**
```json
{
  "success": true,
  "added": 2,
  "word_count": 247
}
```

#### `DELETE /v1/word-lists/:id/words`
Remove words from a list.

**Request:**
```json
{
  "words": ["word1", "word2"]
}
```

**Response:**
```json
{
  "success": true,
  "removed": 2,
  "word_count": 245
}
```

### Evaluations

#### `GET /v1/evaluations`
List evaluation history.

**Query Parameters:**
- `action`: Filter by action (allow, flag, block)
- `user_id`: Filter by user
- `content_type`: Filter by content type
- `from`: Start date (ISO 8601)
- `to`: End date (ISO 8601)
- `limit`: Results per page (default: 50)
- `offset`: Pagination offset (default: 0)

**Response:**
```json
{
  "data": [
    {
      "id": "eval-uuid",
      "content_type": "post",
      "user_id": "user123",
      "action": "flag",
      "reason": "Potential profanity detected",
      "confidence": 0.85,
      "reviewed": false,
      "created_at": "2026-02-11T10:30:00Z"
    }
  ],
  "total": 1234,
  "limit": 50,
  "offset": 0,
  "hasMore": true
}
```

#### `GET /v1/evaluations/:id`
Get evaluation details.

**Response:**
```json
{
  "id": "eval-uuid",
  "content": "Original content...",
  "content_type": "post",
  "user_id": "user123",
  "content_id": "post-abc-123",
  "action": "flag",
  "reason": "Potential profanity detected",
  "confidence": 0.85,
  "matched_rules": [...],
  "suggestions": [...],
  "reviewed": false,
  "reviewed_by": null,
  "reviewed_at": null,
  "override_action": null,
  "created_at": "2026-02-11T10:30:00Z"
}
```

### Overrides

#### `POST /v1/overrides`
Create a manual policy override.

**Request:**
```json
{
  "evaluation_id": "eval-uuid",
  "content_id": "post-abc-123",
  "override_action": "allow",
  "reason": "False positive - content is acceptable",
  "reviewed_by": "moderator123"
}
```

**Response:**
```json
{
  "id": "override-uuid",
  "evaluation_id": "eval-uuid",
  "content_id": "post-abc-123",
  "original_action": "flag",
  "override_action": "allow",
  "reason": "False positive - content is acceptable",
  "reviewed_by": "moderator123",
  "created_at": "2026-02-11T10:35:00Z"
}
```

#### `GET /v1/queue`
Get flagged content queue for review.

**Query Parameters:**
- `reviewed`: Filter by review status (true/false)
- `action`: Filter by action
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "id": "eval-uuid",
      "content_preview": "First 100 chars of content...",
      "content_type": "post",
      "user_id": "user123",
      "action": "flag",
      "reason": "Potential profanity detected",
      "confidence": 0.85,
      "reviewed": false,
      "created_at": "2026-02-11T10:30:00Z"
    }
  ],
  "total": 234,
  "unreviewed": 187
}
```

### Statistics

#### `GET /v1/stats`
Get comprehensive moderation statistics.

**Response:**
```json
{
  "totalEvaluations": 15234,
  "evaluationsByAction": {
    "allow": 13456,
    "flag": 1234,
    "block": 544
  },
  "evaluationsByContentType": {
    "post": 8234,
    "comment": 5432,
    "message": 1568
  },
  "flaggedQueue": {
    "total": 234,
    "unreviewed": 187,
    "overridden": 47
  },
  "topViolations": [
    {"type": "profanity", "count": 892},
    {"type": "spam", "count": 234},
    {"type": "harassment", "count": 108}
  ],
  "averageConfidence": 0.87,
  "processingTimeMs": {
    "avg": 42,
    "p50": 38,
    "p95": 75,
    "p99": 120
  }
}
```

---

## Database Schema

### `cp_policies`
```sql
CREATE TABLE cp_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  enabled BOOLEAN DEFAULT true,
  priority INTEGER DEFAULT 0,
  rules JSONB DEFAULT '[]',
  applies_to JSONB DEFAULT '{}',
  evaluation_count INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### `cp_rules`
```sql
CREATE TABLE cp_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  policy_id UUID NOT NULL REFERENCES cp_policies(id) ON DELETE CASCADE,
  type VARCHAR(64) NOT NULL,
  action VARCHAR(16) NOT NULL,
  severity VARCHAR(16),
  config JSONB DEFAULT '{}',
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (action IN ('allow', 'flag', 'block'))
);
```

### `cp_evaluations`
```sql
CREATE TABLE cp_evaluations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  content TEXT NOT NULL,
  content_type VARCHAR(64),
  user_id VARCHAR(255),
  content_id VARCHAR(255),
  action VARCHAR(16) NOT NULL,
  reason TEXT,
  confidence DECIMAL(3,2),
  matched_rules JSONB DEFAULT '[]',
  suggestions JSONB DEFAULT '[]',
  context JSONB DEFAULT '{}',
  reviewed BOOLEAN DEFAULT false,
  reviewed_by VARCHAR(255),
  reviewed_at TIMESTAMP WITH TIME ZONE,
  override_action VARCHAR(16),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (action IN ('allow', 'flag', 'block'))
);
```

### `cp_word_lists`
```sql
CREATE TABLE cp_word_lists (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  type VARCHAR(16) DEFAULT 'blocklist',
  words TEXT[] DEFAULT '{}',
  case_sensitive BOOLEAN DEFAULT false,
  match_partial BOOLEAN DEFAULT true,
  usage_count INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (type IN ('blocklist', 'allowlist'))
);
```

### `cp_overrides`
```sql
CREATE TABLE cp_overrides (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  evaluation_id UUID REFERENCES cp_evaluations(id),
  content_id VARCHAR(255),
  original_action VARCHAR(16) NOT NULL,
  override_action VARCHAR(16) NOT NULL,
  reason TEXT,
  reviewed_by VARCHAR(255) NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (original_action IN ('allow', 'flag', 'block')),
  CHECK (override_action IN ('allow', 'flag', 'block'))
);
```

---

## Examples

### Example 1: Basic Content Evaluation

```bash
# Evaluate a user post
curl -X POST http://localhost:3504/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Check this content for policy violations",
    "content_type": "post",
    "user_id": "user123"
  }'
```

### Example 2: Create Multi-Rule Policy

```bash
curl -X POST http://localhost:3504/v1/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Community Guidelines",
    "enabled": true,
    "rules": [
      {
        "type": "profanity",
        "action": "block",
        "severity": "high"
      },
      {
        "type": "spam",
        "action": "flag",
        "threshold": 0.8
      },
      {
        "type": "harassment",
        "action": "block",
        "severity": "high"
      }
    ]
  }'
```

### Example 3: Custom Word List

```bash
# Create word list
curl -X POST http://localhost:3504/v1/word-lists \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Spam Keywords",
    "type": "blocklist",
    "words": ["crypto", "bitcoin", "forex", "investment"],
    "match_partial": false
  }'

# Add more words
curl -X POST http://localhost:3504/v1/word-lists/list-id/words \
  -H "Content-Type: application/json" \
  -d '{"words": ["nft", "trading"]}'
```

### Example 4: Review Flagged Content

```bash
# Get flagged queue
curl http://localhost:3504/v1/queue?reviewed=false

# Override evaluation (approve flagged content)
curl -X POST http://localhost:3504/v1/overrides \
  -H "Content-Type: application/json" \
  -d '{
    "evaluation_id": "eval-uuid",
    "override_action": "allow",
    "reason": "False positive - acceptable content",
    "reviewed_by": "moderator123"
  }'
```

### Example 5: Batch Evaluation

```bash
curl -X POST http://localhost:3504/v1/evaluate/batch \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {"content": "Post 1", "content_id": "p1", "user_id": "u1"},
      {"content": "Post 2", "content_id": "p2", "user_id": "u2"},
      {"content": "Post 3", "content_id": "p3", "user_id": "u3"}
    ]
  }'
```

---

## Troubleshooting

### High False Positive Rate

**Solution:**
- Adjust confidence thresholds in rules
- Use `flag` instead of `block` for review
- Refine word lists to exclude common terms
- Add context-specific rules
- Review and create overrides for patterns

### Slow Evaluation Performance

**Solution:**
- Reduce `CP_MAX_CONTENT_LENGTH`
- Optimize word lists (fewer, more specific terms)
- Disable evaluation logging: `CP_EVALUATION_LOG_ENABLED=false`
- Use caching for frequent evaluations
- Consider async evaluation for non-critical paths

### Missing Violations

**Solution:**
- Ensure policies are enabled
- Check policy priority and rule order
- Verify word lists are properly formatted
- Review confidence thresholds
- Enable debug logging to trace evaluation

---

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/content-policy
