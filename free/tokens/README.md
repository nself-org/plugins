# tokens

Secure content delivery tokens, HLS encryption key management, and entitlement checks. Issues HMAC-SHA256 signed JWT-style tokens that gate access to content, manages per-content AES-128 encryption keys for HLS delivery, and tracks user entitlements with optional IP and device restrictions. Designed for nself-tv and any platform that needs signed URL or DRM-lite access control.

## Installation

```bash
nself plugin install tokens
```

## Features

- Issue HMAC-SHA256 signed access tokens with configurable TTL (default 1 hour, max 24 hours)
- Token validation with expiry, revocation, content ID, and IP address checks
- Individual token revocation, bulk revocation by user, and bulk revocation by content
- Signing key management — create, list, rotate, and deactivate keys
- Key rotation with configurable grace period for old key expiry (default 24 hours)
- AES-128 HLS encryption key generation and delivery for per-content key management
- Encryption key rotation with generation tracking and grace-period expiry
- Entitlement management — grant, revoke, and check user access to specific content
- Entitlement types (e.g., `stream`, `download`, `playback`) with optional expiry dates
- `allowAllIfNoEntitlements` mode for open-access deployments
- Webhook event log for all token and entitlement lifecycle events
- Per-request usage tracking (`last_used_at`, `use_count`)
- Multi-app isolation via `source_account_id`
- API key authentication and rate limiting

## Configuration

| Name | Required | Default | Description |
| ---- | -------- | ------- | ----------- |
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `TOKENS_ENCRYPTION_KEY` | Yes | — | Secret used to encrypt stored signing and encryption key material (AES-256-CBC) |
| `TOKENS_PLUGIN_PORT` | No | `3107` | HTTP server port |
| `TOKENS_APP_IDS` | No | `primary` | Comma-separated list of app IDs (source account IDs) this instance serves |
| `TOKENS_DEFAULT_TTL_SECONDS` | No | `3600` | Default token lifetime in seconds (1 hour) |
| `TOKENS_MAX_TTL_SECONDS` | No | `86400` | Maximum token lifetime in seconds (24 hours) |
| `TOKENS_HLS_ENCRYPTION_ENABLED` | No | `false` | Enable HLS encryption key endpoints |
| `TOKENS_DEFAULT_ENTITLEMENT_CHECK` | No | `true` | Enforce entitlement check on token issuance |
| `TOKENS_ALLOW_ALL_IF_NO_ENTITLEMENTS` | No | `true` | Allow token issuance when a user has no entitlements at all |
| `TOKENS_EXPIRED_RETENTION_DAYS` | No | `7` | Days to retain expired token records before cleanup |
| `TOKENS_API_KEY` | No | — | API key required on all requests (if set) |
| `TOKENS_RATE_LIMIT_MAX` | No | `100` | Inbound API rate limit — requests per window |
| `TOKENS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

`TOKENS_ENCRYPTION_KEY` protects signing key material and HLS key material at rest. Use a random 32-byte hex string. Never reuse across environments.

## API Reference

### Health

#### GET /health

Returns `{ status: "ok", plugin: "tokens", version, timestamp }`. No authentication required.

#### GET /ready

Returns `{ ready: true, database: "ok" }`, or `{ ready: false, database: "error" }` when the database is unreachable.

#### GET /live

Returns uptime, memory usage, and a full stats snapshot.

### Token Issuance

#### POST /api/issue

Issues a signed access token. If `TOKENS_DEFAULT_ENTITLEMENT_CHECK` is enabled, the user must have a valid entitlement for the requested content. Returns `403` if the entitlement check fails.

Request body:

```json
{
  "userId": "user-123",
  "contentId": "movie-456",
  "tokenType": "playback",
  "ttlSeconds": 3600,
  "permissions": { "quality": "4k", "download": false },
  "deviceId": "device-abc",
  "ipRestriction": "203.0.113.5",
  "contentType": "movie"
}
```

`tokenType` and `contentType` are stored as metadata. `permissions` is a free-form JSON object embedded in the token payload. `ipRestriction` causes validation to fail for requests from other IPs. `ttlSeconds` is capped at `TOKENS_MAX_TTL_SECONDS`.

Response:

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expiresAt": "2026-02-21T11:00:00Z",
  "tokenId": "uuid"
}
```

The token is a base64url-encoded JWT-style string signed with the active signing key using HMAC-SHA256. Only the hash of the token is stored — the raw token is returned once and cannot be retrieved again.

#### POST /api/validate

Validates a token and returns the associated claims. Checks: token exists in DB, not revoked, not expired, content ID matches (if provided), IP address matches (if restricted).

Request body:

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "contentId": "movie-456",
  "ipAddress": "203.0.113.5"
}
```

`contentId` and `ipAddress` are optional additional validation constraints.

Response when valid:

```json
{
  "valid": true,
  "userId": "user-123",
  "contentId": "movie-456",
  "permissions": { "quality": "4k", "download": false },
  "expiresAt": "2026-02-21T11:00:00Z"
}
```

Response when invalid: `{ "valid": false }`

Each successful validation updates `last_used_at` and increments `use_count` on the token record.

#### POST /api/revoke

Revokes a single token by its `tokenId` (from the issue response).

Request body:

```json
{
  "tokenId": "uuid",
  "reason": "user_logout"
}
```

#### POST /api/revoke/user

Revokes all active tokens for a user.

Request body:

```json
{
  "userId": "user-123",
  "reason": "account_suspended"
}
```

Response: `{ "revoked": 5, "userId": "user-123" }`

#### POST /api/revoke/content

Revokes all active tokens for a content item.

Request body:

```json
{
  "contentId": "movie-456",
  "reason": "content_removed"
}
```

Response: `{ "revoked": 12, "contentId": "movie-456" }`

### Signing Keys

Tokens are signed using the most recently created active signing key. Creating a new key does not automatically retire the old one — use the rotate endpoint for zero-downtime key rotation.

#### POST /api/keys

Creates a new signing key. A 32-byte random key is generated and stored encrypted.

Request body:

```json
{
  "name": "primary-key",
  "algorithm": "hmac-sha256"
}
```

`algorithm` defaults to `hmac-sha256`.

Response:

```json
{
  "id": "uuid",
  "name": "primary-key",
  "algorithm": "hmac-sha256",
  "isActive": true,
  "createdAt": "2026-02-21T00:00:00Z"
}
```

The raw key material is never returned. It is stored AES-256-CBC encrypted using `TOKENS_ENCRYPTION_KEY`.

#### GET /api/keys

Lists all signing keys. Returns metadata only — no key material.

#### POST /api/keys/:id/rotate

Rotates a signing key. Creates a new key with the same name and algorithm, and schedules the old key for expiry after `expireOldAfterHours` hours.

Request body:

```json
{
  "expireOldAfterHours": 24
}
```

The new key becomes active immediately. Tokens issued with the old key remain valid until they expire or are revoked — the old key is not deactivated instantly, only scheduled to expire.

#### DELETE /api/keys/:id

Deactivates a signing key immediately. Active tokens signed by this key can no longer be validated.

### Encryption Keys (HLS)

Encryption keys are per-content AES-128 keys used for HLS playlist encryption (`#EXT-X-KEY`). Each key has a delivery endpoint that returns the raw key bytes.

#### POST /api/encryption/keys

Creates a new AES-128 encryption key for a content ID.

Request body:

```json
{
  "contentId": "movie-456"
}
```

Response:

```json
{
  "keyId": "uuid",
  "keyUri": "http://localhost:3107/api/encryption/keys/uuid/deliver"
}
```

The `keyUri` is the URL to embed in the HLS playlist (`#EXT-X-KEY:METHOD=AES-128,URI="..."`). Key material is stored encrypted and returned as raw bytes on delivery.

#### GET /api/encryption/keys/:id/deliver

Delivers raw AES-128 key bytes (`application/octet-stream`, 16 bytes). This URL is called by HLS players when loading an encrypted segment. Returns `404` if the key is inactive.

#### POST /api/encryption/keys/:contentId/rotate

Rotates the encryption key for a content item. Creates a new key and schedules the old key to expire after `expireOldAfterHours` hours. Returns the new key ID, delivery URI, and generation number.

Request body:

```json
{
  "expireOldAfterHours": 24
}
```

Response:

```json
{
  "keyId": "uuid",
  "keyUri": "http://localhost:3107/api/encryption/keys/uuid/deliver",
  "generation": 2
}
```

### Entitlements

Entitlements express that a user has the right to access a specific content item with a specific entitlement type. Token issuance checks entitlements when `TOKENS_DEFAULT_ENTITLEMENT_CHECK` is enabled.

#### POST /api/entitlements/check

Checks whether a user has a valid entitlement.

Request body:

```json
{
  "userId": "user-123",
  "contentId": "movie-456",
  "entitlementType": "stream"
}
```

Response when allowed:

```json
{
  "allowed": true,
  "reason": "entitlement_active",
  "expiresAt": "2027-01-01T00:00:00Z"
}
```

Response when denied:

```json
{
  "allowed": false,
  "reason": "no_valid_entitlement"
}
```

When `allowAllIfNoEntitlements` is `true` and the user has no entitlements at all, the response is `{ "allowed": true, "reason": "no_entitlements_mode" }`.

#### POST /api/entitlements

Grants an entitlement to a user. If an entitlement with the same `(userId, contentId, entitlementType)` already exists, it is updated (upsert).

Request body:

```json
{
  "userId": "user-123",
  "contentId": "movie-456",
  "contentType": "movie",
  "entitlementType": "stream",
  "expiresAt": "2027-01-01T00:00:00Z",
  "metadata": { "plan": "premium" }
}
```

#### DELETE /api/entitlements

Revokes an entitlement. The record is soft-deleted (`revoked: true`).

Request body:

```json
{
  "userId": "user-123",
  "contentId": "movie-456",
  "entitlementType": "stream"
}
```

#### GET /api/entitlements/:userId

Lists entitlements for a user. Optionally filter by content type and active state.

Query parameters:

| Parameter | Description |
| --------- | ----------- |
| `contentType` | Filter by content type |
| `active` | `false` to include expired and revoked entitlements (default `true`) |

## Database Tables

| Table | Purpose |
| ----- | ------- |
| `np_tokens_signing_keys` | HMAC signing keys — name, algorithm, encrypted key material, rotation lineage, expiry |
| `np_tokens_issued` | Issued token records — token hash, user, content, device, IP restriction, expiry, revocation, usage stats |
| `np_tokens_encryption_keys` | AES-128 HLS encryption keys — content ID, encrypted key material, IV, delivery URI, rotation generation |
| `np_tokens_entitlements` | User-content entitlements — type, expiry, revocation, metadata |
| `np_tokens_webhook_events` | Lifecycle events for tokens, keys, and entitlements — idempotent inserts, processed flag |

All tables include `source_account_id` for multi-app isolation.

## Usage Examples

### Issue a token for a user who just purchased access

```typescript
// Grant the entitlement first
await fetch('http://localhost:3107/api/entitlements', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    userId: 'user-123',
    contentId: 'movie-456',
    entitlementType: 'stream',
    expiresAt: '2027-01-01T00:00:00Z',
  }),
});

// Issue a playback token
const res = await fetch('http://localhost:3107/api/issue', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    userId: 'user-123',
    contentId: 'movie-456',
    tokenType: 'playback',
    ttlSeconds: 7200,
  }),
});
const { token, expiresAt } = await res.json();
```

### Validate a token in a media server middleware

```typescript
const res = await fetch('http://localhost:3107/api/validate', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    token: request.headers['x-playback-token'],
    contentId: params.contentId,
  }),
});
const { valid, userId, permissions } = await res.json();
if (!valid) return reply.status(403).send({ error: 'Access denied' });
```

### Create a signing key and start issuing tokens

```typescript
// Create the key (only needed once per deployment)
await fetch('http://localhost:3107/api/keys', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ name: 'primary', algorithm: 'hmac-sha256' }),
});

// Tokens can now be issued — the plugin uses the active key automatically
```

### Set up HLS encryption for a content item

```typescript
// Create encryption key
const res = await fetch('http://localhost:3107/api/encryption/keys', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ contentId: 'movie-456' }),
});
const { keyUri } = await res.json();

// keyUri goes into the HLS playlist:
// #EXT-X-KEY:METHOD=AES-128,URI="<keyUri>",IV=0x...
```

### Revoke all tokens when a user is suspended

```typescript
await fetch('http://localhost:3107/api/revoke/user', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ userId: 'user-123', reason: 'account_suspended' }),
});
```

## Integration

This plugin is the access control layer for **nself-tv**. It integrates with the TV server to gate playback: before streaming begins, the server issues a token; the player attaches the token to segment requests; the CDN or edge middleware validates the token before serving bytes.

For HLS-encrypted content, the delivery endpoint (`/api/encryption/keys/:id/deliver`) is embedded in the playlist as `#EXT-X-KEY:URI`. HLS players call it automatically when loading a new key period. The plugin verifies the caller's token before returning key bytes.

The entitlement system integrates with billing — when a user purchases access, the billing webhook grants an entitlement, which then allows the token issuance endpoint to succeed.

## Changelog

### v1.0.0

- Initial release
- HMAC-SHA256 signed token issuance and validation
- Token revocation by ID, user, and content
- Signing key creation, listing, rotation, and deactivation
- AES-128 HLS encryption key creation, delivery, and rotation
- Entitlement management with grant, revoke, check, and list
- `allowAllIfNoEntitlements` open-access mode
- Webhook event log for all lifecycle events
- IP and device restriction support on tokens
- Multi-app isolation via `source_account_id`
- API key authentication and rate limiting
