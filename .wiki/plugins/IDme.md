# ID.me Plugin

OAuth authentication with government-grade identity verification for military, veterans, first responders, government employees, teachers, students, and nurses for nself.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Performance Considerations](#performance-considerations)
- [Security Notes](#security-notes)
- [Advanced Code Examples](#advanced-code-examples)
- [Monitoring & Alerting](#monitoring--alerting)
- [Use Cases](#use-cases)
- [Troubleshooting](#troubleshooting)

---

## Overview

The ID.me plugin provides a complete OAuth 2.0 integration with ID.me for government-grade identity verification. It supports 7 verification groups and stores verification records, badges, and attributes in PostgreSQL.

- **5 Database Tables** - Verifications, groups, badges, attributes, webhook events
- **3 Analytics Views** - Verified users, group summary, recent verifications
- **7 Verification Groups** - Military, Veteran, First Responder, Government, Teacher, Student, Nurse
- **Complete OAuth 2.0** - Authorization, token exchange, refresh
- **Badge Management** - Visual badges for verified groups
- **Sandbox Mode** - Test with ID.me sandbox environment

### Verification Groups

| Group | Scopes | Attributes |
|-------|--------|------------|
| Military | `military` | branch, rank, status, affiliation |
| Veteran | `veteran` | branch, service_era, rank, affiliation |
| First Responder | `first_responder` | department, role |
| Government | `government` | agency, level |
| Teacher | `teacher` | school, subject |
| Student | `student` | school, graduation_year |
| Nurse | `nurse` | specialty, license |

---

## Quick Start

```bash
# 1. Register your application at developers.id.me

# 2. Install the plugin
cd plugins/idme
./install.sh

# 3. Configure environment
cp .env.example .env
# Edit .env with your ID.me credentials

# 4. Install TypeScript dependencies
cd ts
npm install
npm run build

# 5. Initialize database schema
nself plugin idme init

# 6. Start HTTP server
nself plugin idme server --port 3010

# 7. Test configuration
nself plugin idme test
```

### Prerequisites

- Node.js 20+
- PostgreSQL
- ID.me developer account ([developers.id.me](https://developers.id.me))

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `IDME_CLIENT_ID` | Yes | - | ID.me OAuth client ID |
| `IDME_CLIENT_SECRET` | Yes | - | ID.me OAuth client secret |
| `IDME_REDIRECT_URI` | Yes | - | OAuth callback URL (must match ID.me dashboard) |
| `IDME_SCOPES` | No | `openid,email,profile` | Comma-separated OAuth scopes |
| `IDME_SANDBOX` | No | `false` | Use ID.me sandbox environment (api.idmelabs.com) |
| `IDME_WEBHOOK_SECRET` | No | - | Secret for webhook signature verification |
| `PORT` | No | `3010` | HTTP server port |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### OAuth Scopes

| Scope | Required | Description |
|-------|----------|-------------|
| `openid` | Yes | OpenID Connect |
| `email` | Yes | User's email address |
| `profile` | Yes | Basic profile information |
| `military` | No | Active duty military verification |
| `veteran` | No | Military veteran verification |
| `first_responder` | No | First responder verification (police, fire, EMT) |
| `government` | No | Government employee verification |
| `teacher` | No | Teacher/educator verification |
| `student` | No | Student verification |
| `nurse` | No | Nurse/healthcare worker verification |

### Example .env File

```bash
# Required
IDME_CLIENT_ID=your_client_id
IDME_CLIENT_SECRET=your_client_secret
IDME_REDIRECT_URI=https://your-domain.com/callback/idme
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Scopes (include the groups you want to verify)
IDME_SCOPES=openid,email,profile,military,veteran

# Optional
IDME_SANDBOX=false
IDME_WEBHOOK_SECRET=your_webhook_secret
PORT=3010
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize OAuth configuration and database schema
nself plugin idme init

# Generate authorization URL for OAuth flow
nself plugin idme init auth

# Test configuration (config, database, API connectivity)
nself plugin idme test
```

### Verification

```bash
# Check verification status for a user
nself plugin idme verify user@example.com
```

### Group Management

```bash
# List all verification groups
nself plugin idme groups list

# Show users in a specific group
nself plugin idme groups type military
```

### Testing

```bash
# Test configuration
nself plugin idme test config

# Test database connectivity
nself plugin idme test database

# Test API connectivity
nself plugin idme test api

# Run all tests
nself plugin idme test
```

### Server

```bash
# Start HTTP server
nself plugin idme server --port 3010
```

---

## REST API

The plugin exposes an HTTP server for OAuth callbacks, webhooks, and API queries.

### Base URL

```
http://localhost:3010
```

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/auth/idme` | Start OAuth flow (redirects to ID.me) |
| `GET` | `/callback/idme` | OAuth callback handler |
| `POST` | `/webhook/idme` | Webhook receiver for verification events |
| `GET` | `/api/verifications/:userId` | Get verification status for a user |

### OAuth Flow

1. Redirect users to `/auth/idme` to begin the OAuth flow
2. Users authenticate with ID.me and grant permissions
3. ID.me redirects back to `/callback/idme` with an authorization code
4. The callback handler automatically:
   - Exchanges the authorization code for an access token
   - Fetches user profile data
   - Fetches verification groups
   - Stores all data in the database

### Get Verification Status

```http
GET /api/verifications/:userId
```

Returns the user's verification status including verified groups, badges, and attributes.

---

## Webhook Events

ID.me sends webhooks for verification lifecycle events. Configure your webhook URL in the ID.me developer dashboard.

### Supported Events

| Event | Description |
|-------|-------------|
| `verification.created` | New verification record created |
| `verification.updated` | Verification status updated |
| `verification.completed` | Verification completed successfully |
| `verification.failed` | Verification failed |
| `group.verified` | User verified for a specific group |
| `group.revoked` | Group verification revoked |
| `attribute.updated` | User attribute updated |

### Webhook Setup

1. Set `IDME_WEBHOOK_SECRET` in your `.env` file
2. Configure webhook URL in ID.me dashboard: `https://your-domain.com/webhook/idme`
3. Select verification events to receive
4. Webhooks are verified using HMAC-SHA256 signature

---

## Database Schema

### idme_verifications

Main verification records with OAuth tokens.

```sql
CREATE TABLE idme_verifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,                 -- your application user ID
    idme_user_id VARCHAR(255) NOT NULL,    -- ID.me user ID
    email VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    access_token TEXT,                     -- encrypted in production
    refresh_token TEXT,                    -- encrypted in production
    token_expires_at TIMESTAMP WITH TIME ZONE,
    verified BOOLEAN DEFAULT FALSE,
    verified_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50),                    -- pending, verified, failed, expired
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_idme_verifications_user ON idme_verifications(user_id);
CREATE INDEX idx_idme_verifications_idme_user ON idme_verifications(idme_user_id);
CREATE INDEX idx_idme_verifications_email ON idme_verifications(email);
CREATE INDEX idx_idme_verifications_status ON idme_verifications(status);
```

### idme_groups

Verification group membership.

```sql
CREATE TABLE idme_groups (
    id UUID PRIMARY KEY,
    verification_id UUID REFERENCES idme_verifications(id),
    user_id UUID NOT NULL,
    group_type VARCHAR(50) NOT NULL,       -- military, veteran, first_responder, government, teacher, student, nurse
    verified BOOLEAN DEFAULT FALSE,
    verified_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_idme_groups_user ON idme_groups(user_id);
CREATE INDEX idx_idme_groups_type ON idme_groups(group_type);
CREATE INDEX idx_idme_groups_verified ON idme_groups(verified);
```

### idme_badges

Visual badges for verified groups.

```sql
CREATE TABLE idme_badges (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    group_type VARCHAR(50) NOT NULL,
    badge_name VARCHAR(255),
    icon VARCHAR(50),
    color VARCHAR(50),
    active BOOLEAN DEFAULT TRUE,
    display_order INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_idme_badges_user ON idme_badges(user_id);
CREATE INDEX idx_idme_badges_active ON idme_badges(active);
```

### idme_attributes

Additional verified attributes (branch, rank, school, etc.).

```sql
CREATE TABLE idme_attributes (
    id UUID PRIMARY KEY,
    verification_id UUID REFERENCES idme_verifications(id),
    user_id UUID NOT NULL,
    attribute_name VARCHAR(100) NOT NULL,  -- branch, rank, school, department, etc.
    attribute_value TEXT,
    verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_idme_attributes_user ON idme_attributes(user_id);
CREATE INDEX idx_idme_attributes_name ON idme_attributes(attribute_name);
```

### idme_webhook_events

Webhook event audit log.

```sql
CREATE TABLE idme_webhook_events (
    id UUID PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    data JSONB NOT NULL,
    signature VARCHAR(255),
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_idme_webhook_events_type ON idme_webhook_events(event_type);
CREATE INDEX idx_idme_webhook_events_processed ON idme_webhook_events(processed);
CREATE INDEX idx_idme_webhook_events_received ON idme_webhook_events(received_at DESC);
```

---

## Analytics Views

### idme_verified_users

All verified users with their groups and badges.

```sql
CREATE VIEW idme_verified_users AS
SELECT
    v.user_id,
    v.email,
    v.first_name,
    v.last_name,
    v.verified,
    v.verified_at,
    v.status,
    ARRAY_AGG(DISTINCT g.group_type) FILTER (WHERE g.verified = TRUE) AS verified_groups,
    COUNT(DISTINCT g.id) FILTER (WHERE g.verified = TRUE) AS group_count,
    COUNT(DISTINCT b.id) FILTER (WHERE b.active = TRUE) AS badge_count
FROM idme_verifications v
LEFT JOIN idme_groups g ON v.id = g.verification_id
LEFT JOIN idme_badges b ON v.user_id = b.user_id
GROUP BY v.user_id, v.email, v.first_name, v.last_name, v.verified, v.verified_at, v.status
ORDER BY v.verified_at DESC;
```

### idme_group_summary

Verification counts by group type.

```sql
CREATE VIEW idme_group_summary AS
SELECT
    group_type,
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE verified = TRUE) AS verified,
    COUNT(*) FILTER (WHERE verified = FALSE) AS pending
FROM idme_groups
GROUP BY group_type
ORDER BY verified DESC;
```

### idme_recent_verifications

Verifications from the last 30 days.

```sql
CREATE VIEW idme_recent_verifications AS
SELECT
    v.user_id,
    v.email,
    v.first_name,
    v.last_name,
    v.status,
    v.verified_at,
    ARRAY_AGG(DISTINCT g.group_type) FILTER (WHERE g.verified = TRUE) AS groups
FROM idme_verifications v
LEFT JOIN idme_groups g ON v.id = g.verification_id
WHERE v.created_at > NOW() - INTERVAL '30 days'
GROUP BY v.user_id, v.email, v.first_name, v.last_name, v.status, v.verified_at
ORDER BY v.created_at DESC;
```

---

## Performance Considerations

### OAuth Flow Optimization

The OAuth flow is optimized for speed and reliability while maintaining security standards.

#### Connection Pooling

Use PostgreSQL connection pooling to reduce latency:

```typescript
// Database configuration
const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
  max: 20,                    // Maximum pool connections
  idleTimeoutMillis: 30000,   // Close idle connections after 30s
  connectionTimeoutMillis: 2000, // Fail fast on connection issues
  keepAlive: true,
  keepAliveInitialDelayMillis: 10000
});
```

#### Token Exchange Performance

The authorization code exchange is the critical path in OAuth:

```typescript
// Optimized token exchange with timeout
async function exchangeCodeForToken(code: string): Promise<TokenResponse> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 5000); // 5s timeout

  try {
    const response = await fetch('https://api.id.me/oauth/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        grant_type: 'authorization_code',
        client_id: process.env.IDME_CLIENT_ID,
        client_secret: process.env.IDME_CLIENT_SECRET,
        code,
        redirect_uri: process.env.IDME_REDIRECT_URI
      }),
      signal: controller.signal
    });

    clearTimeout(timeout);

    if (!response.ok) {
      throw new Error(`Token exchange failed: ${response.status}`);
    }

    return await response.json();
  } catch (error) {
    clearTimeout(timeout);
    if (error.name === 'AbortError') {
      throw new Error('Token exchange timeout - ID.me may be experiencing issues');
    }
    throw error;
  }
}
```

**Expected Latencies:**
- Token exchange: 200-800ms
- User profile fetch: 100-400ms
- Database writes: 20-50ms per table
- Total OAuth callback: 500-1500ms

#### Parallel Data Fetching

Fetch user data in parallel to reduce total time:

```typescript
async function fetchUserData(accessToken: string) {
  // Fetch profile, groups, and attributes in parallel
  const [profile, groups, attributes] = await Promise.all([
    fetchUserProfile(accessToken),
    fetchVerificationGroups(accessToken),
    fetchUserAttributes(accessToken)
  ]);

  return { profile, groups, attributes };
}
```

### Token Refresh Optimization

Token refresh should be proactive, not reactive.

#### Background Token Refresh

Implement a background job to refresh tokens before expiry:

```typescript
// Refresh tokens expiring in the next 24 hours
async function refreshExpiringTokens() {
  const expiringVerifications = await db.query(`
    SELECT id, user_id, refresh_token, token_expires_at
    FROM idme_verifications
    WHERE token_expires_at < NOW() + INTERVAL '24 hours'
      AND token_expires_at > NOW()
      AND refresh_token IS NOT NULL
      AND status = 'verified'
  `);

  const results = {
    success: 0,
    failed: 0,
    errors: []
  };

  for (const verification of expiringVerifications.rows) {
    try {
      const newTokens = await refreshAccessToken(verification.refresh_token);

      await db.query(`
        UPDATE idme_verifications
        SET access_token = $1,
            refresh_token = $2,
            token_expires_at = $3,
            updated_at = NOW()
        WHERE id = $4
      `, [
        newTokens.access_token,
        newTokens.refresh_token,
        new Date(Date.now() + newTokens.expires_in * 1000),
        verification.id
      ]);

      results.success++;
      logger.info('Token refreshed', {
        verificationId: verification.id,
        expiresAt: newTokens.expires_in
      });
    } catch (error) {
      results.failed++;
      results.errors.push({
        verificationId: verification.id,
        error: error.message
      });

      logger.error('Token refresh failed', {
        verificationId: verification.id,
        error: error.message
      });
    }
  }

  return results;
}

// Run every hour via cron
// 0 * * * * /path/to/nself plugin idme refresh-tokens
```

#### Lazy Token Refresh

For real-time requests, check token expiry and refresh if needed:

```typescript
async function getValidAccessToken(userId: string): Promise<string> {
  const verification = await db.query(`
    SELECT access_token, refresh_token, token_expires_at
    FROM idme_verifications
    WHERE user_id = $1 AND status = 'verified'
    ORDER BY created_at DESC LIMIT 1
  `, [userId]);

  if (!verification.rows.length) {
    throw new Error('No verification found for user');
  }

  const { access_token, refresh_token, token_expires_at } = verification.rows[0];

  // Check if token expires in next 5 minutes
  const expiresIn = new Date(token_expires_at).getTime() - Date.now();
  if (expiresIn < 5 * 60 * 1000) {
    logger.debug('Access token expiring soon, refreshing', { userId, expiresIn });

    const newTokens = await refreshAccessToken(refresh_token);

    await db.query(`
      UPDATE idme_verifications
      SET access_token = $1,
          refresh_token = $2,
          token_expires_at = $3,
          updated_at = NOW()
      WHERE user_id = $4
    `, [
      newTokens.access_token,
      newTokens.refresh_token,
      new Date(Date.now() + newTokens.expires_in * 1000),
      userId
    ]);

    return newTokens.access_token;
  }

  return access_token;
}
```

### Caching Strategies

#### In-Memory Badge Cache

Cache badge configurations to avoid repeated database queries:

```typescript
import { LRUCache } from 'lru-cache';

const badgeCache = new LRUCache<string, Badge[]>({
  max: 1000,              // Store up to 1000 users
  ttl: 1000 * 60 * 15,    // 15 minute TTL
  allowStale: false,
  updateAgeOnGet: true
});

async function getUserBadges(userId: string): Promise<Badge[]> {
  // Check cache first
  const cached = badgeCache.get(userId);
  if (cached) {
    return cached;
  }

  // Fetch from database
  const badges = await db.query(`
    SELECT * FROM idme_badges
    WHERE user_id = $1 AND active = TRUE
    ORDER BY display_order ASC
  `, [userId]);

  const badgeList = badges.rows;
  badgeCache.set(userId, badgeList);

  return badgeList;
}
```

#### Redis for Distributed Caching

For multi-server deployments, use Redis:

```typescript
import { createClient } from 'redis';

const redis = createClient({
  url: process.env.REDIS_URL,
  socket: {
    connectTimeout: 2000,
    keepAlive: 5000
  }
});

await redis.connect();

async function getCachedVerification(userId: string): Promise<Verification | null> {
  const cached = await redis.get(`verification:${userId}`);
  if (cached) {
    return JSON.parse(cached);
  }

  const verification = await db.getVerification(userId);
  if (verification) {
    // Cache for 5 minutes
    await redis.setEx(`verification:${userId}`, 300, JSON.stringify(verification));
  }

  return verification;
}

// Invalidate cache on updates
async function updateVerification(userId: string, data: any) {
  await db.updateVerification(userId, data);
  await redis.del(`verification:${userId}`);
}
```

### Database Optimization

#### Efficient Indexes

The plugin creates indexes for common queries. Monitor slow queries and add indexes as needed:

```sql
-- Find slow queries
SELECT
  calls,
  total_exec_time,
  mean_exec_time,
  query
FROM pg_stat_statements
WHERE query LIKE '%idme%'
ORDER BY mean_exec_time DESC
LIMIT 10;

-- Add composite indexes for common joins
CREATE INDEX idx_idme_groups_user_type
  ON idme_groups(user_id, group_type)
  WHERE verified = TRUE;

CREATE INDEX idx_idme_badges_user_active
  ON idme_badges(user_id, active)
  WHERE active = TRUE;
```

#### Batch Operations

When syncing multiple users, use batch operations:

```typescript
async function batchInsertGroups(groups: GroupRecord[]) {
  const values = groups.map((g, i) =>
    `($${i*6+1}, $${i*6+2}, $${i*6+3}, $${i*6+4}, $${i*6+5}, $${i*6+6})`
  ).join(',');

  const params = groups.flatMap(g => [
    g.id,
    g.verification_id,
    g.user_id,
    g.group_type,
    g.verified,
    g.verified_at
  ]);

  await db.query(`
    INSERT INTO idme_groups
      (id, verification_id, user_id, group_type, verified, verified_at)
    VALUES ${values}
    ON CONFLICT (id) DO UPDATE SET
      verified = EXCLUDED.verified,
      verified_at = EXCLUDED.verified_at
  `, params);
}
```

### Rate Limiting

Implement rate limiting for public endpoints to prevent abuse:

```typescript
import rateLimit from '@fastify/rate-limit';

// Apply to auth endpoint
server.register(rateLimit, {
  max: 10,                    // 10 requests
  timeWindow: '1 minute',     // per minute
  cache: 10000,               // cache 10k users
  allowList: ['127.0.0.1'],   // whitelist localhost
  redis: redisClient,         // distributed rate limiting
  keyGenerator: (request) => {
    // Rate limit by IP + user agent
    return `${request.ip}:${request.headers['user-agent']}`;
  }
});

server.get('/auth/idme', async (request, reply) => {
  // Generate authorization URL
  const authUrl = generateAuthUrl();
  return reply.redirect(authUrl);
});
```

---

## Security Notes

### OAuth 2.0 Security Best Practices

The ID.me plugin implements OAuth 2.0 with security-first design principles.

#### PKCE (Proof Key for Code Exchange)

PKCE prevents authorization code interception attacks. ID.me supports PKCE for enhanced security:

```typescript
import crypto from 'crypto';

// Generate code verifier and challenge
function generatePKCE() {
  // Code verifier: random 43-128 character string
  const codeVerifier = crypto.randomBytes(32).toString('base64url');

  // Code challenge: SHA256 hash of verifier
  const codeChallenge = crypto
    .createHash('sha256')
    .update(codeVerifier)
    .digest('base64url');

  return {
    codeVerifier,
    codeChallenge,
    codeChallengeMethod: 'S256'
  };
}

// Store code verifier in session for callback
function startAuthFlow(userId: string) {
  const { codeVerifier, codeChallenge, codeChallengeMethod } = generatePKCE();

  // Store verifier in session/cache (NOT in URL)
  await redis.setEx(
    `pkce:${userId}`,
    300, // 5 minute expiry
    codeVerifier
  );

  // Build authorization URL with challenge
  const authUrl = new URL('https://api.id.me/oauth/authorize');
  authUrl.searchParams.append('client_id', process.env.IDME_CLIENT_ID);
  authUrl.searchParams.append('redirect_uri', process.env.IDME_REDIRECT_URI);
  authUrl.searchParams.append('response_type', 'code');
  authUrl.searchParams.append('scope', process.env.IDME_SCOPES);
  authUrl.searchParams.append('code_challenge', codeChallenge);
  authUrl.searchParams.append('code_challenge_method', codeChallengeMethod);
  authUrl.searchParams.append('state', generateState(userId));

  return authUrl.toString();
}

// Exchange code with verifier
async function exchangeCodeWithPKCE(code: string, userId: string) {
  // Retrieve code verifier from session
  const codeVerifier = await redis.get(`pkce:${userId}`);
  if (!codeVerifier) {
    throw new Error('PKCE verification failed: code verifier not found');
  }

  // Exchange authorization code for tokens
  const response = await fetch('https://api.id.me/oauth/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type: 'authorization_code',
      client_id: process.env.IDME_CLIENT_ID,
      code,
      redirect_uri: process.env.IDME_REDIRECT_URI,
      code_verifier: codeVerifier
    })
  });

  // Clean up verifier
  await redis.del(`pkce:${userId}`);

  if (!response.ok) {
    throw new Error('Token exchange failed');
  }

  return await response.json();
}
```

#### State Parameter Protection

The `state` parameter prevents CSRF attacks:

```typescript
// Generate cryptographically secure state parameter
function generateState(userId: string): string {
  const randomBytes = crypto.randomBytes(16).toString('hex');
  const timestamp = Date.now();

  // HMAC to prevent tampering
  const hmac = crypto
    .createHmac('sha256', process.env.STATE_SECRET)
    .update(`${userId}:${timestamp}:${randomBytes}`)
    .digest('hex');

  // Combine into state parameter
  const state = Buffer.from(
    JSON.stringify({ userId, timestamp, randomBytes, hmac })
  ).toString('base64url');

  // Store state with expiry
  await redis.setEx(`state:${state}`, 600, userId); // 10 minute expiry

  return state;
}

// Verify state parameter
async function verifyState(state: string, expectedUserId: string): Promise<boolean> {
  try {
    // Decode state
    const decoded = JSON.parse(Buffer.from(state, 'base64url').toString());
    const { userId, timestamp, randomBytes, hmac } = decoded;

    // Check expiry (10 minutes)
    if (Date.now() - timestamp > 600000) {
      logger.warn('State parameter expired', { state });
      return false;
    }

    // Verify HMAC
    const expectedHmac = crypto
      .createHmac('sha256', process.env.STATE_SECRET)
      .update(`${userId}:${timestamp}:${randomBytes}`)
      .digest('hex');

    if (hmac !== expectedHmac) {
      logger.error('State HMAC verification failed', { state });
      return false;
    }

    // Verify user ID matches
    if (userId !== expectedUserId) {
      logger.error('State user ID mismatch', { state, userId, expectedUserId });
      return false;
    }

    // Check Redis to prevent replay
    const storedUserId = await redis.get(`state:${state}`);
    if (storedUserId !== expectedUserId) {
      logger.error('State not found in cache', { state });
      return false;
    }

    // Delete state to prevent reuse
    await redis.del(`state:${state}`);

    return true;
  } catch (error) {
    logger.error('State verification error', { error: error.message });
    return false;
  }
}
```

### Token Storage Security

**CRITICAL:** Never store tokens in plaintext.

#### Encryption at Rest

Encrypt OAuth tokens before storing in PostgreSQL:

```typescript
import crypto from 'crypto';

const ALGORITHM = 'aes-256-gcm';
const KEY = Buffer.from(process.env.ENCRYPTION_KEY, 'hex'); // 32-byte key

function encryptToken(token: string): string {
  const iv = crypto.randomBytes(16);
  const cipher = crypto.createCipheriv(ALGORITHM, KEY, iv);

  let encrypted = cipher.update(token, 'utf8', 'hex');
  encrypted += cipher.final('hex');

  const authTag = cipher.getAuthTag();

  // Combine iv + authTag + encrypted
  return `${iv.toString('hex')}:${authTag.toString('hex')}:${encrypted}`;
}

function decryptToken(encryptedToken: string): string {
  const [ivHex, authTagHex, encrypted] = encryptedToken.split(':');

  const iv = Buffer.from(ivHex, 'hex');
  const authTag = Buffer.from(authTagHex, 'hex');

  const decipher = crypto.createDecipheriv(ALGORITHM, KEY, iv);
  decipher.setAuthTag(authTag);

  let decrypted = decipher.update(encrypted, 'hex', 'utf8');
  decrypted += decipher.final('utf8');

  return decrypted;
}

// Use in database operations
async function storeVerification(data: VerificationData) {
  await db.query(`
    INSERT INTO idme_verifications
      (id, user_id, access_token, refresh_token, ...)
    VALUES ($1, $2, $3, $4, ...)
  `, [
    data.id,
    data.userId,
    encryptToken(data.accessToken),
    encryptToken(data.refreshToken),
    // ... other fields
  ]);
}

async function getAccessToken(userId: string): Promise<string> {
  const result = await db.query(`
    SELECT access_token FROM idme_verifications
    WHERE user_id = $1 AND status = 'verified'
    ORDER BY created_at DESC LIMIT 1
  `, [userId]);

  if (!result.rows.length) {
    throw new Error('No verification found');
  }

  return decryptToken(result.rows[0].access_token);
}
```

#### Environment Variable Security

Store encryption keys in secure key management:

```bash
# Generate encryption key (run once)
node -e "console.log(require('crypto').randomBytes(32).toString('hex'))"

# Store in .env (NEVER commit to git)
ENCRYPTION_KEY=your_64_character_hex_key
STATE_SECRET=your_random_secret_for_state_hmac
```

For production, use a key management service:

```typescript
// AWS KMS example
import { KMSClient, DecryptCommand } from '@aws-sdk/client-kms';

const kms = new KMSClient({ region: 'us-east-1' });

async function getEncryptionKey(): Promise<Buffer> {
  const encryptedKey = process.env.ENCRYPTED_ENCRYPTION_KEY;

  const command = new DecryptCommand({
    CiphertextBlob: Buffer.from(encryptedKey, 'base64')
  });

  const response = await kms.send(command);
  return Buffer.from(response.Plaintext);
}
```

### Webhook Security

#### Signature Verification

Always verify webhook signatures to prevent spoofing:

```typescript
import crypto from 'crypto';

function verifyWebhookSignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  // ID.me uses HMAC-SHA256
  const expectedSignature = crypto
    .createHmac('sha256', secret)
    .update(payload)
    .digest('hex');

  // Constant-time comparison to prevent timing attacks
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expectedSignature)
  );
}

// Fastify webhook handler
server.post('/webhook/idme', {
  config: {
    rawBody: true // Preserve raw body for signature verification
  }
}, async (request, reply) => {
  const signature = request.headers['x-idme-signature'] as string;

  if (!signature) {
    logger.warn('Webhook received without signature');
    return reply.code(401).send({ error: 'No signature provided' });
  }

  const isValid = verifyWebhookSignature(
    request.rawBody as string,
    signature,
    process.env.IDME_WEBHOOK_SECRET
  );

  if (!isValid) {
    logger.error('Invalid webhook signature', {
      signature,
      ip: request.ip
    });
    return reply.code(401).send({ error: 'Invalid signature' });
  }

  // Process webhook
  await processWebhookEvent(request.body);

  return reply.code(200).send({ received: true });
});
```

#### Webhook Replay Protection

Prevent replay attacks by tracking processed events:

```typescript
async function processWebhookEvent(event: WebhookEvent) {
  const eventId = event.id;

  // Check if already processed
  const existing = await db.query(`
    SELECT id FROM idme_webhook_events
    WHERE id = $1
  `, [eventId]);

  if (existing.rows.length > 0) {
    logger.warn('Duplicate webhook event ignored', { eventId });
    return;
  }

  // Store and process
  await db.query(`
    INSERT INTO idme_webhook_events
      (id, event_type, data, signature, received_at)
    VALUES ($1, $2, $3, $4, NOW())
  `, [eventId, event.type, JSON.stringify(event.data), event.signature]);

  // Process event
  await handleWebhookEvent(event);

  // Mark as processed
  await db.query(`
    UPDATE idme_webhook_events
    SET processed = TRUE, processed_at = NOW()
    WHERE id = $1
  `, [eventId]);
}
```

### Compliance Considerations

#### HIPAA Compliance

When handling healthcare worker verifications (nurses), ensure HIPAA compliance:

```typescript
// Audit logging for HIPAA
async function logDataAccess(action: string, userId: string, actor: string) {
  await db.query(`
    INSERT INTO audit_log
      (action, user_id, actor, ip_address, timestamp)
    VALUES ($1, $2, $3, $4, NOW())
  `, [action, userId, actor, request.ip]);
}

// Example: Log when accessing nurse verification data
server.get('/api/verifications/:userId', async (request, reply) => {
  const { userId } = request.params;
  const actor = request.user.id; // From auth middleware

  await logDataAccess('VIEW_VERIFICATION', userId, actor);

  const verification = await db.getVerification(userId);
  return verification;
});

// Data retention policy
async function enforceDataRetention() {
  // Delete verifications older than 7 years (HIPAA requirement)
  await db.query(`
    DELETE FROM idme_verifications
    WHERE created_at < NOW() - INTERVAL '7 years'
  `);
}
```

#### FERPA Compliance

For student verifications, comply with FERPA:

```typescript
// Student data requires parental consent for minors
async function verifyStudentWithConsent(userId: string, birthdate: Date) {
  const age = calculateAge(birthdate);

  if (age < 18) {
    // Check for parental consent
    const consent = await db.query(`
      SELECT id FROM parental_consent
      WHERE student_user_id = $1
        AND consent_given = TRUE
        AND expires_at > NOW()
    `, [userId]);

    if (!consent.rows.length) {
      throw new Error('Parental consent required for minor students');
    }
  }

  // Proceed with verification
  return await verifyStudent(userId);
}

// Limit data sharing for educational records
async function getStudentVerification(userId: string, requester: string) {
  // Check if requester is authorized (school official, etc.)
  const authorized = await checkFERPAAuthorization(requester, userId);

  if (!authorized) {
    throw new Error('FERPA authorization required to access student records');
  }

  return await db.getVerification(userId);
}
```

#### Data Minimization

Only request scopes needed for your use case:

```typescript
// Bad: Requesting all scopes
const scopes = 'openid,email,profile,military,veteran,first_responder,government,teacher,student,nurse';

// Good: Only request what you need
const scopes = 'openid,email,profile,military,veteran';

// Dynamic scope selection
function getScopesForUseCase(useCase: string): string {
  const baseScopeMap = {
    'military_discount': 'openid,email,military,veteran',
    'teacher_resources': 'openid,email,teacher',
    'student_pricing': 'openid,email,student',
    'first_responder_access': 'openid,email,first_responder',
    'healthcare_portal': 'openid,email,nurse',
    'government_system': 'openid,email,government'
  };

  return baseScopeMap[useCase] || 'openid,email,profile';
}
```

---

## Advanced Code Examples

### Complete OAuth Implementation

A full production-ready OAuth flow with error handling, PKCE, and state management.

```typescript
import Fastify from 'fastify';
import crypto from 'crypto';
import { Pool } from 'pg';
import Redis from 'ioredis';

const server = Fastify({ logger: true });
const db = new Pool({ connectionString: process.env.DATABASE_URL });
const redis = new Redis(process.env.REDIS_URL);

// Configuration
const config = {
  clientId: process.env.IDME_CLIENT_ID,
  clientSecret: process.env.IDME_CLIENT_SECRET,
  redirectUri: process.env.IDME_REDIRECT_URI,
  scopes: process.env.IDME_SCOPES || 'openid,email,profile',
  baseUrl: process.env.IDME_SANDBOX === 'true'
    ? 'https://api.idmelabs.com'
    : 'https://api.id.me',
  encryptionKey: Buffer.from(process.env.ENCRYPTION_KEY, 'hex'),
  stateSecret: process.env.STATE_SECRET
};

// PKCE utilities
function generatePKCE() {
  const codeVerifier = crypto.randomBytes(32).toString('base64url');
  const codeChallenge = crypto
    .createHash('sha256')
    .update(codeVerifier)
    .digest('base64url');

  return {
    codeVerifier,
    codeChallenge,
    codeChallengeMethod: 'S256'
  };
}

// State utilities
function generateState(userId: string): string {
  const randomBytes = crypto.randomBytes(16).toString('hex');
  const timestamp = Date.now();

  const hmac = crypto
    .createHmac('sha256', config.stateSecret)
    .update(`${userId}:${timestamp}:${randomBytes}`)
    .digest('hex');

  return Buffer.from(
    JSON.stringify({ userId, timestamp, randomBytes, hmac })
  ).toString('base64url');
}

async function verifyState(state: string, expectedUserId: string): Promise<boolean> {
  try {
    const decoded = JSON.parse(Buffer.from(state, 'base64url').toString());
    const { userId, timestamp, randomBytes, hmac } = decoded;

    // Check expiry (10 minutes)
    if (Date.now() - timestamp > 600000) {
      return false;
    }

    // Verify HMAC
    const expectedHmac = crypto
      .createHmac('sha256', config.stateSecret)
      .update(`${userId}:${timestamp}:${randomBytes}`)
      .digest('hex');

    if (hmac !== expectedHmac || userId !== expectedUserId) {
      return false;
    }

    // Check Redis to prevent replay
    const storedUserId = await redis.get(`state:${state}`);
    if (storedUserId !== expectedUserId) {
      return false;
    }

    await redis.del(`state:${state}`);
    return true;
  } catch {
    return false;
  }
}

// Encryption utilities
function encryptToken(token: string): string {
  const iv = crypto.randomBytes(16);
  const cipher = crypto.createCipheriv('aes-256-gcm', config.encryptionKey, iv);

  let encrypted = cipher.update(token, 'utf8', 'hex');
  encrypted += cipher.final('hex');

  const authTag = cipher.getAuthTag();

  return `${iv.toString('hex')}:${authTag.toString('hex')}:${encrypted}`;
}

function decryptToken(encryptedToken: string): string {
  const [ivHex, authTagHex, encrypted] = encryptedToken.split(':');

  const iv = Buffer.from(ivHex, 'hex');
  const authTag = Buffer.from(authTagHex, 'hex');

  const decipher = crypto.createDecipheriv('aes-256-gcm', config.encryptionKey, iv);
  decipher.setAuthTag(authTag);

  let decrypted = decipher.update(encrypted, 'hex', 'utf8');
  decrypted += decipher.final('utf8');

  return decrypted;
}

// OAuth endpoints
server.get('/auth/idme', async (request, reply) => {
  const userId = request.query.user_id as string;

  if (!userId) {
    return reply.code(400).send({ error: 'user_id required' });
  }

  // Generate PKCE
  const { codeVerifier, codeChallenge, codeChallengeMethod } = generatePKCE();

  // Store code verifier
  await redis.setEx(`pkce:${userId}`, 300, codeVerifier);

  // Generate state
  const state = generateState(userId);
  await redis.setEx(`state:${state}`, 600, userId);

  // Build authorization URL
  const authUrl = new URL(`${config.baseUrl}/oauth/authorize`);
  authUrl.searchParams.append('client_id', config.clientId);
  authUrl.searchParams.append('redirect_uri', config.redirectUri);
  authUrl.searchParams.append('response_type', 'code');
  authUrl.searchParams.append('scope', config.scopes);
  authUrl.searchParams.append('code_challenge', codeChallenge);
  authUrl.searchParams.append('code_challenge_method', codeChallengeMethod);
  authUrl.searchParams.append('state', state);

  return reply.redirect(authUrl.toString());
});

server.get('/callback/idme', async (request, reply) => {
  const { code, state, error, error_description } = request.query as any;

  // Handle OAuth errors
  if (error) {
    server.log.error('OAuth error', { error, error_description });
    return reply.redirect(`/error?message=${encodeURIComponent(error_description)}`);
  }

  if (!code || !state) {
    return reply.code(400).send({ error: 'Missing code or state' });
  }

  try {
    // Decode state to get user ID
    const decoded = JSON.parse(Buffer.from(state, 'base64url').toString());
    const userId = decoded.userId;

    // Verify state
    const stateValid = await verifyState(state, userId);
    if (!stateValid) {
      server.log.error('Invalid state parameter', { state });
      return reply.code(400).send({ error: 'Invalid state parameter' });
    }

    // Get code verifier
    const codeVerifier = await redis.get(`pkce:${userId}`);
    if (!codeVerifier) {
      return reply.code(400).send({ error: 'PKCE verification failed' });
    }

    // Exchange code for tokens
    const tokenResponse = await fetch(`${config.baseUrl}/oauth/token`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        grant_type: 'authorization_code',
        client_id: config.clientId,
        client_secret: config.clientSecret,
        code,
        redirect_uri: config.redirectUri,
        code_verifier: codeVerifier
      })
    });

    if (!tokenResponse.ok) {
      const errorData = await tokenResponse.json();
      server.log.error('Token exchange failed', errorData);
      return reply.code(500).send({ error: 'Token exchange failed' });
    }

    const tokens = await tokenResponse.json();

    // Clean up PKCE
    await redis.del(`pkce:${userId}`);

    // Fetch user data in parallel
    const [profile, groups, attributes] = await Promise.all([
      fetchUserProfile(tokens.access_token),
      fetchVerificationGroups(tokens.access_token),
      fetchUserAttributes(tokens.access_token)
    ]);

    // Store verification in database
    const verificationId = crypto.randomUUID();
    const tokenExpiresAt = new Date(Date.now() + tokens.expires_in * 1000);

    await db.query(`
      INSERT INTO idme_verifications
        (id, user_id, idme_user_id, email, first_name, last_name,
         access_token, refresh_token, token_expires_at,
         verified, verified_at, status, created_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11, NOW())
    `, [
      verificationId,
      userId,
      profile.sub,
      profile.email,
      profile.given_name,
      profile.family_name,
      encryptToken(tokens.access_token),
      encryptToken(tokens.refresh_token),
      tokenExpiresAt,
      true,
      'verified'
    ]);

    // Store groups
    for (const group of groups) {
      await db.query(`
        INSERT INTO idme_groups
          (id, verification_id, user_id, group_type, verified, verified_at)
        VALUES ($1, $2, $3, $4, $5, $6)
      `, [
        crypto.randomUUID(),
        verificationId,
        userId,
        group.type,
        group.verified,
        group.verified ? new Date() : null
      ]);

      // Create badge
      await db.query(`
        INSERT INTO idme_badges
          (id, user_id, group_type, badge_name, icon, color, active)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
      `, [
        crypto.randomUUID(),
        userId,
        group.type,
        getBadgeName(group.type),
        getBadgeIcon(group.type),
        getBadgeColor(group.type),
        true
      ]);
    }

    // Store attributes
    for (const attr of attributes) {
      await db.query(`
        INSERT INTO idme_attributes
          (id, verification_id, user_id, attribute_name, attribute_value, verified)
        VALUES ($1, $2, $3, $4, $5, $6)
      `, [
        crypto.randomUUID(),
        verificationId,
        userId,
        attr.name,
        attr.value,
        attr.verified
      ]);
    }

    server.log.info('Verification completed', { userId, groups: groups.length });

    // Redirect to success page
    return reply.redirect(`/success?groups=${groups.map(g => g.type).join(',')}`);

  } catch (error) {
    server.log.error('Callback error', error);
    return reply.code(500).send({ error: 'Verification failed' });
  }
});

// Helper functions
async function fetchUserProfile(accessToken: string) {
  const response = await fetch(`${config.baseUrl}/api/public/v3/userinfo.json`, {
    headers: { Authorization: `Bearer ${accessToken}` }
  });
  return await response.json();
}

async function fetchVerificationGroups(accessToken: string) {
  const response = await fetch(`${config.baseUrl}/api/public/v3/groups.json`, {
    headers: { Authorization: `Bearer ${accessToken}` }
  });
  const data = await response.json();
  return data.groups || [];
}

async function fetchUserAttributes(accessToken: string) {
  const response = await fetch(`${config.baseUrl}/api/public/v3/attributes.json`, {
    headers: { Authorization: `Bearer ${accessToken}` }
  });
  const data = await response.json();
  return data.attributes || [];
}

function getBadgeName(groupType: string): string {
  const names = {
    military: 'Active Military',
    veteran: 'Veteran',
    first_responder: 'First Responder',
    government: 'Government Employee',
    teacher: 'Teacher',
    student: 'Student',
    nurse: 'Healthcare Worker'
  };
  return names[groupType] || groupType;
}

function getBadgeIcon(groupType: string): string {
  const icons = {
    military: 'shield',
    veteran: 'star',
    first_responder: 'fire',
    government: 'building',
    teacher: 'book',
    student: 'graduation-cap',
    nurse: 'heart-pulse'
  };
  return icons[groupType] || 'check';
}

function getBadgeColor(groupType: string): string {
  const colors = {
    military: '#1e40af',
    veteran: '#dc2626',
    first_responder: '#ea580c',
    government: '#059669',
    teacher: '#7c3aed',
    student: '#0891b2',
    nurse: '#db2777'
  };
  return colors[groupType] || '#6b7280';
}

// Start server
server.listen({ port: 3010, host: '0.0.0.0' }, (err, address) => {
  if (err) {
    server.log.error(err);
    process.exit(1);
  }
  server.log.info(`Server listening at ${address}`);
});
```

### Multi-Group Verification

Check if a user is verified for multiple groups:

```typescript
async function checkMultiGroupVerification(
  userId: string,
  requiredGroups: string[]
): Promise<{ verified: boolean; missingGroups: string[] }> {
  const result = await db.query(`
    SELECT group_type
    FROM idme_groups
    WHERE user_id = $1
      AND verified = TRUE
      AND group_type = ANY($2)
      AND (expires_at IS NULL OR expires_at > NOW())
  `, [userId, requiredGroups]);

  const verifiedGroups = result.rows.map(row => row.group_type);
  const missingGroups = requiredGroups.filter(g => !verifiedGroups.includes(g));

  return {
    verified: missingGroups.length === 0,
    missingGroups
  };
}

// Usage example: Military or veteran discount
const { verified, missingGroups } = await checkMultiGroupVerification(
  userId,
  ['military', 'veteran']
);

if (!verified) {
  console.log(`User needs verification for: ${missingGroups.join(', ')}`);
}
```

### Badge Display Component

React component for displaying verification badges:

```typescript
import React from 'react';

interface Badge {
  id: string;
  group_type: string;
  badge_name: string;
  icon: string;
  color: string;
  active: boolean;
}

interface VerificationBadgesProps {
  userId: string;
}

export function VerificationBadges({ userId }: VerificationBadgesProps) {
  const [badges, setBadges] = React.useState<Badge[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    async function loadBadges() {
      try {
        const response = await fetch(`/api/badges/${userId}`);
        const data = await response.json();
        setBadges(data.badges);
      } catch (error) {
        console.error('Failed to load badges:', error);
      } finally {
        setLoading(false);
      }
    }

    loadBadges();
  }, [userId]);

  if (loading) {
    return <div className="badge-loading">Loading badges...</div>;
  }

  if (badges.length === 0) {
    return null;
  }

  return (
    <div className="verification-badges">
      {badges.map(badge => (
        <div
          key={badge.id}
          className="badge"
          style={{
            backgroundColor: badge.color,
            borderColor: badge.color
          }}
          title={`Verified ${badge.badge_name}`}
        >
          <i className={`icon-${badge.icon}`} />
          <span>{badge.badge_name}</span>
        </div>
      ))}
    </div>
  );
}

// CSS
const styles = `
.verification-badges {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.badge {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 16px;
  font-size: 14px;
  font-weight: 500;
  color: white;
  border: 2px solid;
}

.badge i {
  font-size: 16px;
}

.badge-loading {
  color: #6b7280;
  font-style: italic;
}
`;
```

### Frontend Integration

Complete frontend flow with React:

```typescript
import React from 'react';

interface VerificationStatus {
  verified: boolean;
  groups: string[];
  badges: Badge[];
  verified_at: string | null;
}

export function IDmeVerification() {
  const [status, setStatus] = React.useState<VerificationStatus | null>(null);
  const [loading, setLoading] = React.useState(true);
  const userId = getCurrentUserId(); // Your auth system

  React.useEffect(() => {
    loadVerificationStatus();
  }, []);

  async function loadVerificationStatus() {
    try {
      const response = await fetch(`/api/verifications/${userId}`);
      if (response.ok) {
        const data = await response.json();
        setStatus(data);
      }
    } catch (error) {
      console.error('Failed to load verification status:', error);
    } finally {
      setLoading(false);
    }
  }

  function startVerification() {
    // Redirect to OAuth flow
    window.location.href = `/auth/idme?user_id=${userId}`;
  }

  if (loading) {
    return <div>Loading verification status...</div>;
  }

  if (!status || !status.verified) {
    return (
      <div className="verification-prompt">
        <h3>Verify Your Identity</h3>
        <p>Get exclusive benefits by verifying your status:</p>
        <ul>
          <li>Military and Veteran discounts</li>
          <li>First Responder benefits</li>
          <li>Student pricing</li>
          <li>Teacher resources</li>
        </ul>
        <button onClick={startVerification} className="verify-button">
          Verify with ID.me
        </button>
      </div>
    );
  }

  return (
    <div className="verification-status">
      <h3>✓ Verified</h3>
      <p>Verified on {new Date(status.verified_at).toLocaleDateString()}</p>
      <VerificationBadges userId={userId} />

      {status.groups.includes('military') && (
        <div className="benefit-card">
          <h4>Military Discount Active</h4>
          <p>You receive 15% off all purchases</p>
        </div>
      )}

      {status.groups.includes('teacher') && (
        <div className="benefit-card">
          <h4>Teacher Resources Unlocked</h4>
          <p>Access exclusive teaching materials</p>
        </div>
      )}
    </div>
  );
}

function getCurrentUserId(): string {
  // Get from your auth system
  return document.querySelector('[data-user-id]')?.getAttribute('data-user-id') || '';
}
```

---

## Monitoring & Alerting

### Key Metrics to Track

Monitor these metrics to ensure healthy verification operations:

#### Verification Rates

```sql
-- Verification success rate (last 24 hours)
SELECT
  COUNT(*) FILTER (WHERE status = 'verified') AS successful,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed,
  COUNT(*) FILTER (WHERE status = 'pending') AS pending,
  ROUND(
    COUNT(*) FILTER (WHERE status = 'verified')::decimal /
    NULLIF(COUNT(*), 0) * 100,
    2
  ) AS success_rate_pct
FROM idme_verifications
WHERE created_at > NOW() - INTERVAL '24 hours';

-- Group verification breakdown
SELECT
  group_type,
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE verified = TRUE) AS verified,
  ROUND(
    COUNT(*) FILTER (WHERE verified = TRUE)::decimal /
    NULLIF(COUNT(*), 0) * 100,
    2
  ) AS verification_rate_pct
FROM idme_groups
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY group_type
ORDER BY total DESC;
```

#### Failed Verification Attempts

Track and alert on failed verifications:

```typescript
// Monitor failed verifications
async function checkFailedVerifications(): Promise<Alert[]> {
  const alerts: Alert[] = [];

  // Check recent failure rate
  const result = await db.query(`
    SELECT
      COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
      COUNT(*) AS total_count,
      ROUND(
        COUNT(*) FILTER (WHERE status = 'failed')::decimal /
        NULLIF(COUNT(*), 0) * 100,
        2
      ) AS failure_rate
    FROM idme_verifications
    WHERE created_at > NOW() - INTERVAL '1 hour'
  `);

  const { failed_count, total_count, failure_rate } = result.rows[0];

  // Alert if failure rate > 10% and at least 10 attempts
  if (total_count >= 10 && failure_rate > 10) {
    alerts.push({
      severity: 'warning',
      message: `High verification failure rate: ${failure_rate}% (${failed_count}/${total_count})`,
      timestamp: new Date()
    });
  }

  // Alert if failure rate > 25%
  if (total_count >= 10 && failure_rate > 25) {
    alerts.push({
      severity: 'critical',
      message: `Critical verification failure rate: ${failure_rate}% - possible ID.me outage`,
      timestamp: new Date()
    });
  }

  return alerts;
}

// Run every 5 minutes via cron
// */5 * * * * /path/to/nself plugin idme monitor failures
```

#### Token Expiry Tracking

Monitor tokens approaching expiration:

```sql
-- Tokens expiring in next 24 hours
SELECT
  COUNT(*) AS expiring_soon,
  COUNT(*) FILTER (WHERE token_expires_at < NOW() + INTERVAL '1 hour') AS expiring_very_soon,
  COUNT(*) FILTER (WHERE token_expires_at < NOW()) AS already_expired
FROM idme_verifications
WHERE status = 'verified'
  AND token_expires_at < NOW() + INTERVAL '24 hours';

-- Users with expired tokens
SELECT
  user_id,
  email,
  token_expires_at,
  AGE(NOW(), token_expires_at) AS expired_for
FROM idme_verifications
WHERE status = 'verified'
  AND token_expires_at < NOW()
ORDER BY token_expires_at DESC
LIMIT 100;
```

#### Webhook Health

Monitor webhook processing:

```sql
-- Webhook processing status
SELECT
  event_type,
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE processed = TRUE) AS processed,
  COUNT(*) FILTER (WHERE processed = FALSE) AS pending,
  COUNT(*) FILTER (WHERE error IS NOT NULL) AS errors,
  AVG(EXTRACT(EPOCH FROM (processed_at - received_at))) AS avg_processing_time_seconds
FROM idme_webhook_events
WHERE received_at > NOW() - INTERVAL '24 hours'
GROUP BY event_type
ORDER BY total DESC;

-- Failed webhooks requiring attention
SELECT
  id,
  event_type,
  received_at,
  error,
  data->>'idme_user_id' AS idme_user_id
FROM idme_webhook_events
WHERE processed = FALSE
  AND error IS NOT NULL
  AND received_at > NOW() - INTERVAL '1 day'
ORDER BY received_at DESC;
```

### Prometheus Metrics

Export metrics for Prometheus monitoring:

```typescript
import promClient from 'prom-client';

// Create metrics
const verificationCounter = new promClient.Counter({
  name: 'idme_verifications_total',
  help: 'Total number of verification attempts',
  labelNames: ['status', 'group_type']
});

const verificationDuration = new promClient.Histogram({
  name: 'idme_verification_duration_seconds',
  help: 'Verification flow duration',
  buckets: [0.5, 1, 2, 5, 10, 30]
});

const tokenRefreshCounter = new promClient.Counter({
  name: 'idme_token_refresh_total',
  help: 'Total token refresh attempts',
  labelNames: ['status']
});

const webhookCounter = new promClient.Counter({
  name: 'idme_webhooks_total',
  help: 'Total webhook events received',
  labelNames: ['event_type', 'status']
});

// Instrument verification flow
async function handleOAuthCallback(code: string, state: string) {
  const startTime = Date.now();
  let status = 'failed';
  let groups: string[] = [];

  try {
    // ... verification logic ...
    status = 'success';
    groups = ['military', 'veteran']; // From verification result

    // Record success for each group
    for (const group of groups) {
      verificationCounter.inc({ status: 'success', group_type: group });
    }
  } catch (error) {
    verificationCounter.inc({ status: 'failed', group_type: 'unknown' });
    throw error;
  } finally {
    const duration = (Date.now() - startTime) / 1000;
    verificationDuration.observe(duration);
  }
}

// Instrument token refresh
async function refreshAccessToken(refreshToken: string) {
  try {
    const newTokens = await performTokenRefresh(refreshToken);
    tokenRefreshCounter.inc({ status: 'success' });
    return newTokens;
  } catch (error) {
    tokenRefreshCounter.inc({ status: 'failed' });
    throw error;
  }
}

// Instrument webhook processing
async function processWebhook(event: WebhookEvent) {
  try {
    await handleWebhookEvent(event);
    webhookCounter.inc({
      event_type: event.type,
      status: 'processed'
    });
  } catch (error) {
    webhookCounter.inc({
      event_type: event.type,
      status: 'failed'
    });
    throw error;
  }
}

// Expose metrics endpoint
server.get('/metrics', async (request, reply) => {
  reply.type('text/plain');
  return promClient.register.metrics();
});
```

### Grafana Dashboards

Create monitoring dashboards with these queries:

```promql
# Verification success rate (%)
rate(idme_verifications_total{status="success"}[5m]) /
rate(idme_verifications_total[5m]) * 100

# Verification duration p95
histogram_quantile(0.95,
  rate(idme_verification_duration_seconds_bucket[5m])
)

# Token refresh failure rate
rate(idme_token_refresh_total{status="failed"}[5m])

# Webhook processing lag
idme_webhooks_total{status="pending"}

# Verifications by group type
sum by (group_type) (
  rate(idme_verifications_total{status="success"}[1h])
)
```

### Alert Rules

Configure alerting for critical issues:

```yaml
# Prometheus alert rules
groups:
  - name: idme
    interval: 1m
    rules:
      # High verification failure rate
      - alert: HighVerificationFailureRate
        expr: |
          rate(idme_verifications_total{status="failed"}[5m]) /
          rate(idme_verifications_total[5m]) > 0.25
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High ID.me verification failure rate"
          description: "{{ $value | humanizePercentage }} of verifications are failing"

      # Slow verification flow
      - alert: SlowVerificationFlow
        expr: |
          histogram_quantile(0.95,
            rate(idme_verification_duration_seconds_bucket[5m])
          ) > 10
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "ID.me verification flow is slow"
          description: "P95 latency is {{ $value }}s"

      # Token refresh failures
      - alert: TokenRefreshFailures
        expr: |
          rate(idme_token_refresh_total{status="failed"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "ID.me token refresh failures detected"
          description: "{{ $value }} token refreshes per second are failing"

      # Webhook processing lag
      - alert: WebhookProcessingLag
        expr: idme_webhooks_total{status="pending"} > 100
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "ID.me webhook processing lag"
          description: "{{ $value }} webhooks are pending processing"

      # No verifications in last hour
      - alert: NoRecentVerifications
        expr: |
          rate(idme_verifications_total[1h]) == 0
        for: 1h
        labels:
          severity: info
        annotations:
          summary: "No ID.me verifications in the last hour"
          description: "This may be expected during off-hours"
```

### Health Check Endpoint

Comprehensive health check for monitoring:

```typescript
server.get('/health', async (request, reply) => {
  const health = {
    status: 'healthy',
    timestamp: new Date().toISOString(),
    checks: {}
  };

  // Check database
  try {
    await db.query('SELECT 1');
    health.checks.database = { status: 'up' };
  } catch (error) {
    health.status = 'unhealthy';
    health.checks.database = {
      status: 'down',
      error: error.message
    };
  }

  // Check Redis
  try {
    await redis.ping();
    health.checks.redis = { status: 'up' };
  } catch (error) {
    health.status = 'degraded';
    health.checks.redis = {
      status: 'down',
      error: error.message
    };
  }

  // Check ID.me API
  try {
    const response = await fetch(`${config.baseUrl}/.well-known/openid-configuration`);
    if (response.ok) {
      health.checks.idme_api = { status: 'up' };
    } else {
      health.checks.idme_api = {
        status: 'down',
        statusCode: response.status
      };
    }
  } catch (error) {
    health.status = 'unhealthy';
    health.checks.idme_api = {
      status: 'down',
      error: error.message
    };
  }

  // Check recent verification rate
  try {
    const result = await db.query(`
      SELECT COUNT(*) as count
      FROM idme_verifications
      WHERE created_at > NOW() - INTERVAL '1 hour'
    `);
    health.checks.recent_verifications = {
      count: parseInt(result.rows[0].count),
      status: 'ok'
    };
  } catch (error) {
    health.checks.recent_verifications = {
      status: 'error',
      error: error.message
    };
  }

  // Set appropriate status code
  const statusCode = health.status === 'unhealthy' ? 503 : 200;

  return reply.code(statusCode).send(health);
});
```

---

## Use Cases

### Military Discount E-Commerce

Provide discounts to active duty military members:

```typescript
async function applyMilitaryDiscount(userId: string, cartTotal: number) {
  // Check military verification
  const verification = await db.query(`
    SELECT g.verified, g.verified_at
    FROM idme_groups g
    JOIN idme_verifications v ON g.verification_id = v.id
    WHERE v.user_id = $1
      AND g.group_type = 'military'
      AND g.verified = TRUE
      AND v.status = 'verified'
  `, [userId]);

  if (!verification.rows.length) {
    return {
      eligible: false,
      discount: 0,
      message: 'Military verification required'
    };
  }

  // Apply 15% military discount
  const discountRate = 0.15;
  const discountAmount = cartTotal * discountRate;

  return {
    eligible: true,
    discount: discountAmount,
    finalTotal: cartTotal - discountAmount,
    message: 'Thank you for your service! 15% military discount applied.',
    verifiedSince: verification.rows[0].verified_at
  };
}

// Usage in checkout flow
const cart = { total: 100.00 };
const discount = await applyMilitaryDiscount(userId, cart.total);

if (discount.eligible) {
  console.log(`Original: $${cart.total}`);
  console.log(`Discount: -$${discount.discount.toFixed(2)}`);
  console.log(`Final: $${discount.finalTotal.toFixed(2)}`);
}
```

### Veteran Benefits Portal

Grant access to veteran-specific resources:

```typescript
async function checkVeteranAccess(userId: string) {
  const result = await db.query(`
    SELECT
      v.verified,
      v.verified_at,
      a.attribute_name,
      a.attribute_value
    FROM idme_verifications v
    JOIN idme_groups g ON v.id = g.verification_id
    LEFT JOIN idme_attributes a ON v.id = a.verification_id
    WHERE v.user_id = $1
      AND g.group_type = 'veteran'
      AND g.verified = TRUE
      AND v.status = 'verified'
  `, [userId]);

  if (!result.rows.length) {
    return {
      hasAccess: false,
      message: 'Veteran verification required to access these benefits'
    };
  }

  // Extract veteran details
  const attributes = {};
  for (const row of result.rows) {
    if (row.attribute_name) {
      attributes[row.attribute_name] = row.attribute_value;
    }
  }

  return {
    hasAccess: true,
    verifiedAt: result.rows[0].verified_at,
    branch: attributes.branch,
    serviceEra: attributes.service_era,
    benefits: [
      'VA Home Loan Information',
      'Healthcare Enrollment',
      'Education Benefits (GI Bill)',
      'Career Services',
      'Mental Health Resources'
    ]
  };
}

// Usage in portal
const access = await checkVeteranAccess(userId);

if (access.hasAccess) {
  console.log(`Welcome, ${access.branch} Veteran!`);
  console.log(`Service Era: ${access.serviceEra}`);
  console.log('Available benefits:', access.benefits);
}
```

### First Responder Emergency Access

Grant priority access to first responders:

```typescript
async function grantFirstResponderAccess(userId: string) {
  const result = await db.query(`
    SELECT
      g.verified,
      a.attribute_name,
      a.attribute_value
    FROM idme_groups g
    JOIN idme_verifications v ON g.verification_id = v.id
    LEFT JOIN idme_attributes a ON v.id = a.verification_id
    WHERE v.user_id = $1
      AND g.group_type = 'first_responder'
      AND g.verified = TRUE
  `, [userId]);

  if (!result.rows.length) {
    return { granted: false };
  }

  // Extract first responder details
  const attributes = {};
  for (const row of result.rows) {
    if (row.attribute_name) {
      attributes[row.attribute_name] = row.attribute_value;
    }
  }

  return {
    granted: true,
    department: attributes.department,
    role: attributes.role,
    accessLevel: 'priority',
    features: [
      'Emergency Alerts',
      'Priority Support',
      'Resource Database',
      'Training Materials',
      '24/7 Hotline Access'
    ]
  };
}

// Usage in emergency system
const access = await grantFirstResponderAccess(userId);

if (access.granted) {
  console.log(`First Responder Verified: ${access.role}`);
  console.log(`Department: ${access.department}`);
  console.log(`Access Level: ${access.accessLevel}`);
}
```

### Teacher Resource Platform

Unlock educational resources for teachers:

```typescript
async function unlockTeacherResources(userId: string) {
  const result = await db.query(`
    SELECT
      g.verified,
      a.attribute_name,
      a.attribute_value
    FROM idme_groups g
    JOIN idme_verifications v ON g.verification_id = v.id
    LEFT JOIN idme_attributes a ON v.id = a.verification_id
    WHERE v.user_id = $1
      AND g.group_type = 'teacher'
      AND g.verified = TRUE
  `, [userId]);

  if (!result.rows.length) {
    return { unlocked: false };
  }

  const attributes = {};
  for (const row of result.rows) {
    if (row.attribute_name) {
      attributes[row.attribute_name] = row.attribute_value;
    }
  }

  return {
    unlocked: true,
    school: attributes.school,
    subject: attributes.subject,
    resources: [
      'Lesson Plan Library (10,000+ plans)',
      'Educational Software (50% discount)',
      'Classroom Management Tools',
      'Assessment Generators',
      'Parent Communication Templates',
      'Professional Development Courses'
    ],
    discount: {
      rate: 0.30, // 30% off
      message: 'Thank you for educating our future!'
    }
  };
}

// Usage in educational platform
const resources = await unlockTeacherResources(userId);

if (resources.unlocked) {
  console.log(`Welcome, ${resources.subject} Teacher!`);
  console.log(`School: ${resources.school}`);
  console.log(`Unlocked Resources: ${resources.resources.length}`);
  console.log(`Special Discount: ${resources.discount.rate * 100}%`);
}
```

### Student Pricing System

Offer student pricing for software and services:

```typescript
async function applyStudentPricing(userId: string, productPrice: number) {
  const result = await db.query(`
    SELECT
      g.verified,
      g.expires_at,
      a.attribute_name,
      a.attribute_value
    FROM idme_groups g
    JOIN idme_verifications v ON g.verification_id = v.id
    LEFT JOIN idme_attributes a ON v.id = a.verification_id
    WHERE v.user_id = $1
      AND g.group_type = 'student'
      AND g.verified = TRUE
      AND (g.expires_at IS NULL OR g.expires_at > NOW())
  `, [userId]);

  if (!result.rows.length) {
    return {
      eligible: false,
      price: productPrice,
      message: 'Student verification required for discounted pricing'
    };
  }

  const attributes = {};
  for (const row of result.rows) {
    if (row.attribute_name) {
      attributes[row.attribute_name] = row.attribute_value;
    }
  }

  // Apply 50% student discount
  const studentPrice = productPrice * 0.50;

  return {
    eligible: true,
    originalPrice: productPrice,
    studentPrice: studentPrice,
    savings: productPrice - studentPrice,
    school: attributes.school,
    graduationYear: attributes.graduation_year,
    expiresAt: result.rows[0].expires_at,
    message: 'Student discount applied!'
  };
}

// Usage in pricing page
const product = { name: 'Pro Plan', price: 99.99 };
const pricing = await applyStudentPricing(userId, product.price);

if (pricing.eligible) {
  console.log(`Original Price: $${pricing.originalPrice}/mo`);
  console.log(`Student Price: $${pricing.studentPrice}/mo`);
  console.log(`You Save: $${pricing.savings}/mo`);
  console.log(`School: ${pricing.school}`);
}
```

### Healthcare Worker Portal

Grant access to healthcare resources:

```typescript
async function grantHealthcareAccess(userId: string) {
  const result = await db.query(`
    SELECT
      g.verified,
      a.attribute_name,
      a.attribute_value
    FROM idme_groups g
    JOIN idme_verifications v ON g.verification_id = v.id
    LEFT JOIN idme_attributes a ON v.id = a.verification_id
    WHERE v.user_id = $1
      AND g.group_type = 'nurse'
      AND g.verified = TRUE
  `, [userId]);

  if (!result.rows.length) {
    return { granted: false };
  }

  const attributes = {};
  for (const row of result.rows) {
    if (row.attribute_name) {
      attributes[row.attribute_name] = row.attribute_value;
    }
  }

  return {
    granted: true,
    specialty: attributes.specialty,
    license: attributes.license,
    resources: [
      'Medical Reference Library',
      'Continuing Education Credits',
      'Peer Support Network',
      'Shift Management Tools',
      'Mental Health Resources',
      'Equipment Discounts (20% off)'
    ]
  };
}

// Usage in healthcare platform
const access = await grantHealthcareAccess(userId);

if (access.granted) {
  console.log(`Healthcare Worker Verified`);
  console.log(`Specialty: ${access.specialty}`);
  console.log(`License: ${access.license}`);
  console.log(`Access Granted to: ${access.resources.length} resources`);
}
```

### Government Employee System

Secure access for government workers:

```typescript
async function verifyGovernmentEmployee(userId: string) {
  const result = await db.query(`
    SELECT
      g.verified,
      a.attribute_name,
      a.attribute_value
    FROM idme_groups g
    JOIN idme_verifications v ON g.verification_id = v.id
    LEFT JOIN idme_attributes a ON v.id = a.verification_id
    WHERE v.user_id = $1
      AND g.group_type = 'government'
      AND g.verified = TRUE
  `, [userId]);

  if (!result.rows.length) {
    return { verified: false };
  }

  const attributes = {};
  for (const row of result.rows) {
    if (row.attribute_name) {
      attributes[row.attribute_name] = row.attribute_value;
    }
  }

  return {
    verified: true,
    agency: attributes.agency,
    level: attributes.level, // federal, state, local
    clearanceLevel: determineClearanceLevel(attributes),
    access: [
      'Secure Document Portal',
      'Interagency Communication',
      'Policy Database',
      'Training & Compliance',
      'Procurement System'
    ]
  };
}

function determineClearanceLevel(attributes: any): string {
  // Determine access level based on agency and role
  if (attributes.level === 'federal') {
    return 'Level 3 - Federal Access';
  } else if (attributes.level === 'state') {
    return 'Level 2 - State Access';
  } else {
    return 'Level 1 - Local Access';
  }
}

// Usage in government portal
const employee = await verifyGovernmentEmployee(userId);

if (employee.verified) {
  console.log(`Government Employee Verified`);
  console.log(`Agency: ${employee.agency}`);
  console.log(`Level: ${employee.level}`);
  console.log(`Clearance: ${employee.clearanceLevel}`);
}
```

### Multi-Group Exclusive Access

Require multiple verifications for exclusive access:

```typescript
async function checkExclusiveAccess(userId: string) {
  // Require both military AND first responder status
  const result = await db.query(`
    SELECT
      v.user_id,
      ARRAY_AGG(DISTINCT g.group_type) FILTER (WHERE g.verified = TRUE) AS verified_groups
    FROM idme_verifications v
    JOIN idme_groups g ON v.id = g.verification_id
    WHERE v.user_id = $1
      AND v.status = 'verified'
    GROUP BY v.user_id
  `, [userId]);

  if (!result.rows.length) {
    return { granted: false, reason: 'No verifications found' };
  }

  const verifiedGroups = result.rows[0].verified_groups || [];

  // Check if user has both required verifications
  const hasMilitary = verifiedGroups.includes('military');
  const hasFirstResponder = verifiedGroups.includes('first_responder');

  if (hasMilitary && hasFirstResponder) {
    return {
      granted: true,
      message: 'Exclusive access granted: Military First Responder',
      perks: [
        'Priority Emergency Response Support',
        'Dual-status Benefits',
        'Specialized Training Access',
        'Extended Discount (25%)',
        'VIP Support Line'
      ]
    };
  }

  return {
    granted: false,
    reason: 'Requires both military and first responder verification',
    missing: [
      !hasMilitary && 'military',
      !hasFirstResponder && 'first_responder'
    ].filter(Boolean)
  };
}

// Usage
const access = await checkExclusiveAccess(userId);

if (access.granted) {
  console.log(access.message);
  console.log('Exclusive Perks:', access.perks);
} else {
  console.log(`Access Denied: ${access.reason}`);
  console.log(`Missing verifications: ${access.missing.join(', ')}`);
}
```

### Tiered Membership System

Create tiered benefits based on verification groups:

```typescript
async function determineMembershipTier(userId: string) {
  const result = await db.query(`
    SELECT
      ARRAY_AGG(DISTINCT g.group_type) FILTER (WHERE g.verified = TRUE) AS verified_groups,
      COUNT(DISTINCT g.group_type) FILTER (WHERE g.verified = TRUE) AS group_count
    FROM idme_groups g
    JOIN idme_verifications v ON g.verification_id = v.id
    WHERE v.user_id = $1
      AND v.status = 'verified'
    GROUP BY v.user_id
  `, [userId]);

  if (!result.rows.length) {
    return {
      tier: 'None',
      benefits: []
    };
  }

  const groups = result.rows[0].verified_groups || [];
  const count = result.rows[0].group_count;

  // Determine tier based on verifications
  if (count >= 3) {
    return {
      tier: 'Platinum',
      discount: 0.30,
      benefits: [
        'All Premium Features',
        '30% Lifetime Discount',
        'Priority Support',
        'Exclusive Events',
        'Partner Network Access'
      ],
      groups
    };
  } else if (count >= 2) {
    return {
      tier: 'Gold',
      discount: 0.20,
      benefits: [
        'Premium Features',
        '20% Discount',
        'Priority Support',
        'Exclusive Events'
      ],
      groups
    };
  } else if (count === 1) {
    return {
      tier: 'Silver',
      discount: 0.15,
      benefits: [
        'Standard Features',
        '15% Discount',
        'Standard Support'
      ],
      groups
    };
  }

  return {
    tier: 'None',
    benefits: []
  };
}

// Usage in membership page
const membership = await determineMembershipTier(userId);

console.log(`Membership Tier: ${membership.tier}`);
console.log(`Verified Groups: ${membership.groups.join(', ')}`);
console.log(`Discount: ${membership.discount * 100}%`);
console.log('Benefits:', membership.benefits);
```

---

## Troubleshooting

### Common Issues

#### "Invalid redirect_uri"

```
Error: The redirect_uri is not valid for this application
```

**Solution:** Ensure `IDME_REDIRECT_URI` matches exactly what is registered in the ID.me developer dashboard. The URL must be identical, including trailing slashes and protocol.

#### "Invalid client credentials"

```
Error: Client authentication failed
```

**Solution:** Verify `IDME_CLIENT_ID` and `IDME_CLIENT_SECRET` are correct. Ensure you are using the right credentials for the environment (sandbox vs production).

#### "relation idme_verifications does not exist"

```
Error: relation "idme_verifications" does not exist
```

**Solution:** Run the installer to create tables.

```bash
cd plugins/idme
./install.sh
```

Or initialize via CLI:

```bash
nself plugin idme init
```

#### "Database Connection Failed"

```
Error: Connection refused
```

**Solutions:**
1. Verify PostgreSQL is running
2. Check `DATABASE_URL` format
3. Test connection: `psql $DATABASE_URL -c "SELECT 1"`

#### "Invalid webhook signature"

```
Error: Webhook signature verification failed
```

**Solution:** Ensure `IDME_WEBHOOK_SECRET` matches the secret configured in the ID.me webhook settings.

### Sandbox Mode

Enable sandbox mode for testing without real verification:

```bash
IDME_SANDBOX=true
```

This uses the ID.me test environment at `api.idmelabs.com`.

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug nself plugin idme server --port 3010
```

### Health Checks

```bash
# Check server health
curl http://localhost:3010/health

# Test all components
nself plugin idme test
```

---

## Support

- **GitHub Issues:** [nself-plugins/issues](https://github.com/acamarata/nself-plugins/issues)
- **ID.me Developer Portal:** [developers.id.me](https://developers.id.me)

---

*Last Updated: January 2026*
*Plugin Version: 1.0.0*
