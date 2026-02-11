# Content Moderation

Content moderation and safety service with automated review, manual queue, and appeals

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

The Content Moderation plugin provides a comprehensive content safety and moderation system for the nself platform. It combines automated AI-powered content analysis with human review workflows, appeals processing, and user strike management to ensure platform safety and compliance.

This plugin is essential for social platforms, marketplaces, forums, or any application that needs to moderate user-generated content, implement community guidelines, or maintain content quality standards.

### Key Features

- **Automated Moderation** - AI-powered content analysis using OpenAI, Google Vision, or AWS Rekognition
- **Manual Review Queue** - Human moderator workflow with SLA tracking and assignment
- **Confidence Scoring** - Configurable confidence thresholds for auto-approve/reject/flag decisions
- **User Strike System** - Track violations with configurable warning and ban thresholds
- **Appeals Process** - Allow users to appeal moderation decisions with resolution tracking
- **Policy Management** - Define and enforce custom content policies and rules
- **Multi-Content Types** - Moderate text, images, videos, audio, and links
- **Audit Trail** - Complete history of all moderation decisions and actions
- **Webhook Events** - Real-time notifications for moderation decisions and policy violations
- **Multi-App Support** - Isolate moderation data by application ID

## Quick Start

```bash
# Install the plugin
nself plugin install content-moderation

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export MOD_PROVIDER="openai"  # or google, aws, none
export MOD_OPENAI_API_KEY="your-api-key"
export MOD_PLUGIN_PORT=3028

# Initialize the database schema
nself plugin content-moderation init

# Start the server
nself plugin content-moderation server
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `""` | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL connection |
| `MOD_PROVIDER` | Yes | `none` | AI provider (openai, google, aws, none) |
| `MOD_OPENAI_API_KEY` | No | `""` | OpenAI API key |
| `MOD_GOOGLE_VISION_KEY` | No | `""` | Google Vision API key |
| `MOD_AWS_REKOGNITION_KEY` | No | `""` | AWS Rekognition access key |
| `MOD_AWS_REKOGNITION_SECRET` | No | `""` | AWS Rekognition secret key |
| `MOD_AWS_REKOGNITION_REGION` | No | `us-east-1` | AWS Rekognition region |
| `MOD_PLUGIN_PORT` | No | `3028` | HTTP server port |
| `MOD_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `MOD_LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `MOD_APP_IDS` | No | `primary` | Comma-separated list of application IDs |
| `MOD_AUTO_APPROVE_BELOW` | No | `0.1` | Auto-approve if confidence score below this (0.0-1.0) |
| `MOD_AUTO_REJECT_ABOVE` | No | `0.9` | Auto-reject if confidence score above this (0.0-1.0) |
| `MOD_FLAG_THRESHOLD` | No | `0.5` | Flag for manual review if score above this |
| `MOD_STRIKE_WARN_THRESHOLD` | No | `3` | Number of strikes before warning user |
| `MOD_STRIKE_BAN_THRESHOLD` | No | `5` | Number of strikes before banning user |
| `MOD_STRIKE_EXPIRY_DAYS` | No | `90` | Days until strikes expire |
| `MOD_REVIEW_SLA_HOURS` | No | `24` | SLA for manual review completion (hours) |
| `MOD_QUEUE_WORKER_CONCURRENCY` | No | `5` | Number of concurrent review queue workers |

### Example .env

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
MOD_PROVIDER=openai
MOD_OPENAI_API_KEY=sk-your-openai-api-key

# Server Configuration
MOD_PLUGIN_PORT=3028
MOD_PLUGIN_HOST=0.0.0.0
MOD_LOG_LEVEL=info

# Multi-App Support
MOD_APP_IDS=primary,community,marketplace

# Confidence Thresholds
MOD_AUTO_APPROVE_BELOW=0.1   # Auto-approve if < 10% confidence
MOD_AUTO_REJECT_ABOVE=0.9    # Auto-reject if > 90% confidence
MOD_FLAG_THRESHOLD=0.5       # Flag for review if > 50% confidence

# Strike System
MOD_STRIKE_WARN_THRESHOLD=3  # Warn at 3 strikes
MOD_STRIKE_BAN_THRESHOLD=5   # Ban at 5 strikes
MOD_STRIKE_EXPIRY_DAYS=90    # Strikes expire after 90 days

# Review Queue
MOD_REVIEW_SLA_HOURS=24      # Reviews should be completed within 24 hours
MOD_QUEUE_WORKER_CONCURRENCY=5

# Alternative Providers
# MOD_PROVIDER=google
# MOD_GOOGLE_VISION_KEY=your-google-vision-key

# MOD_PROVIDER=aws
# MOD_AWS_REKOGNITION_KEY=your-aws-access-key
# MOD_AWS_REKOGNITION_SECRET=your-aws-secret-key
# MOD_AWS_REKOGNITION_REGION=us-east-1
```

## CLI Commands

### `init`

Initialize the content moderation database schema.

```bash
nself plugin content-moderation init
```

### `server`

Start the content moderation HTTP server.

```bash
nself plugin content-moderation server
```

### `queue`

View the moderation queue.

```bash
# View all pending reviews
nself plugin content-moderation queue

# Filter by status
nself plugin content-moderation queue --status flagged

# Limit results
nself plugin content-moderation queue --limit 20
```

### `review`

Make a moderation decision.

```bash
# Approve content
nself plugin content-moderation review \
  --id review-uuid \
  --action approve \
  --reviewer moderator123

# Reject content with reason
nself plugin content-moderation review \
  --id review-uuid \
  --action reject \
  --reason "Violates community guidelines: hate speech" \
  --policy hate-speech \
  --reviewer moderator123
```

### `appeals`

Manage appeals.

```bash
# List pending appeals
nself plugin content-moderation appeals list --status pending

# Resolve appeal
nself plugin content-moderation appeals resolve \
  --id appeal-uuid \
  --action uphold \
  --resolution "Original decision upheld after review" \
  --resolver moderator123
```

### `user-status`

Check user moderation status.

```bash
# Get user strike count and status
nself plugin content-moderation user-status --user user123

# Example output:
# {
#   "userId": "user123",
#   "activeStrikes": 2,
#   "totalStrikes": 4,
#   "status": "warned",
#   "strikes": [...]
# }
```

### `policies`

Manage moderation policies.

```bash
# List policies
nself plugin content-moderation policies list

# Create policy
nself plugin content-moderation policies create \
  --name "No Hate Speech" \
  --description "Policy prohibiting hate speech" \
  --content-types "text,comment,post" \
  --auto-action "reject" \
  --severity "high"

# Activate/deactivate policy
nself plugin content-moderation policies toggle --id policy-uuid --active false
```

### `stats`

Show moderation statistics.

```bash
nself plugin content-moderation stats

# Example output:
# {
#   "totalReviews": 15420,
#   "pendingReviews": 23,
#   "autoApproved": 12500,
#   "autoRejected": 850,
#   "manualReviews": 2070,
#   "totalAppeals": 120,
#   "totalStrikes": 450,
#   "activeUsers": 8500,
#   "bannedUsers": 15
# }
```

## REST API

### Submit Content for Review

#### `POST /api/reviews`

Submit content for moderation review.

**Request Body:**
```json
{
  "contentType": "text",
  "contentId": "post-123",
  "contentSource": "forum",
  "contentText": "This is the content to moderate...",
  "contentUrl": null,
  "authorId": "user123",
  "metadata": {
    "postTitle": "Example Post",
    "category": "general"
  }
}
```

For image moderation:
```json
{
  "contentType": "image",
  "contentId": "image-456",
  "contentUrl": "https://example.com/uploads/image.jpg",
  "authorId": "user123"
}
```

**Response:**
```json
{
  "id": "review-uuid",
  "status": "approved",
  "autoAction": "approve",
  "autoConfidence": 0.05,
  "autoResult": {
    "categories": {
      "violence": 0.02,
      "hate": 0.01,
      "sexual": 0.03
    }
  }
}
```

For flagged content:
```json
{
  "id": "review-uuid",
  "status": "flagged",
  "autoAction": "flag",
  "autoConfidence": 0.65,
  "message": "Content flagged for manual review"
}
```

### Review Management

#### `GET /api/reviews`

List reviews with filtering.

**Query Parameters:**
- `status` (optional): Filter by status (pending, flagged, approved, rejected)
- `authorId` (optional): Filter by author
- `contentType` (optional): Filter by content type
- `from` (optional): Start date (ISO 8601)
- `to` (optional): End date (ISO 8601)
- `limit` (optional, default: 50): Maximum results
- `offset` (optional, default: 0): Pagination offset

**Response:**
```json
{
  "data": [
    {
      "id": "review-uuid",
      "contentType": "text",
      "contentId": "post-123",
      "authorId": "user123",
      "status": "flagged",
      "autoConfidence": 0.65,
      "createdAt": "2025-02-11T10:30:00Z"
    }
  ],
  "total": 250
}
```

#### `GET /api/reviews/:id`

Get review details.

**Response:**
```json
{
  "id": "review-uuid",
  "contentType": "text",
  "contentId": "post-123",
  "contentText": "...",
  "authorId": "user123",
  "status": "approved",
  "autoResult": {...},
  "autoAction": "approve",
  "autoConfidence": 0.05,
  "manualResult": null,
  "reviewedAt": null,
  "createdAt": "2025-02-11T10:30:00Z"
}
```

#### `POST /api/reviews/:id/decision`

Make a manual moderation decision.

**Request Body:**
```json
{
  "action": "reject",
  "reason": "Violates community guidelines",
  "policyViolated": "hate-speech",
  "reviewerId": "moderator123"
}
```

Actions: `approve`, `reject`

**Response:**
```json
{
  "id": "review-uuid",
  "status": "rejected",
  "manualResult": "rejected",
  "manualAction": "reject",
  "reviewerId": "moderator123",
  "reviewedAt": "2025-02-11T10:35:00Z"
}
```

### Appeals

#### `POST /api/appeals`

Submit an appeal.

**Request Body:**
```json
{
  "reviewId": "review-uuid",
  "appellantId": "user123",
  "reason": "I believe this decision was incorrect because..."
}
```

**Response:** `201 Created`

#### `GET /api/appeals`

List appeals.

**Query Parameters:**
- `status` (optional): Filter by status (pending, approved, rejected)
- `limit` (optional, default: 50)
- `offset` (optional, default: 0)

**Response:**
```json
{
  "data": [
    {
      "id": "appeal-uuid",
      "reviewId": "review-uuid",
      "appellantId": "user123",
      "reason": "...",
      "status": "pending",
      "createdAt": "2025-02-11T11:00:00Z"
    }
  ],
  "total": 45
}
```

#### `POST /api/appeals/:id/resolve`

Resolve an appeal.

**Request Body:**
```json
{
  "action": "uphold",
  "resolution": "Original decision upheld after review",
  "resolvedBy": "moderator-lead"
}
```

Actions: `uphold`, `overturn`

### User Strikes

#### `GET /api/users/:userId/strikes`

Get user strike history.

**Response:**
```json
{
  "userId": "user123",
  "activeStrikes": 2,
  "totalStrikes": 4,
  "status": "warned",
  "strikes": [
    {
      "id": "strike-uuid",
      "strikeType": "hate-speech",
      "severity": "high",
      "reason": "...",
      "expiresAt": "2025-05-10T10:30:00Z",
      "createdAt": "2025-02-10T10:30:00Z"
    }
  ]
}
```

#### `POST /api/users/:userId/strikes`

Add a strike to a user.

**Request Body:**
```json
{
  "strikeType": "spam",
  "severity": "warning",
  "reason": "Posted spam content",
  "reviewId": "review-uuid"
}
```

### Policies

#### `GET /api/policies`

List moderation policies.

**Response:**
```json
{
  "data": [
    {
      "id": "policy-uuid",
      "name": "No Hate Speech",
      "description": "...",
      "contentTypes": ["text", "comment"],
      "autoAction": "reject",
      "severity": "high",
      "active": true
    }
  ]
}
```

#### `POST /api/policies`

Create a new policy.

**Request Body:**
```json
{
  "name": "No Spam",
  "description": "Policy prohibiting spam content",
  "contentTypes": ["text", "post", "comment"],
  "rules": {
    "keywords": ["spam", "buy now", "click here"],
    "patterns": ["repeated posts"]
  },
  "autoAction": "reject",
  "severity": "medium",
  "active": true
}
```

## Webhook Events

| Event Type | Description | Payload |
|------------|-------------|---------|
| `mod.review.approved` | Content auto-approved | `{ reviewId, contentId, confidence }` |
| `mod.review.flagged` | Content flagged for review | `{ reviewId, contentId, confidence }` |
| `mod.review.rejected` | Content auto-rejected | `{ reviewId, contentId, reason }` |
| `mod.review.manual.completed` | Manual review completed | `{ reviewId, action, reviewerId }` |
| `mod.appeal.submitted` | Appeal submitted | `{ appealId, reviewId, appellantId }` |
| `mod.appeal.resolved` | Appeal resolved | `{ appealId, action, resolvedBy }` |
| `mod.user.strike.added` | Strike added to user | `{ strikeId, userId, strikeType }` |
| `mod.user.strike.threshold` | User reached strike threshold | `{ userId, strikeCount, threshold }` |

## Database Schema

### mod_reviews

Stores content moderation reviews.

```sql
CREATE TABLE IF NOT EXISTS mod_reviews (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  content_type VARCHAR(20) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_source VARCHAR(50),
  content_text TEXT,
  content_url TEXT,
  author_id VARCHAR(255),
  status VARCHAR(20) DEFAULT 'pending',
  auto_result JSONB,
  auto_action VARCHAR(20),
  auto_confidence DOUBLE PRECISION,
  manual_result VARCHAR(20),
  manual_action VARCHAR(20),
  reviewer_id VARCHAR(255),
  reviewed_at TIMESTAMPTZ,
  reason TEXT,
  policy_violated VARCHAR(255),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### mod_policies

Stores moderation policies and rules.

```sql
CREATE TABLE IF NOT EXISTS mod_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  content_types TEXT[] DEFAULT '{}',
  rules JSONB NOT NULL,
  auto_action VARCHAR(20) DEFAULT 'flag',
  severity VARCHAR(20) DEFAULT 'medium',
  active BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);
```

### mod_appeals

Stores appeal requests for moderation decisions.

```sql
CREATE TABLE IF NOT EXISTS mod_appeals (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  review_id UUID NOT NULL REFERENCES mod_reviews(id) ON DELETE CASCADE,
  appellant_id VARCHAR(255) NOT NULL,
  reason TEXT NOT NULL,
  status VARCHAR(20) DEFAULT 'pending',
  resolved_by VARCHAR(255),
  resolution TEXT,
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### mod_user_strikes

Stores user strikes and violations.

```sql
CREATE TABLE IF NOT EXISTS mod_user_strikes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  review_id UUID REFERENCES mod_reviews(id),
  strike_type VARCHAR(50) NOT NULL,
  severity VARCHAR(20) DEFAULT 'warning',
  reason TEXT,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

## Examples

### Example 1: Automated Content Moderation Flow

```javascript
// Submit user comment for moderation
const response = await fetch('http://localhost:3028/api/reviews', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    contentType: 'text',
    contentId: 'comment-456',
    contentText: 'User comment text here...',
    authorId: 'user123'
  })
});

const review = await response.json();

if (review.status === 'approved') {
  // Content is safe, publish immediately
  publishComment(comment);
} else if (review.status === 'flagged') {
  // Hold for manual review
  holdForReview(comment);
} else if (review.status === 'rejected') {
  // Content violated policy, notify user
  notifyUser('Your content was removed for violating our policies');
}
```

### Example 2: Manual Review Queue

```sql
-- Get reviews pending manual review ordered by SLA
SELECT
  r.id,
  r.content_type,
  r.content_id,
  r.author_id,
  r.auto_confidence,
  r.created_at,
  EXTRACT(EPOCH FROM (NOW() - r.created_at))/3600 as hours_waiting
FROM mod_reviews r
WHERE r.source_account_id = 'primary'
  AND r.status = 'flagged'
ORDER BY r.created_at ASC
LIMIT 50;
```

### Example 3: User Strike Report

```sql
-- Generate user strike summary
SELECT
  user_id,
  COUNT(*) as total_strikes,
  SUM(CASE WHEN expires_at > NOW() THEN 1 ELSE 0 END) as active_strikes,
  MAX(severity) as highest_severity,
  MAX(created_at) as last_strike_date
FROM mod_user_strikes
WHERE source_account_id = 'primary'
GROUP BY user_id
HAVING SUM(CASE WHEN expires_at > NOW() THEN 1 ELSE 0 END) >= 3
ORDER BY active_strikes DESC;
```

## Troubleshooting

### Common Issues

#### 1. AI Provider Not Working

**Symptom:** All reviews remain in pending status.

**Solutions:**
- Verify provider is configured: `echo $MOD_PROVIDER`
- Check API key is valid: `echo $MOD_OPENAI_API_KEY`
- Test API key directly with provider
- Check server logs for API errors
- Verify network connectivity to provider API

#### 2. Too Many False Positives

**Symptom:** Safe content is being auto-rejected.

**Solutions:**
- Increase auto-reject threshold: `MOD_AUTO_REJECT_ABOVE=0.95`
- Lower flag threshold: `MOD_FLAG_THRESHOLD=0.7`
- Review and adjust policies
- Tune AI model parameters
- Implement custom review rules

#### 3. Review Queue Growing

**Symptom:** Manual review queue is overwhelming.

**Solutions:**
- Increase auto-approve threshold: `MOD_AUTO_APPROVE_BELOW=0.2`
- Hire more moderators
- Increase queue concurrency: `MOD_QUEUE_WORKER_CONCURRENCY=10`
- Implement triage system for high-priority reviews
- Use batch review tools

---

**Need more help?** Check the [main documentation](https://github.com/acamarata/nself-plugins) or [open an issue](https://github.com/acamarata/nself-plugins/issues).
