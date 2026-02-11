# Entitlements

Feature gating, subscription plan management, usage quota tracking, and metered billing system with full multi-account support.

---

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

---

## Overview

The Entitlements plugin provides a complete subscription management, feature gating, and quota tracking system. It enables you to:

- **Manage subscription plans** with flexible pricing models (monthly, yearly, one-time, usage-based)
- **Control feature access** through subscription-based feature flags and manual grants
- **Track and enforce quotas** with automatic usage monitoring and limit enforcement
- **Handle billing** with support for multiple payment providers (Stripe, Paddle, PayPal, manual)
- **Support add-ons** for extending base subscription plans
- **Record events** for audit trails and analytics
- **Multi-account isolation** for SaaS and multi-tenant applications

### Key Features

- **Flexible Plan Types**: free, standard, enterprise, custom, addon plans
- **Trial Support**: configurable trial periods with optional feature limits
- **Custom Pricing**: per-subscription price overrides and custom quotas
- **Feature Grants**: time-bound manual feature grants that override subscription features
- **Quota Management**: track usage with automatic resets (daily, weekly, monthly, yearly, billing period)
- **Event Logging**: comprehensive audit trail of all subscription and entitlement changes
- **MRR Calculation**: automatic Monthly Recurring Revenue tracking
- **Multi-Account Support**: full isolation with `source_account_id` column across all tables

### Use Cases

- **SaaS Pricing**: implement freemium, tiered pricing, or enterprise plans
- **API Rate Limiting**: enforce usage quotas per subscription tier
- **Feature Flags**: gate features based on subscription level
- **Metered Billing**: track usage for pay-as-you-go pricing
- **Beta Access**: grant temporary feature access to select users
- **Team Subscriptions**: manage workspace-level or user-level subscriptions

---

## Quick Start

```bash
# Install the plugin
nself plugin install entitlements

# Configure environment variables
cat > .env << 'EOF'
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
ENTITLEMENTS_PLUGIN_PORT=3714
ENTITLEMENTS_DEFAULT_CURRENCY=USD
ENTITLEMENTS_DEFAULT_TRIAL_DAYS=14
ENTITLEMENTS_API_KEY=your-secret-api-key
EOF

# Initialize database schema
nself plugin entitlements init

# Start the server
nself plugin entitlements server
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string (or use individual POSTGRES_* vars) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `` | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL connection |
| `ENTITLEMENTS_PLUGIN_PORT` | No | `3714` | HTTP server port |
| `ENTITLEMENTS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `ENTITLEMENTS_DEFAULT_CURRENCY` | No | `USD` | Default currency for plans |
| `ENTITLEMENTS_DEFAULT_TRIAL_DAYS` | No | `14` | Default trial period in days |
| `ENTITLEMENTS_QUOTA_WARNING_THRESHOLD` | No | `80` | Percentage threshold for quota warnings |
| `ENTITLEMENTS_API_KEY` | No | - | API key for authentication (recommended for production) |
| `ENTITLEMENTS_RATE_LIMIT_MAX` | No | `500` | Maximum requests per window |
| `ENTITLEMENTS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env File

```bash
# Database Configuration
DATABASE_URL=postgresql://nself:password@localhost:5432/nself_production

# Server Configuration
ENTITLEMENTS_PLUGIN_PORT=3714
ENTITLEMENTS_PLUGIN_HOST=0.0.0.0

# Entitlements Configuration
ENTITLEMENTS_DEFAULT_CURRENCY=USD
ENTITLEMENTS_DEFAULT_TRIAL_DAYS=14
ENTITLEMENTS_QUOTA_WARNING_THRESHOLD=80

# Security
ENTITLEMENTS_API_KEY=super-secret-key-change-this
ENTITLEMENTS_RATE_LIMIT_MAX=500
ENTITLEMENTS_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

---

## CLI Commands

### Initialize Schema

```bash
nself plugin entitlements init
```

Creates all required database tables, indexes, and constraints.

### Start Server

```bash
# Default port (3714)
nself plugin entitlements server

# Custom port
nself plugin entitlements server --port 8080

# Custom host and port
nself plugin entitlements server --host 127.0.0.1 --port 8080
```

### View Status

```bash
nself plugin entitlements status
```

Output:
```
Entitlements Status
====================
Plans:                5
Active Subscriptions: 142
Trialing:             23
Features:             18
Active Grants:        7
Quotas:               284
Exceeded Quotas:      3
Events:               1,247
MRR:                  $8,450.00
```

### Manage Plans

```bash
# List all plans
nself plugin entitlements plans list

# List plans by type
nself plugin entitlements plans list --type=standard

# Get plan details
nself plugin entitlements plans get --id=uuid
nself plugin entitlements plans get --slug=pro-monthly
```

### Manage Subscriptions

```bash
# List all subscriptions
nself plugin entitlements subscriptions list

# List by workspace
nself plugin entitlements subscriptions list --workspace=ws_123

# List by user
nself plugin entitlements subscriptions list --user=usr_456

# Filter by status
nself plugin entitlements subscriptions list --status=active

# Get subscription details
nself plugin entitlements subscriptions get --id=uuid

# Get active subscription for workspace
nself plugin entitlements subscriptions active --workspace=ws_123
```

### Check Feature Access

```bash
# Check if workspace has feature access
nself plugin entitlements check-feature --key=advanced_analytics --workspace=ws_123

# Check if user has feature access
nself plugin entitlements check-feature --key=api_access --user=usr_456
```

Output:
```
Feature Access Check
====================
Feature:    advanced_analytics
Has Access: Yes
Value:      true
Source:     subscription
```

### Check Quota Availability

```bash
# Check quota availability
nself plugin entitlements check-quota --key=api_requests --workspace=ws_123

# Check with specific amount
nself plugin entitlements check-quota --key=api_requests --workspace=ws_123 --amount=100
```

Output:
```
Quota Availability Check
========================
Quota:     api_requests
Available: Yes
Usage:     4250
Limit:     10000
Remaining: 5750
```

---

## REST API

### Base URL
```
http://localhost:3714
```

### Authentication

If `ENTITLEMENTS_API_KEY` is set, include it in requests:

```http
Authorization: Bearer your-api-key-here
```

### Multi-Account Context

Pass the source account ID via header for account isolation:

```http
X-Source-Account-ID: customer-123
```

If not provided, defaults to `primary`.

### Health Checks

#### GET /health

Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "entitlements",
  "timestamp": "2026-02-11T10:30:00.000Z"
}
```

#### GET /ready

Database readiness check.

**Response:**
```json
{
  "ready": true,
  "plugin": "entitlements",
  "timestamp": "2026-02-11T10:30:00.000Z"
}
```

#### GET /live

Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "entitlements",
  "version": "1.0.0",
  "uptime": 3600.5,
  "stats": {
    "total_plans": 5,
    "active_subscriptions": 142,
    "trialing_subscriptions": 23,
    "total_features": 18,
    "total_grants": 7,
    "active_quotas": 284,
    "exceeded_quotas": 3,
    "total_events": 1247,
    "mrr_cents": 845000
  },
  "timestamp": "2026-02-11T10:30:00.000Z"
}
```

### Plans

#### GET /api/entitlements/plans

List all plans.

**Query Parameters:**
- `plan_type` (string): Filter by plan type (free, standard, enterprise, custom, addon)
- `billing_interval` (string): Filter by interval (month, year, one_time, usage)
- `is_public` (boolean): Filter by visibility
- `is_archived` (boolean): Include archived plans

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "source_account_id": "primary",
      "name": "Pro Monthly",
      "slug": "pro-monthly",
      "description": "Full-featured professional plan",
      "billing_interval": "month",
      "price_cents": 4900,
      "currency": "USD",
      "trial_days": 14,
      "trial_limits": null,
      "plan_type": "standard",
      "is_public": true,
      "is_archived": false,
      "features": {
        "advanced_analytics": true,
        "api_access": true,
        "custom_branding": true
      },
      "quotas": {
        "api_requests": 10000,
        "storage_gb": 100,
        "team_members": 10
      },
      "metadata": {},
      "display_order": 2,
      "created_at": "2026-01-15T08:00:00.000Z",
      "updated_at": "2026-01-15T08:00:00.000Z"
    }
  ]
}
```

#### POST /api/entitlements/plans

Create a new plan.

**Request Body:**
```json
{
  "name": "Enterprise Yearly",
  "slug": "enterprise-yearly",
  "description": "Full enterprise features with annual billing",
  "billing_interval": "year",
  "price_cents": 49900,
  "currency": "USD",
  "trial_days": 30,
  "plan_type": "enterprise",
  "is_public": true,
  "features": {
    "everything": true,
    "white_label": true,
    "dedicated_support": true
  },
  "quotas": {
    "api_requests": -1,
    "storage_gb": 1000,
    "team_members": -1
  },
  "display_order": 3
}
```

**Response:**
```json
{
  "success": true,
  "id": "uuid"
}
```

#### GET /api/entitlements/plans/:id

Get plan details by ID.

#### GET /api/entitlements/plans/slug/:slug

Get plan details by slug.

#### PUT /api/entitlements/plans/:id

Update a plan.

**Request Body:**
```json
{
  "name": "Updated Plan Name",
  "price_cents": 5900,
  "features": {
    "advanced_analytics": true,
    "new_feature": true
  }
}
```

#### DELETE /api/entitlements/plans/:id

Archive a plan (soft delete).

### Subscriptions

#### GET /api/entitlements/subscriptions

List subscriptions.

**Query Parameters:**
- `workspace_id` (string): Filter by workspace
- `user_id` (string): Filter by user
- `status` (string): Filter by status (trialing, active, past_due, canceled, unpaid, expired, paused)
- `plan_id` (string): Filter by plan

#### POST /api/entitlements/subscriptions

Create a new subscription.

**Request Body:**
```json
{
  "workspace_id": "ws_123",
  "plan_id": "uuid",
  "start_trial": true,
  "metadata": {
    "source": "website_signup"
  }
}
```

**Response:**
```json
{
  "success": true,
  "id": "uuid"
}
```

#### GET /api/entitlements/subscriptions/:id

Get subscription details.

#### PUT /api/entitlements/subscriptions/:id

Update subscription.

#### POST /api/entitlements/subscriptions/:id/cancel

Cancel a subscription.

**Request Body:**
```json
{
  "reason": "User requested cancellation",
  "immediate": false
}
```

If `immediate` is true, cancels immediately. Otherwise, cancels at period end.

#### POST /api/entitlements/subscriptions/:id/pause

Pause a subscription.

**Request Body:**
```json
{
  "resume_at": "2026-03-01T00:00:00.000Z"
}
```

#### POST /api/entitlements/subscriptions/:id/resume

Resume a paused subscription.

### Features

#### GET /api/entitlements/features

List all features.

**Query Parameters:**
- `category` (string): Filter by category
- `is_active` (boolean): Filter by active status

#### POST /api/entitlements/features

Create a feature definition.

**Request Body:**
```json
{
  "key": "advanced_analytics",
  "name": "Advanced Analytics",
  "description": "Access to advanced analytics dashboard",
  "feature_type": "boolean",
  "default_value": false,
  "category": "analytics"
}
```

#### GET /api/entitlements/features/:key/check

Check feature access for a workspace or user.

**Query Parameters:**
- `workspaceId` (string): Workspace ID
- `userId` (string): User ID

**Response:**
```json
{
  "has_access": true,
  "value": true,
  "source": "subscription"
}
```

Sources: `grant` (manual grant), `subscription` (from plan), `none` (no access).

### Quotas

#### GET /api/entitlements/quotas

List quotas.

**Query Parameters:**
- `workspace_id`, `user_id`, `subscription_id`, `quota_key`

#### GET /api/entitlements/quotas/:key/check

Check quota availability.

**Query Parameters:**
- `workspaceId`, `userId`, `amount`

**Response:**
```json
{
  "available": true,
  "current_usage": 4250,
  "limit_value": 10000,
  "remaining": 5750
}
```

#### POST /api/entitlements/quotas/:id/reset

Manually reset a quota to zero.

### Usage Tracking

#### POST /api/entitlements/usage/track

Track usage and increment quota.

**Request Body:**
```json
{
  "workspace_id": "ws_123",
  "quota_key": "api_requests",
  "usage_amount": 1,
  "resource_type": "api_call",
  "resource_id": "endpoint_123",
  "metadata": {
    "endpoint": "/api/v1/data",
    "method": "GET"
  }
}
```

**Response:**
```json
{
  "success": true,
  "usage_id": "uuid",
  "new_usage": 4251,
  "limit_value": 10000,
  "remaining": 5749
}
```

If quota exceeded:
```json
{
  "success": false,
  "error": "quota_exceeded",
  "new_usage": 10000,
  "limit_value": 10000,
  "remaining": 0
}
```

### Addons

#### GET /api/entitlements/subscriptions/:id/addons

List addons for a subscription.

#### POST /api/entitlements/subscriptions/:id/addons

Add an addon to a subscription.

**Request Body:**
```json
{
  "addon_plan_id": "uuid",
  "quantity": 2
}
```

#### DELETE /api/entitlements/addons/:id

Remove an addon.

### Grants

#### GET /api/entitlements/grants

List feature grants.

**Query Parameters:**
- `workspace_id`, `user_id`, `feature_key`, `is_active`

#### POST /api/entitlements/grants

Create a feature grant (manual override).

**Request Body:**
```json
{
  "workspace_id": "ws_123",
  "feature_key": "beta_features",
  "feature_value": true,
  "granted_by": "admin_user_id",
  "grant_reason": "Beta program participant",
  "expires_at": "2026-12-31T23:59:59.000Z"
}
```

#### DELETE /api/entitlements/grants/:id

Revoke a grant.

### Events

#### GET /api/entitlements/events

List entitlement events.

**Query Parameters:**
- `workspace_id`, `user_id`, `event_type`, `subscription_id`, `limit`, `offset`

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "source_account_id": "primary",
      "event_type": "subscription_created",
      "workspace_id": "ws_123",
      "user_id": null,
      "subscription_id": "uuid",
      "plan_id": "uuid",
      "event_data": null,
      "actor_user_id": null,
      "created_at": "2026-02-11T10:00:00.000Z"
    }
  ]
}
```

### Status

#### GET /v1/status

Get plugin status and statistics.

---

## Webhook Events

The entitlements plugin emits the following webhook events:

| Event | Description |
|-------|-------------|
| `subscription.created` | New subscription created |
| `subscription.updated` | Subscription modified |
| `subscription.canceled` | Subscription canceled |
| `subscription.renewed` | Subscription renewed for new period |
| `trial.started` | Trial period started |
| `trial.ended` | Trial period ended |
| `quota.exceeded` | Quota limit exceeded |
| `quota.reset` | Quota reset to zero |
| `grant.created` | Feature grant created |
| `grant.revoked` | Feature grant revoked |
| `addon.added` | Addon added to subscription |
| `addon.removed` | Addon removed from subscription |
| `plan.upgraded` | Subscription upgraded to higher plan |
| `plan.downgraded` | Subscription downgraded to lower plan |

These events are stored in the `entitlement_events` table and can be consumed via the `/api/entitlements/events` endpoint or via external webhook delivery systems.

---

## Database Schema

### entitlement_plans

Subscription plan definitions.

```sql
CREATE TABLE entitlement_plans (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  description TEXT,
  billing_interval VARCHAR(32) NOT NULL,
  price_cents INTEGER NOT NULL DEFAULT 0,
  currency VARCHAR(8) NOT NULL DEFAULT 'USD',
  trial_days INTEGER DEFAULT 0,
  trial_limits JSONB,
  plan_type VARCHAR(32) NOT NULL DEFAULT 'standard',
  is_public BOOLEAN DEFAULT true,
  is_archived BOOLEAN DEFAULT false,
  features JSONB NOT NULL DEFAULT '{}',
  quotas JSONB NOT NULL DEFAULT '{}',
  metadata JSONB,
  display_order INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_entitlement_plans_source_account ON entitlement_plans(source_account_id);
CREATE INDEX idx_entitlement_plans_slug ON entitlement_plans(source_account_id, slug);
CREATE INDEX idx_entitlement_plans_type ON entitlement_plans(plan_type);
CREATE INDEX idx_entitlement_plans_features ON entitlement_plans USING GIN(features);
```

**Columns:**
| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation key |
| `name` | TEXT | Display name |
| `slug` | TEXT | URL-friendly identifier |
| `description` | TEXT | Plan description |
| `billing_interval` | VARCHAR(32) | month, year, one_time, usage |
| `price_cents` | INTEGER | Price in cents |
| `currency` | VARCHAR(8) | ISO currency code |
| `trial_days` | INTEGER | Trial period duration |
| `trial_limits` | JSONB | Optional feature/quota limits during trial |
| `plan_type` | VARCHAR(32) | free, standard, enterprise, custom, addon |
| `is_public` | BOOLEAN | Publicly visible on pricing page |
| `is_archived` | BOOLEAN | Soft delete flag |
| `features` | JSONB | Feature flags (e.g., {"api_access": true}) |
| `quotas` | JSONB | Quota limits (e.g., {"api_requests": 10000}) |
| `metadata` | JSONB | Custom metadata |
| `display_order` | INTEGER | Sort order for display |

### entitlement_subscriptions

Active and historical subscriptions.

```sql
CREATE TABLE entitlement_subscriptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  workspace_id VARCHAR(255),
  user_id VARCHAR(255),
  plan_id UUID NOT NULL REFERENCES entitlement_plans(id) ON DELETE RESTRICT,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  billing_interval VARCHAR(32) NOT NULL,
  price_cents INTEGER NOT NULL,
  currency VARCHAR(8) NOT NULL DEFAULT 'USD',
  is_custom_pricing BOOLEAN DEFAULT false,
  custom_quotas JSONB,
  custom_features JSONB,
  payment_provider VARCHAR(32),
  payment_provider_subscription_id TEXT,
  payment_provider_customer_id TEXT,
  trial_start TIMESTAMP WITH TIME ZONE,
  trial_end TIMESTAMP WITH TIME ZONE,
  current_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
  current_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
  cancel_at_period_end BOOLEAN DEFAULT false,
  canceled_at TIMESTAMP WITH TIME ZONE,
  cancellation_reason TEXT,
  pause_collection VARCHAR(32),
  pause_start TIMESTAMP WITH TIME ZONE,
  pause_end TIMESTAMP WITH TIME ZONE,
  metadata JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_entitlement_subscriptions_source_account ON entitlement_subscriptions(source_account_id);
CREATE INDEX idx_entitlement_subscriptions_workspace ON entitlement_subscriptions(workspace_id);
CREATE INDEX idx_entitlement_subscriptions_user ON entitlement_subscriptions(user_id);
CREATE INDEX idx_entitlement_subscriptions_status ON entitlement_subscriptions(status);
```

### entitlement_features

Feature flag definitions.

```sql
CREATE TABLE entitlement_features (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  key TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT,
  feature_type VARCHAR(32) NOT NULL,
  default_value JSONB,
  category TEXT,
  metadata JSONB,
  is_active BOOLEAN DEFAULT true,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### entitlement_quotas

Quota instances per subscription.

```sql
CREATE TABLE entitlement_quotas (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  workspace_id VARCHAR(255),
  user_id VARCHAR(255),
  subscription_id UUID NOT NULL REFERENCES entitlement_subscriptions(id) ON DELETE CASCADE,
  quota_key TEXT NOT NULL,
  quota_name TEXT NOT NULL,
  limit_value BIGINT,
  is_unlimited BOOLEAN DEFAULT false,
  current_usage BIGINT DEFAULT 0,
  reset_interval VARCHAR(32),
  last_reset_at TIMESTAMP WITH TIME ZONE,
  next_reset_at TIMESTAMP WITH TIME ZONE,
  metadata JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### entitlement_usage

Usage records for quota tracking.

```sql
CREATE TABLE entitlement_usage (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  workspace_id VARCHAR(255),
  user_id VARCHAR(255),
  quota_id UUID NOT NULL REFERENCES entitlement_quotas(id) ON DELETE CASCADE,
  quota_key TEXT NOT NULL,
  usage_amount BIGINT NOT NULL DEFAULT 1,
  resource_type TEXT,
  resource_id VARCHAR(255),
  metadata JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### entitlement_addons

Addon subscriptions attached to base subscriptions.

```sql
CREATE TABLE entitlement_addons (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  addon_plan_id UUID NOT NULL REFERENCES entitlement_plans(id) ON DELETE RESTRICT,
  subscription_id UUID NOT NULL REFERENCES entitlement_subscriptions(id) ON DELETE CASCADE,
  quantity INTEGER NOT NULL DEFAULT 1,
  price_cents INTEGER NOT NULL,
  currency VARCHAR(8) NOT NULL DEFAULT 'USD',
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  current_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
  current_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
  payment_provider_item_id TEXT,
  metadata JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### entitlement_grants

Manual feature grants (overrides).

```sql
CREATE TABLE entitlement_grants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  workspace_id VARCHAR(255),
  user_id VARCHAR(255),
  feature_key TEXT NOT NULL,
  feature_value JSONB NOT NULL,
  granted_by VARCHAR(255),
  grant_reason TEXT,
  expires_at TIMESTAMP WITH TIME ZONE,
  is_active BOOLEAN DEFAULT true,
  metadata JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### entitlement_events

Event log for audit trail.

```sql
CREATE TABLE entitlement_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(64) NOT NULL,
  workspace_id VARCHAR(255),
  user_id VARCHAR(255),
  subscription_id UUID REFERENCES entitlement_subscriptions(id) ON DELETE SET NULL,
  plan_id UUID REFERENCES entitlement_plans(id) ON DELETE SET NULL,
  event_data JSONB,
  actor_user_id VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Examples

### Example 1: Create a Freemium Pricing Model

```sql
-- Create free plan
INSERT INTO entitlement_plans (name, slug, billing_interval, price_cents, plan_type, features, quotas)
VALUES (
  'Free',
  'free',
  'month',
  0,
  'free',
  '{"basic_features": true}',
  '{"api_requests": 1000, "storage_gb": 1}'
);

-- Create pro plan
INSERT INTO entitlement_plans (name, slug, billing_interval, price_cents, plan_type, features, quotas)
VALUES (
  'Pro',
  'pro-monthly',
  'month',
  2900,
  'standard',
  '{"basic_features": true, "advanced_analytics": true, "api_access": true}',
  '{"api_requests": 10000, "storage_gb": 100}'
);
```

### Example 2: Subscribe a Workspace

```bash
curl -X POST http://localhost:3714/api/entitlements/subscriptions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "workspace_id": "ws_123",
    "plan_id": "uuid-of-pro-plan",
    "start_trial": true
  }'
```

### Example 3: Track API Usage

```bash
curl -X POST http://localhost:3714/api/entitlements/usage/track \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "workspace_id": "ws_123",
    "quota_key": "api_requests",
    "usage_amount": 1,
    "resource_type": "api_call",
    "resource_id": "/api/v1/users",
    "metadata": {"method": "GET", "status": 200}
  }'
```

### Example 4: Check Feature Access Before Rendering UI

```javascript
const response = await fetch(
  'http://localhost:3714/api/entitlements/features/advanced_analytics/check?workspaceId=ws_123',
  {
    headers: { 'Authorization': 'Bearer your-api-key' }
  }
);

const { has_access } = await response.json();

if (has_access) {
  // Show advanced analytics dashboard
}
```

### Example 5: Grant Beta Access

```bash
curl -X POST http://localhost:3714/api/entitlements/grants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "workspace_id": "ws_123",
    "feature_key": "beta_ai_features",
    "feature_value": true,
    "granted_by": "admin_user_456",
    "grant_reason": "Early access beta program",
    "expires_at": "2026-06-30T23:59:59.000Z"
  }'
```

---

## Troubleshooting

### Issue: Quota Not Updating

**Symptom**: Usage tracking returns success, but quota doesn't increment.

**Solution**: Verify the quota record exists for the subscription. Quotas are auto-created when subscriptions are created, but if quotas were added to a plan after subscription creation, they won't exist automatically.

```sql
-- Check if quota exists
SELECT * FROM entitlement_quotas
WHERE subscription_id = 'your-subscription-id'
AND quota_key = 'api_requests';

-- If missing, create manually via API or database
```

### Issue: Feature Check Returns False Despite Valid Subscription

**Symptom**: `check-feature` returns `has_access: false` even though subscription is active.

**Solution**: Check feature key spelling and ensure the feature is defined in the plan's `features` JSONB column. Feature checks are case-sensitive.

```sql
-- Verify plan features
SELECT features FROM entitlement_plans WHERE id = (
  SELECT plan_id FROM entitlement_subscriptions WHERE id = 'sub-id'
);
```

### Issue: MRR Calculation Incorrect

**Symptom**: MRR doesn't match expected value.

**Solution**: MRR only counts `active` subscriptions. Check subscription statuses:

```sql
SELECT status, COUNT(*), SUM(price_cents)
FROM entitlement_subscriptions
GROUP BY status;
```

Yearly subscriptions are divided by 12 for MRR calculation.

### Issue: Multi-Account Isolation Not Working

**Symptom**: Users see data from other accounts.

**Solution**: Ensure `X-Source-Account-ID` header is being passed in all requests. Check that your application correctly sets this header based on the authenticated user's account.

```javascript
// Always pass source account ID
fetch('/api/entitlements/plans', {
  headers: {
    'X-Source-Account-ID': currentUser.accountId,
    'Authorization': 'Bearer token'
  }
});
```

### Issue: Rate Limit Errors

**Symptom**: 429 Too Many Requests errors.

**Solution**: Increase rate limits in environment variables or implement client-side request queuing:

```bash
ENTITLEMENTS_RATE_LIMIT_MAX=1000
ENTITLEMENTS_RATE_LIMIT_WINDOW_MS=60000
```

---
