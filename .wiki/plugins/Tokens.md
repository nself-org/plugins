# Tokens Plugin

Secure content delivery tokens, HLS encryption key management, and entitlement checks

---

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Token Architecture](#token-architecture)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Tokens plugin provides enterprise-grade token-based access control for content delivery. It implements signed JWT-like tokens, HLS encryption key management, and entitlement-based access control.

### Key Features
- **Signed Tokens**: HMAC-SHA256 signed access tokens with configurable TTL
- **HLS Encryption**: AES-128 encryption key management with automatic rotation
- **Entitlement System**: User-content access control with expiration
- **IP Restrictions**: Optional IP-based token validation
- **Device Binding**: Optional device-specific tokens
- **Key Rotation**: Zero-downtime signing key and encryption key rotation
- **Revocation**: Instant token, user, or content-level revocation
- **Audit Trail**: Complete token issuance and validation history

### Use Cases
- Streaming media DRM and access control
- Signed URL generation for CDN content
- HLS playlist encryption (AES-128)
- Pay-per-view access management
- Geographic content restrictions
- Device limit enforcement

---

## Quick Start

```bash
# Install
nself plugin install tokens

# Configure (minimal .env)
cat > .env <<EOF
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
TOKENS_ENCRYPTION_KEY=$(openssl rand -hex 32)
EOF

# Initialize
nself plugin tokens init

# Start server
nself plugin tokens server

# Issue a token
nself plugin tokens issue \
  --user "user123" \
  --content "video456" \
  --type playback \
  --ttl 3600
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `TOKENS_PLUGIN_PORT` | No | `3021` | HTTP server port |
| `TOKENS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `TOKENS_LOG_LEVEL` | No | `info` | Logging level |
| `TOKENS_APP_IDS` | No | `primary` | Comma-separated app IDs |
| `TOKENS_ENCRYPTION_KEY` | **Yes** | - | Master encryption key (hex, 32+ bytes) |
| `TOKENS_DEFAULT_TTL_SECONDS` | No | `3600` | Default token TTL (1 hour) |
| `TOKENS_MAX_TTL_SECONDS` | No | `86400` | Maximum token TTL (24 hours) |
| `TOKENS_SIGNING_ALGORITHM` | No | `hmac-sha256` | Token signing algorithm |
| `TOKENS_HLS_ENCRYPTION_ENABLED` | No | `false` | Enable HLS encryption key management |
| `TOKENS_HLS_KEY_ROTATION_HOURS` | No | `168` | HLS key rotation interval (7 days) |
| `TOKENS_DEFAULT_ENTITLEMENT_CHECK` | No | `true` | Enforce entitlement checks on token issuance |
| `TOKENS_ALLOW_ALL_IF_NO_ENTITLEMENTS` | No | `true` | Allow access if user has no entitlements defined |
| `TOKENS_EXPIRED_RETENTION_DAYS` | No | `7` | Days to retain expired token records |

### Example .env File
```bash
# Database
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server
TOKENS_PLUGIN_PORT=3021
TOKENS_LOG_LEVEL=info

# Encryption (Generate with: openssl rand -hex 32)
TOKENS_ENCRYPTION_KEY=a1b2c3d4e5f6...

# Token defaults
TOKENS_DEFAULT_TTL_SECONDS=3600
TOKENS_MAX_TTL_SECONDS=86400

# HLS encryption
TOKENS_HLS_ENCRYPTION_ENABLED=true
TOKENS_HLS_KEY_ROTATION_HOURS=168

# Entitlement enforcement
TOKENS_DEFAULT_ENTITLEMENT_CHECK=true
TOKENS_ALLOW_ALL_IF_NO_ENTITLEMENTS=true

# Multi-app
TOKENS_APP_IDS=app1,app2,primary
```

### Generating Encryption Key
```bash
# Generate secure 32-byte key
openssl rand -hex 32
```

---

## CLI Commands

### Initialize
```bash
nself plugin tokens init
```

### Start Server
```bash
nself plugin tokens server
```

### Issue Token
```bash
# Basic playback token
nself plugin tokens issue \
  --user "user123" \
  --content "video456" \
  --type playback \
  --ttl 3600

# Download token with device binding
nself plugin tokens issue \
  --user "user123" \
  --content "video456" \
  --type download \
  --device "device789" \
  --ttl 7200

# Preview token (short-lived)
nself plugin tokens issue \
  --user "user123" \
  --content "video456" \
  --type preview \
  --ttl 300
```

### Validate Token
```bash
nself plugin tokens validate \
  --token "eyJhbGc..."
```

### Revoke Tokens
```bash
# Revoke specific token
nself plugin tokens revoke \
  --token-id "uuid-here" \
  --reason "User requested"

# Revoke all tokens for user
nself plugin tokens revoke \
  --user "user123" \
  --reason "Account suspended"

# Revoke all tokens for content
nself plugin tokens revoke \
  --content "video456" \
  --reason "Content removed"
```

### Manage Signing Keys
```bash
# List signing keys
nself plugin tokens keys list

# Rotate a key
nself plugin tokens keys rotate \
  --name "primary" \
  --expire-hours 24
```

### Manage Entitlements
```bash
# List user entitlements
nself plugin tokens entitlements \
  --user "user123"

# Filter by content type
nself plugin tokens entitlements \
  --user "user123" \
  --content-type "video"
```

### Statistics
```bash
nself plugin tokens stats
```

---

## REST API

### Base URL
```
http://localhost:3021
```

### Token Issuance

#### POST /api/issue
Issue a signed access token
```http
POST /api/issue
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "userId": "user123",
  "contentId": "video456",
  "tokenType": "playback",
  "ttlSeconds": 3600,
  "deviceId": "device789",
  "permissions": {"quality": "4K", "offline": false},
  "ipRestriction": "192.168.1.100",
  "contentType": "video"
}
```

Response:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyMTIzIiwiY2lkIjoidmlkZW80NTYiLCJ0eXAiOiJwbGF5YmFjayIsImV4cCI6MTcwNjEyMzQ1NiwiaWF0IjoxNzA2MTE5ODU2LCJwZXJtIjp7InF1YWxpdHkiOiI0SyJ9fQ.signature",
  "expiresAt": "2025-01-24T13:00:00.000Z",
  "tokenId": "550e8400-e29b-41d4-a716-446655440000"
}
```

#### POST /api/validate
Validate a token
```http
POST /api/validate
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "token": "eyJhbGc...",
  "contentId": "video456",
  "ipAddress": "192.168.1.100"
}
```

Response (valid):
```json
{
  "valid": true,
  "userId": "user123",
  "contentId": "video456",
  "permissions": {"quality": "4K"},
  "expiresAt": "2025-01-24T13:00:00.000Z"
}
```

Response (invalid):
```json
{
  "valid": false
}
```

#### POST /api/revoke
Revoke a token
```http
POST /api/revoke
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "tokenId": "550e8400-e29b-41d4-a716-446655440000",
  "reason": "User requested"
}
```

#### POST /api/revoke/user
Revoke all user tokens
```http
POST /api/revoke/user
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "userId": "user123",
  "reason": "Account suspended"
}
```

#### POST /api/revoke/content
Revoke all content tokens
```http
POST /api/revoke/content
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "contentId": "video456",
  "reason": "Content removed"
}
```

### Signing Keys

#### POST /api/keys
Create signing key
```http
POST /api/keys
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "name": "primary",
  "algorithm": "hmac-sha256"
}
```

#### GET /api/keys
List signing keys
```http
GET /api/keys
Headers:
  X-App-Name: primary
```

#### POST /api/keys/:id/rotate
Rotate signing key
```http
POST /api/keys/550e8400-e29b-41d4-a716-446655440000/rotate
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "expireOldAfterHours": 24
}
```

#### DELETE /api/keys/:id
Deactivate signing key
```http
DELETE /api/keys/550e8400-e29b-41d4-a716-446655440000
Headers:
  X-App-Name: primary
```

### Encryption Keys (HLS)

#### POST /api/encryption/keys
Create HLS encryption key
```http
POST /api/encryption/keys
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "contentId": "video456"
}
```

Response:
```json
{
  "keyId": "661f9510-f39c-52e5-b827-557766551111",
  "keyUri": "http://localhost:3021/api/encryption/keys/661f9510-f39c-52e5-b827-557766551111/deliver"
}
```

#### GET /api/encryption/keys/:id/deliver
Deliver encryption key (binary)
```http
GET /api/encryption/keys/661f9510-f39c-52e5-b827-557766551111/deliver
Headers:
  X-App-Name: primary
```

Returns: 16-byte AES key (application/octet-stream)

#### POST /api/encryption/keys/:contentId/rotate
Rotate encryption key
```http
POST /api/encryption/keys/video456/rotate
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "expireOldAfterHours": 24
}
```

### Entitlements

#### POST /api/entitlements/check
Check entitlement
```http
POST /api/entitlements/check
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "userId": "user123",
  "contentId": "video456",
  "entitlementType": "stream"
}
```

Response (allowed):
```json
{
  "allowed": true,
  "reason": "entitlement_active",
  "restrictions": {"maxResolution": "4K"},
  "expiresAt": "2025-12-31T23:59:59.000Z"
}
```

#### POST /api/entitlements
Grant entitlement
```http
POST /api/entitlements
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "userId": "user123",
  "contentId": "video456",
  "contentType": "video",
  "entitlementType": "stream",
  "expiresAt": "2025-12-31T23:59:59.000Z",
  "metadata": {"tier": "premium"}
}
```

#### DELETE /api/entitlements
Revoke entitlement
```http
DELETE /api/entitlements
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "userId": "user123",
  "contentId": "video456",
  "entitlementType": "stream"
}
```

#### GET /api/entitlements/:userId
List user entitlements
```http
GET /api/entitlements/user123?contentType=video&active=true
Headers:
  X-App-Name: primary
```

---

## Database Schema

### tokens_signing_keys
```sql
CREATE TABLE tokens_signing_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  algorithm VARCHAR(20) NOT NULL DEFAULT 'hmac-sha256',
  key_material_encrypted TEXT NOT NULL,
  is_active BOOLEAN DEFAULT true,
  rotated_from UUID REFERENCES tokens_signing_keys(id),
  created_at TIMESTAMPTZ DEFAULT NOW(),
  rotated_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  UNIQUE(source_account_id, name)
);
```

### tokens_issued
```sql
CREATE TABLE tokens_issued (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  token_hash VARCHAR(128) NOT NULL,
  token_type VARCHAR(50) NOT NULL DEFAULT 'playback',
  signing_key_id UUID REFERENCES tokens_signing_keys(id),
  user_id VARCHAR(255) NOT NULL,
  device_id VARCHAR(255),
  content_id VARCHAR(255) NOT NULL,
  content_type VARCHAR(50),
  permissions JSONB DEFAULT '{}',
  ip_address VARCHAR(45),
  issued_at TIMESTAMPTZ DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL,
  revoked BOOLEAN DEFAULT false,
  revoked_at TIMESTAMPTZ,
  revoked_reason VARCHAR(255),
  last_used_at TIMESTAMPTZ,
  use_count INTEGER DEFAULT 0
);
```

### tokens_encryption_keys
```sql
CREATE TABLE tokens_encryption_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  content_id VARCHAR(255) NOT NULL,
  key_material_encrypted TEXT NOT NULL,
  key_iv VARCHAR(64) NOT NULL,
  key_uri TEXT NOT NULL,
  rotation_generation INTEGER DEFAULT 1,
  is_active BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  rotated_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ
);
```

### tokens_entitlements
```sql
CREATE TABLE tokens_entitlements (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_type VARCHAR(50),
  entitlement_type VARCHAR(50) NOT NULL DEFAULT 'stream',
  granted_by VARCHAR(50) DEFAULT 'system',
  granted_at TIMESTAMPTZ DEFAULT NOW(),
  expires_at TIMESTAMPTZ,
  revoked BOOLEAN DEFAULT false,
  metadata JSONB DEFAULT '{}',
  UNIQUE(source_account_id, user_id, content_id, entitlement_type)
);
```

### tokens_webhook_events
```sql
CREATE TABLE tokens_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMPTZ,
  error TEXT,
  retry_count INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Token Architecture

### Token Format
Tokens use JWT-like format: `{header}.{payload}.{signature}`

**Header:**
```json
{"alg": "HS256", "typ": "JWT"}
```

**Payload:**
```json
{
  "sub": "user123",        // User ID
  "cid": "video456",       // Content ID
  "typ": "playback",       // Token type
  "exp": 1706123456,       // Expiration (Unix timestamp)
  "iat": 1706119856,       // Issued at
  "perm": {"quality": "4K"},  // Permissions
  "did": "device789",      // Device ID (optional)
  "ip": "192.168.1.100",   // IP restriction (optional)
  "ctype": "video"         // Content type (optional)
}
```

**Signature:** HMAC-SHA256(header + payload, signing_key)

### Security Model
- Signing keys stored encrypted with master `TOKENS_ENCRYPTION_KEY`
- Token hashes (not full tokens) stored in database
- HLS encryption keys AES-256-CBC encrypted at rest
- Zero-downtime key rotation with grace periods
- IP and device binding for additional security

---

## Examples

### Example 1: Issue and Validate Token
```bash
# Issue token
curl -X POST http://localhost:3021/api/issue \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "userId": "user123",
    "contentId": "video456",
    "tokenType": "playback",
    "ttlSeconds": 3600
  }'

# Response
{
  "token": "eyJhbGc...",
  "expiresAt": "2025-01-24T13:00:00.000Z",
  "tokenId": "550e8400-..."
}

# Validate token
curl -X POST http://localhost:3021/api/validate \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "token": "eyJhbGc...",
    "contentId": "video456"
  }'

# Response
{
  "valid": true,
  "userId": "user123",
  "contentId": "video456",
  "permissions": {},
  "expiresAt": "2025-01-24T13:00:00.000Z"
}
```

### Example 2: HLS Encryption
```bash
# 1. Create encryption key for content
curl -X POST http://localhost:3021/api/encryption/keys \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{"contentId": "video456"}'

# Response
{
  "keyId": "661f9510-...",
  "keyUri": "http://localhost:3021/api/encryption/keys/661f9510-.../deliver"
}

# 2. Use key URI in HLS manifest (m3u8)
cat video.m3u8
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-KEY:METHOD=AES-128,URI="http://localhost:3021/api/encryption/keys/661f9510-.../deliver"
#EXTINF:10.0,
segment1.ts
#EXTINF:10.0,
segment2.ts

# 3. Player fetches key automatically from keyUri
# No bearer token needed - key URI is time-limited and signed
```

### Example 3: Entitlement-Based Access
```sql
-- Grant user access to premium content
INSERT INTO tokens_entitlements (
  source_account_id,
  user_id,
  content_id,
  entitlement_type,
  expires_at,
  metadata
) VALUES (
  'primary',
  'user123',
  'video456',
  'stream',
  NOW() + INTERVAL '30 days',
  '{"tier": "premium", "maxDevices": 3}'::jsonb
);

-- Query user's active entitlements
SELECT
  content_id,
  entitlement_type,
  expires_at,
  metadata
FROM tokens_entitlements
WHERE source_account_id = 'primary'
  AND user_id = 'user123'
  AND revoked = false
  AND (expires_at IS NULL OR expires_at > NOW());
```

---

## Troubleshooting

### Issue: "No active signing key configured"
**Cause:** No signing keys exist.

**Solution:**
```bash
# Create initial signing key via API
curl -X POST http://localhost:3021/api/keys \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{"name": "primary", "algorithm": "hmac-sha256"}'
```

### Issue: Token validation fails
**Causes:**
- Token expired
- Token revoked
- IP mismatch (if IP restriction enabled)
- Content ID mismatch
- Signing key rotated without grace period

**Debug:**
```bash
# Check token record
SELECT * FROM tokens_issued WHERE token_hash = 'hash_here';

# Check if signing key is active
SELECT * FROM tokens_signing_keys WHERE id = 'key_id_here';
```

### Issue: Entitlement check denies access
**Verify entitlement:**
```bash
curl -X POST http://localhost:3021/api/entitlements/check \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "userId": "user123",
    "contentId": "video456",
    "entitlementType": "stream"
  }'
```

**Check allow-all mode:**
```bash
# If user has NO entitlements and TOKENS_ALLOW_ALL_IF_NO_ENTITLEMENTS=true,
# access is allowed by default
echo $TOKENS_ALLOW_ALL_IF_NO_ENTITLEMENTS
```

---

**Plugin Version:** 1.0.0
**Last Updated:** February 11, 2026
