# Feature Flags Plugin

Feature flags service with targeting rules, user segments, percentage rollouts, and real-time evaluation engine for nself applications.

## Overview

The Feature Flags plugin provides a comprehensive feature flag management system for controlling feature releases, A/B testing, and gradual rollouts. It supports complex targeting rules, user segmentation, percentage-based rollouts, and real-time flag evaluation with caching.

### Key Features

- **Flag Management**: Create and manage feature flags with multiple variants
- **Targeting Rules**: Complex rule-based targeting by user attributes
- **User Segments**: Group users for targeted feature releases
- **Percentage Rollouts**: Gradual feature rollouts with percentage control
- **A/B Testing**: Multi-variant testing support
- **Real-time Evaluation**: Fast flag evaluation with caching
- **Evaluation Logging**: Track flag evaluations for analytics
- **Default Values**: Fallback values for offline scenarios
- **Flag Dependencies**: Support for flag dependencies
- **Multi-Environment**: Separate flags per environment
- **Multi-App Support**: Isolated flags per source account

### Use Cases

- **Feature Releases**: Gradual rollout of new features
- **A/B Testing**: Test multiple feature variants
- **Beta Programs**: Enable features for beta users
- **Kill Switches**: Quick feature disablement
- **Canary Releases**: Test with small user percentage
- **User Targeting**: Show features to specific users
- **Regional Rollouts**: Enable features by region
- **Premium Features**: Gate features by subscription tier

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install feature-flags

# Initialize database schema
nself feature-flags init

# Start the server
nself feature-flags server
```

### Basic Usage

```bash
# Create a feature flag
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "new-dashboard",
    "name": "New Dashboard UI",
    "description": "Next generation dashboard",
    "enabled": true,
    "default_variant": "control",
    "variants": [
      {"key": "control", "value": false},
      {"key": "treatment", "value": true}
    ]
  }'

# Evaluate flag for user
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-dashboard",
    "context": {
      "user_id": "user123",
      "email": "user@example.com",
      "country": "US"
    }
  }'

# Check status
nself feature-flags status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `FF_PLUGIN_PORT` | No | `3305` | HTTP server port |
| `FF_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `FF_EVALUATION_LOG_ENABLED` | No | `true` | Log flag evaluations |
| `FF_EVALUATION_LOG_SAMPLE_RATE` | No | `100` | Log sampling rate (0-100%) |
| `FF_CACHE_TTL_SECONDS` | No | `30` | Flag cache TTL |
| `FF_API_KEY` | No | - | API key for authentication |
| `FF_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `FF_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
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
FF_PLUGIN_PORT=3305
FF_EVALUATION_LOG_ENABLED=true
FF_EVALUATION_LOG_SAMPLE_RATE=10
FF_CACHE_TTL_SECONDS=60
FF_API_KEY=your-secret-key
```

---

## CLI Commands

### `init`
Initialize database schema.

```bash
nself feature-flags init
```

### `server`
Start the API server.

```bash
nself feature-flags server [options]

Options:
  -p, --port <port>    Server port (default: 3305)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

### `flags`
Manage feature flags.

```bash
nself feature-flags flags [command]

Commands:
  list                List all flags
  create              Create new flag
  update <key>        Update flag
  delete <key>        Delete flag
  enable <key>        Enable flag
  disable <key>       Disable flag
```

### `evaluate`
Evaluate flags for context.

```bash
nself feature-flags evaluate <flagKey> [options]

Options:
  --user <userId>      User ID
  --context <json>     Full context (JSON)
```

**Example:**
```bash
nself feature-flags evaluate new-dashboard \
  --user user123 \
  --context '{"country":"US","tier":"premium"}'
```

### `segments`
Manage user segments.

```bash
nself feature-flags segments [command]

Commands:
  list                List all segments
  create              Create new segment
  update <id>         Update segment
  delete <id>         Delete segment
```

### `stats`
View flag statistics.

```bash
nself feature-flags stats
```

**Output:**
```
Feature Flags Statistics
========================
Total Flags:           45
Enabled Flags:         38
Disabled Flags:        7
Total Segments:        12
Total Rules:           89
Total Evaluations:     1523456
Evaluations (24h):     45678
Cache Hit Rate:        94.5%
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
  "plugin": "feature-flags",
  "version": "1.0.0",
  "status": "running",
  "config": {
    "evaluationLogEnabled": true,
    "evaluationLogSampleRate": 100,
    "cacheTtlSeconds": 30
  },
  "stats": {
    "totalFlags": 45,
    "enabledFlags": 38,
    "disabledFlags": 7,
    "totalSegments": 12,
    "totalRules": 89,
    "totalEvaluations": 1523456,
    "evaluationsLast24h": 45678,
    "cacheHitRate": 94.5,
    "flagsByType": {
      "boolean": 32,
      "multivariate": 13
    }
  }
}
```

### Flag Management

#### `POST /v1/flags`
Create a feature flag.

**Request:**
```json
{
  "key": "new-dashboard",
  "name": "New Dashboard UI",
  "description": "Next generation dashboard interface",
  "enabled": true,
  "type": "multivariate",
  "default_variant": "control",
  "variants": [
    {
      "key": "control",
      "name": "Original Dashboard",
      "value": false,
      "weight": 50
    },
    {
      "key": "treatment",
      "name": "New Dashboard",
      "value": true,
      "weight": 50
    }
  ],
  "targeting_rules": [
    {
      "segment_id": "beta-users",
      "variant": "treatment",
      "priority": 1
    }
  ],
  "rollout_percentage": 10,
  "tags": ["ui", "dashboard", "beta"]
}
```

**Response:**
```json
{
  "id": "flag-uuid",
  "key": "new-dashboard",
  "name": "New Dashboard UI",
  "description": "Next generation dashboard interface",
  "enabled": true,
  "type": "multivariate",
  "default_variant": "control",
  "variants": [...],
  "targeting_rules": [...],
  "rollout_percentage": 10,
  "tags": ["ui", "dashboard", "beta"],
  "evaluation_count": 0,
  "created_at": "2026-02-11T10:30:00Z",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/flags`
List all flags.

**Query Parameters:**
- `enabled`: Filter by enabled status (true/false)
- `tag`: Filter by tag
- `search`: Search in key/name/description
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "id": "flag-uuid",
      "key": "new-dashboard",
      "name": "New Dashboard UI",
      "enabled": true,
      "type": "multivariate",
      "rollout_percentage": 10,
      "evaluation_count": 12345,
      "last_evaluated": "2026-02-11T10:29:00Z",
      "created_at": "2026-02-10T08:00:00Z"
    }
  ],
  "total": 45
}
```

#### `GET /v1/flags/:key`
Get flag details.

**Response:**
```json
{
  "id": "flag-uuid",
  "key": "new-dashboard",
  "name": "New Dashboard UI",
  "description": "Next generation dashboard interface",
  "enabled": true,
  "type": "multivariate",
  "default_variant": "control",
  "variants": [...],
  "targeting_rules": [...],
  "rollout_percentage": 10,
  "tags": ["ui", "dashboard"],
  "statistics": {
    "total_evaluations": 12345,
    "evaluations_24h": 567,
    "variant_distribution": {
      "control": 6172,
      "treatment": 6173
    }
  },
  "created_at": "2026-02-10T08:00:00Z",
  "updated_at": "2026-02-11T09:00:00Z"
}
```

#### `PUT /v1/flags/:key`
Update a flag.

**Request:**
```json
{
  "name": "Updated Name",
  "enabled": false,
  "rollout_percentage": 25
}
```

**Response:**
```json
{
  "id": "flag-uuid",
  "key": "new-dashboard",
  "name": "Updated Name",
  "enabled": false,
  "rollout_percentage": 25,
  "updated_at": "2026-02-11T10:35:00Z"
}
```

#### `DELETE /v1/flags/:key`
Delete a flag.

**Response:**
```json
{
  "success": true
}
```

### Flag Evaluation

#### `POST /v1/evaluate`
Evaluate a single flag.

**Request:**
```json
{
  "flag_key": "new-dashboard",
  "context": {
    "user_id": "user123",
    "email": "user@example.com",
    "country": "US",
    "tier": "premium",
    "signup_date": "2024-01-15",
    "custom_attributes": {
      "beta_tester": true
    }
  }
}
```

**Response:**
```json
{
  "flag_key": "new-dashboard",
  "value": true,
  "variant": "treatment",
  "reason": "matched_segment",
  "segment_id": "beta-users",
  "rule_id": "rule-uuid",
  "evaluation_id": "eval-uuid",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `POST /v1/evaluate/batch`
Evaluate multiple flags at once.

**Request:**
```json
{
  "flags": ["new-dashboard", "dark-mode", "premium-features"],
  "context": {
    "user_id": "user123",
    "email": "user@example.com",
    "tier": "premium"
  }
}
```

**Response:**
```json
{
  "evaluations": {
    "new-dashboard": {
      "value": true,
      "variant": "treatment",
      "reason": "matched_segment"
    },
    "dark-mode": {
      "value": true,
      "variant": "enabled",
      "reason": "percentage_rollout"
    },
    "premium-features": {
      "value": true,
      "variant": "enabled",
      "reason": "user_attribute"
    }
  },
  "evaluation_id": "batch-eval-uuid",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `POST /v1/evaluate/all`
Evaluate all flags for context.

**Request:**
```json
{
  "context": {
    "user_id": "user123",
    "tier": "premium"
  }
}
```

**Response:**
```json
{
  "flags": {
    "new-dashboard": true,
    "dark-mode": true,
    "premium-features": true,
    "beta-feature": false
  },
  "evaluation_id": "all-eval-uuid",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

### Segments

#### `POST /v1/segments`
Create a user segment.

**Request:**
```json
{
  "name": "Beta Users",
  "description": "Users in beta program",
  "conditions": [
    {
      "attribute": "beta_tester",
      "operator": "equals",
      "value": true
    },
    {
      "attribute": "tier",
      "operator": "in",
      "value": ["premium", "enterprise"]
    }
  ],
  "condition_operator": "and"
}
```

**Response:**
```json
{
  "id": "segment-uuid",
  "name": "Beta Users",
  "description": "Users in beta program",
  "conditions": [...],
  "condition_operator": "and",
  "user_count": 234,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/segments`
List all segments.

**Response:**
```json
{
  "data": [
    {
      "id": "segment-uuid",
      "name": "Beta Users",
      "description": "Users in beta program",
      "condition_count": 2,
      "user_count": 234,
      "flag_count": 5,
      "created_at": "2026-02-10T08:00:00Z"
    }
  ],
  "total": 12
}
```

#### `GET /v1/segments/:id`
Get segment details.

**Response:**
```json
{
  "id": "segment-uuid",
  "name": "Beta Users",
  "description": "Users in beta program",
  "conditions": [...],
  "condition_operator": "and",
  "user_count": 234,
  "flags_using": [
    {
      "flag_key": "new-dashboard",
      "flag_name": "New Dashboard UI"
    }
  ],
  "statistics": {
    "evaluations_24h": 1234,
    "match_rate": 15.8
  },
  "created_at": "2026-02-10T08:00:00Z",
  "updated_at": "2026-02-11T09:00:00Z"
}
```

#### `PUT /v1/segments/:id`
Update a segment.

**Request:**
```json
{
  "name": "Updated Beta Users",
  "conditions": [...]
}
```

**Response:**
```json
{
  "id": "segment-uuid",
  "name": "Updated Beta Users",
  "updated_at": "2026-02-11T10:35:00Z"
}
```

#### `DELETE /v1/segments/:id`
Delete a segment.

**Response:**
```json
{
  "success": true
}
```

### Evaluations

#### `GET /v1/evaluations`
List evaluation history.

**Query Parameters:**
- `flag_key`: Filter by flag
- `user_id`: Filter by user
- `variant`: Filter by variant
- `from`: Start date (ISO 8601)
- `to`: End date (ISO 8601)
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "id": "eval-uuid",
      "flag_key": "new-dashboard",
      "user_id": "user123",
      "value": true,
      "variant": "treatment",
      "reason": "matched_segment",
      "created_at": "2026-02-11T10:30:00Z"
    }
  ],
  "total": 1523456
}
```

### Statistics

#### `GET /v1/stats`
Get comprehensive statistics.

**Response:**
```json
{
  "totalFlags": 45,
  "enabledFlags": 38,
  "totalSegments": 12,
  "totalEvaluations": 1523456,
  "evaluationsLast24h": 45678,
  "evaluationsLast7d": 285432,
  "cacheHitRate": 94.5,
  "avgEvaluationTimeMs": 2.3,
  "topFlags": [
    {
      "flag_key": "dark-mode",
      "evaluations": 123456,
      "enabled_percentage": 45.2
    }
  ],
  "flagsByStatus": {
    "enabled": 38,
    "disabled": 7
  },
  "variantDistribution": {
    "control": 48.5,
    "treatment": 51.5
  }
}
```

#### `GET /v1/stats/flags/:key`
Get flag-specific statistics.

**Response:**
```json
{
  "flag_key": "new-dashboard",
  "total_evaluations": 12345,
  "evaluations_24h": 567,
  "evaluations_7d": 4234,
  "unique_users_24h": 234,
  "variant_distribution": {
    "control": 6172,
    "treatment": 6173
  },
  "evaluation_trend": [
    {"date": "2026-02-05", "count": 523},
    {"date": "2026-02-06", "count": 612}
  ]
}
```

---

## Database Schema

### `ff_flags`
```sql
CREATE TABLE ff_flags (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  key VARCHAR(128) NOT NULL UNIQUE,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  enabled BOOLEAN DEFAULT true,
  type VARCHAR(32) DEFAULT 'boolean',
  default_variant VARCHAR(64),
  variants JSONB DEFAULT '[]',
  targeting_rules JSONB DEFAULT '[]',
  rollout_percentage INTEGER DEFAULT 100,
  tags TEXT[] DEFAULT '{}',
  evaluation_count INTEGER DEFAULT 0,
  last_evaluated TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (type IN ('boolean', 'multivariate', 'string', 'number', 'json')),
  CHECK (rollout_percentage >= 0 AND rollout_percentage <= 100)
);
```

### `ff_rules`
```sql
CREATE TABLE ff_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  flag_id UUID NOT NULL REFERENCES ff_flags(id) ON DELETE CASCADE,
  segment_id UUID REFERENCES ff_segments(id),
  variant VARCHAR(64) NOT NULL,
  priority INTEGER DEFAULT 0,
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### `ff_segments`
```sql
CREATE TABLE ff_segments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  conditions JSONB DEFAULT '[]',
  condition_operator VARCHAR(8) DEFAULT 'and',
  user_count INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (condition_operator IN ('and', 'or'))
);
```

### `ff_evaluations`
```sql
CREATE TABLE ff_evaluations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  flag_key VARCHAR(128) NOT NULL,
  user_id VARCHAR(255),
  value JSONB,
  variant VARCHAR(64),
  reason VARCHAR(64),
  segment_id UUID,
  rule_id UUID,
  context JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### `ff_webhook_events`
```sql
CREATE TABLE ff_webhook_events (
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

### Example 1: Simple Boolean Flag

```bash
# Create flag
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "dark-mode",
    "name": "Dark Mode",
    "enabled": true,
    "type": "boolean",
    "default_variant": "enabled",
    "variants": [
      {"key": "enabled", "value": true}
    ]
  }'

# Evaluate for user
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "dark-mode",
    "context": {"user_id": "user123"}
  }'
```

### Example 2: Percentage Rollout

```bash
# Create flag with 10% rollout
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "new-feature",
    "name": "New Feature",
    "enabled": true,
    "rollout_percentage": 10,
    "default_variant": "enabled",
    "variants": [
      {"key": "enabled", "value": true}
    ]
  }'

# Gradually increase rollout
curl -X PUT http://localhost:3305/v1/flags/new-feature \
  -H "Content-Type: application/json" \
  -d '{"rollout_percentage": 25}'
```

### Example 3: User Segment Targeting

```bash
# Create beta users segment
curl -X POST http://localhost:3305/v1/segments \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Beta Users",
    "conditions": [
      {"attribute": "beta_tester", "operator": "equals", "value": true}
    ]
  }'

# Create flag targeting segment
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "beta-feature",
    "name": "Beta Feature",
    "enabled": true,
    "targeting_rules": [
      {"segment_id": "segment-uuid", "variant": "enabled"}
    ]
  }'
```

### Example 4: A/B Test

```bash
# Create multivariate flag for A/B test
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "checkout-flow",
    "name": "Checkout Flow Test",
    "type": "multivariate",
    "enabled": true,
    "variants": [
      {"key": "control", "value": "single-page", "weight": 50},
      {"key": "treatment", "value": "multi-step", "weight": 50}
    ]
  }'

# Evaluate to get variant assignment
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "checkout-flow",
    "context": {"user_id": "user123"}
  }'

# Get variant distribution
curl http://localhost:3305/v1/stats/flags/checkout-flow
```

### Example 5: Kill Switch

```bash
# Quickly disable problematic feature
curl -X PUT http://localhost:3305/v1/flags/problematic-feature \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'

# Feature is immediately disabled for all users
```

---

## Troubleshooting

### Evaluations Not Matching Expected Rules

**Solution:**
- Check rule priority order
- Verify segment conditions
- Review rollout percentage
- Check flag enabled status
- Clear evaluation cache

### Poor Cache Performance

**Solution:**
- Adjust `FF_CACHE_TTL_SECONDS`
- Implement client-side caching
- Use batch evaluation for multiple flags
- Pre-fetch flags on app startup

### High Evaluation Latency

**Solution:**
- Enable evaluation logging sampling
- Reduce segment complexity
- Use simpler targeting rules
- Implement SDK-side caching
- Scale database connections

### Inconsistent Flag Values

**Solution:**
- Check for cached old values
- Verify flag update propagation
- Review targeting rule changes
- Check user context attributes

---

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/feature-flags
