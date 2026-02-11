# Content Policy Plugin

Production-ready content policy evaluation and moderation engine for nself.

## Features

- **Rule-Based Evaluation**: Flexible rule engine supporting multiple rule types
- **Real-Time Content Moderation**: Evaluate content against policies in milliseconds
- **Multi-App Support**: Isolated data per source account
- **REST API**: Complete HTTP API for integration
- **CLI Tools**: Command-line interface for management
- **Queue Management**: Track flagged/quarantined content for review
- **Override System**: Moderator overrides with audit trail
- **Statistics & Analytics**: Comprehensive violation tracking

## Quick Start

### 1. Installation

```bash
cd plugins/content-policy/ts
npm install
npm run build
```

### 2. Configuration

```bash
cp .env.example .env
# Edit .env with your database credentials
```

### 3. Initialize Database

```bash
npm run build
node dist/cli.js init
```

### 4. Start Server

```bash
npm run dev  # Development mode
# or
npm start    # Production mode
```

## Rule Types

### 1. Keyword
Check if any word from a word list appears in content:
```json
{
  "type": "keyword",
  "word_list_id": "uuid",
  "case_sensitive": false
}
```

### 2. Regex
Test content against regex pattern:
```json
{
  "type": "regex",
  "pattern": "\\b\\d{3}-\\d{2}-\\d{4}\\b",
  "flags": "i"
}
```

### 3. Length
Check content length constraints:
```json
{
  "type": "length",
  "min_length": 10,
  "max_length": 5000
}
```

### 4. Profanity
Check against built-in profanity list:
```json
{
  "type": "profanity",
  "level": "moderate"
}
```

### 5. Media Type
Validate content type:
```json
{
  "type": "media_type",
  "allowed_types": ["text", "image"],
  "blocked_types": ["video"]
}
```

### 6. Link Check
Validate URLs in content:
```json
{
  "type": "link_check",
  "blocked_domains": ["spam.com"],
  "allowed_domains": ["trusted.com"]
}
```

## API Endpoints

### Evaluation
- `POST /v1/evaluate` - Evaluate single content
- `POST /v1/evaluate/batch` - Batch evaluate
- `GET /v1/evaluations` - List evaluations
- `GET /v1/evaluations/:id` - Get evaluation

### Policies
- `POST /v1/policies` - Create policy
- `GET /v1/policies` - List policies
- `GET /v1/policies/:id` - Get policy with rules
- `PUT /v1/policies/:id` - Update policy
- `DELETE /v1/policies/:id` - Delete policy

### Rules
- `POST /v1/policies/:id/rules` - Add rule
- `GET /v1/policies/:id/rules` - List rules
- `PUT /v1/policies/:id/rules/:ruleId` - Update rule
- `DELETE /v1/policies/:id/rules/:ruleId` - Delete rule

### Word Lists
- `POST /v1/word-lists` - Create word list
- `GET /v1/word-lists` - List word lists
- `PUT /v1/word-lists/:id` - Update word list
- `DELETE /v1/word-lists/:id` - Delete word list

### Moderation
- `POST /v1/overrides` - Override evaluation
- `GET /v1/overrides` - List overrides
- `GET /v1/queue` - Get moderation queue
- `GET /v1/stats` - Get statistics

## CLI Commands

```bash
# Initialize database
nself-content-policy init

# Start server
nself-content-policy server

# Evaluate content
nself-content-policy evaluate "content text" --type text

# Manage policies
nself-content-policy policies list
nself-content-policy policies create --name "Comments Policy"
nself-content-policy policies show <policy-id>

# Manage word lists
nself-content-policy word-lists list
nself-content-policy word-lists create --name "Blocklist" --type blocklist --words "spam,scam"
nself-content-policy word-lists add <list-id> --words "additional,words"

# View queue
nself-content-policy queue --result flagged

# View statistics
nself-content-policy stats --days 7
```

## Example: Creating a Content Policy

```bash
# 1. Create a policy
POLICY_ID=$(curl -X POST http://localhost:3504/v1/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "User Comments Policy",
    "description": "Moderation rules for user comments",
    "content_types": ["comment", "post"],
    "enabled": true,
    "priority": 10,
    "mode": "enforce"
  }' | jq -r '.id')

# 2. Create a word list
WORDLIST_ID=$(curl -X POST http://localhost:3504/v1/word-lists \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Spam Keywords",
    "list_type": "blocklist",
    "words": ["spam", "scam", "viagra", "casino"],
    "case_sensitive": false
  }' | jq -r '.id')

# 3. Add a keyword rule
curl -X POST http://localhost:3504/v1/policies/$POLICY_ID/rules \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Block Spam Keywords\",
    \"rule_type\": \"keyword\",
    \"config\": {
      \"type\": \"keyword\",
      \"word_list_id\": \"$WORDLIST_ID\",
      \"case_sensitive\": false
    },
    \"action\": \"deny\",
    \"severity\": \"high\",
    \"enabled\": true
  }"

# 4. Add a profanity rule
curl -X POST http://localhost:3504/v1/policies/$POLICY_ID/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Profanity Filter",
    "rule_type": "profanity",
    "config": {
      "type": "profanity",
      "level": "moderate"
    },
    "action": "flag",
    "severity": "medium",
    "enabled": true
  }'

# 5. Evaluate content
curl -X POST http://localhost:3504/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "content_type": "comment",
    "content_text": "Check out this amazing offer!",
    "content_id": "comment_123",
    "submitter_id": "user_456"
  }'
```

## Environment Variables

### Required
- `DATABASE_URL` or `POSTGRES_*` - Database connection

### Optional
- `CP_PLUGIN_PORT` - Server port (default: 3504)
- `CP_DEFAULT_ACTION` - Default action: allow, deny, flag, quarantine (default: flag)
- `CP_PROFANITY_ENABLED` - Enable profanity filter (default: true)
- `CP_MAX_CONTENT_LENGTH` - Max content length (default: 100000)
- `CP_EVALUATION_LOG_ENABLED` - Log evaluations (default: true)
- `CP_API_KEY` - API key for authentication
- `CP_RATE_LIMIT_MAX` - Rate limit (default: 200)
- `CP_RATE_LIMIT_WINDOW_MS` - Rate limit window (default: 60000)

## Database Tables

### cp_policies
Content policies with priority and mode settings.

### cp_rules
Rules associated with policies, defining evaluation logic.

### cp_evaluations
Evaluation history with results and matched rules.

### cp_word_lists
Reusable word lists for keyword filtering.

### cp_overrides
Moderator overrides with audit trail.

## License

Source-Available License
