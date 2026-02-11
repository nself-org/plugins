# Feature Flags Plugin

Production-ready feature flags service with advanced targeting rules, user segments, and a powerful evaluation engine.

## Features

- **Multiple Flag Types**: Release flags, ops flags, experiments, and kill switches
- **Targeting Rules**: Percentage rollouts, user lists, segments, attribute matching, and schedules
- **User Segments**: Define reusable user segments with complex matching logic
- **Evaluation Engine**: Fast, consistent flag evaluation with detailed logging
- **REST API**: Complete REST API for flag management and evaluation
- **CLI Tools**: Command-line interface for all operations
- **Multi-Account Support**: Isolate flags across multiple applications

## Quick Start

### 1. Install Dependencies

```bash
cd ts
npm install
npm run build
```

### 2. Configure Environment

```bash
cp .env.example .env
# Edit .env with your database credentials
```

### 3. Initialize Database

```bash
npm run cli init
```

### 4. Start Server

```bash
npm run cli server
```

The server will start on port 3305 (configurable via `FF_PLUGIN_PORT`).

## CLI Commands

### Initialize Schema
```bash
npm run cli init
```

### Start Server
```bash
npm run cli server
npm run cli server -p 3305 -h 0.0.0.0
```

### View Status
```bash
npm run cli status
```

### List Flags
```bash
npm run cli flags
npm run cli flags --type release
npm run cli flags --tag beta
npm run cli flags --enabled
```

### Evaluate Flag
```bash
npm run cli evaluate my-feature
npm run cli evaluate my-feature --user user123
npm run cli evaluate my-feature --user user123 --context '{"country":"US"}'
```

### List Segments
```bash
npm run cli segments
```

### View Statistics
```bash
npm run cli stats
```

## REST API

### Health Checks
- `GET /health` - Basic health check
- `GET /ready` - Readiness check (verifies database connectivity)
- `GET /live` - Liveness check with statistics

### Flag Management
- `POST /v1/flags` - Create flag
- `GET /v1/flags` - List flags (supports filtering)
- `GET /v1/flags/:key` - Get flag details with rules
- `PUT /v1/flags/:key` - Update flag
- `DELETE /v1/flags/:key` - Delete flag
- `POST /v1/flags/:key/enable` - Enable flag
- `POST /v1/flags/:key/disable` - Disable flag (instant kill switch)

### Rule Management
- `POST /v1/flags/:key/rules` - Add targeting rule
- `GET /v1/flags/:key/rules` - List rules for flag
- `PUT /v1/flags/:key/rules/:ruleId` - Update rule
- `DELETE /v1/flags/:key/rules/:ruleId` - Delete rule

### Segment Management
- `POST /v1/segments` - Create segment
- `GET /v1/segments` - List segments
- `PUT /v1/segments/:id` - Update segment
- `DELETE /v1/segments/:id` - Delete segment

### Evaluation
- `POST /v1/evaluate` - Evaluate single flag
- `POST /v1/evaluate/batch` - Evaluate multiple flags
- `GET /v1/evaluations` - Query evaluation history

### Statistics
- `GET /v1/stats` - Flag statistics
- `GET /v1/status` - Plugin status

## Flag Types

- **release** - Feature rollout flags (default)
- **ops** - Operational toggles (circuit breakers, etc.)
- **experiment** - A/B testing flags
- **kill_switch** - Emergency disable switches

## Rule Types

### Percentage Rollout
```json
{
  "rule_type": "percentage",
  "conditions": {
    "percentage": 25
  },
  "value": true
}
```

### User List
```json
{
  "rule_type": "user_list",
  "conditions": {
    "users": ["user1", "user2", "user3"]
  },
  "value": true
}
```

### Segment
```json
{
  "rule_type": "segment",
  "conditions": {
    "segment_id": "premium-users"
  },
  "value": true
}
```

### Attribute Matching
```json
{
  "rule_type": "attribute",
  "conditions": {
    "attribute": "country",
    "operator": "eq",
    "attribute_value": "US"
  },
  "value": true
}
```

### Schedule
```json
{
  "rule_type": "schedule",
  "conditions": {
    "start_at": "2026-01-01T00:00:00Z",
    "end_at": "2026-12-31T23:59:59Z"
  },
  "value": true
}
```

## Evaluation Logic

1. Check if flag exists and is enabled
   - If disabled → return `default_value` with reason='disabled'
2. Get rules sorted by priority (highest first)
3. For each rule, evaluate conditions
4. First matching rule → return `rule.value` with reason='rule_match'
5. No rules match → return `flag.default_value` with reason='default'

## Consistent Hashing

Percentage rollouts use consistent hashing to ensure users always see the same variant:

```typescript
hash(flagKey + userId) % 100 < percentage
```

This guarantees that:
- Same user always gets same result for a flag
- Increasing percentage includes existing users
- Distribution is statistically uniform

## Multi-Account Support

All tables include `source_account_id` column for multi-application isolation. Use headers to specify account:

```bash
curl -H "X-App-Id: my-app" http://localhost:3305/v1/flags
```

## Environment Variables

### Required
- `DATABASE_URL` - PostgreSQL connection string

### Optional
- `FF_PLUGIN_PORT` - Server port (default: 3305)
- `FF_PLUGIN_HOST` - Server host (default: 0.0.0.0)
- `FF_EVALUATION_LOG_ENABLED` - Enable evaluation logging (default: true)
- `FF_EVALUATION_LOG_SAMPLE_RATE` - Sample rate 0-100 (default: 100)
- `FF_CACHE_TTL_SECONDS` - Cache TTL (default: 30)
- `FF_API_KEY` - API key for authentication
- `FF_RATE_LIMIT_MAX` - Max requests per window (default: 500)
- `FF_RATE_LIMIT_WINDOW_MS` - Rate limit window (default: 60000)

## Database Schema

### Tables
1. **ff_flags** - Feature flags with metadata
2. **ff_rules** - Targeting rules for flags
3. **ff_segments** - Reusable user segments
4. **ff_evaluations** - Evaluation history (sampled)
5. **ff_webhook_events** - Webhook event log

## Development

### Build
```bash
npm run build
```

### Watch Mode
```bash
npm run watch
```

### Development Server
```bash
npm run dev
```

### Type Check
```bash
npm run typecheck
```

## Examples

### Create a Flag
```bash
curl -X POST http://localhost:3305/v1/flags \
  -H "Content-Type: application/json" \
  -d '{
    "key": "new-ui",
    "name": "New UI Redesign",
    "flag_type": "release",
    "enabled": true,
    "default_value": false,
    "tags": ["frontend", "beta"]
  }'
```

### Add Percentage Rule
```bash
curl -X POST http://localhost:3305/v1/flags/new-ui/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "25% Rollout",
    "rule_type": "percentage",
    "conditions": {"percentage": 25},
    "value": true,
    "priority": 100
  }'
```

### Evaluate Flag
```bash
curl -X POST http://localhost:3305/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "flag_key": "new-ui",
    "user_id": "user123",
    "context": {"country": "US", "plan": "premium"}
  }'
```

## License

MIT
