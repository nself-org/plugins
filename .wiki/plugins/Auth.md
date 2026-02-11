# Auth Plugin

**Category:** Authentication
**Port:** 3014
**Version:** 1.0.0

Advanced authentication methods service that extends nSelf's core JWT auth. Provides OAuth provider integration (Google, Apple, Facebook, GitHub, Microsoft), WebAuthn/passkey registration and verification, TOTP 2FA enrollment and verification, magic link email flow, device-code flow (TV login pattern), and session management.

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Configuration](#configuration)
4. [CLI Commands](#cli-commands)
5. [REST API](#rest-api)
6. [Database Schema](#database-schema)
7. [Webhook Events](#webhook-events)
8. [Multi-App Support](#multi-app-support)
9. [Security](#security)
10. [Troubleshooting](#troubleshooting)

---

## Overview

### What It Does

The auth plugin provides advanced authentication methods:

- **OAuth Integration**: Google, Apple, Facebook, GitHub, Microsoft
- **WebAuthn/Passkeys**: FIDO2-based passwordless authentication
- **TOTP 2FA**: Time-based one-time password two-factor authentication
- **Magic Links**: Email-based passwordless login
- **Device Code Flow**: TV/device login (scan QR or enter code on phone)
- **Session Management**: Cross-device session tracking and remote logout
- **Login Attempt Tracking**: Rate limiting and security monitoring

### What It Does NOT Do

- Does NOT replace nSelf core auth (JWT tokens, basic user table)
- Does NOT handle government identity verification (idme plugin does that)
- Does NOT send notification emails directly (delegates to notifications plugin)

### Use Cases

- **Multi-platform apps**: OAuth login for web/mobile apps
- **TV apps**: Device code flow for Android TV, Fire TV, Roku, Apple TV
- **High-security apps**: TOTP 2FA for admin accounts
- **Passwordless auth**: WebAuthn passkeys or magic links
- **Session security**: Remote logout, device tracking

---

## Quick Start

### 1. Install Dependencies

```bash
cd plugins/auth/ts
npm install
```

### 2. Set Required Environment Variables

```bash
# Required
export AUTH_ENCRYPTION_KEY="your-32-character-encryption-key-here"
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"

# Optional OAuth providers
export AUTH_GOOGLE_CLIENT_ID="your-google-client-id"
export AUTH_GOOGLE_CLIENT_SECRET="your-google-client-secret"
```

### 3. Initialize Database

```bash
npm run build
node dist/cli.js init
```

### 4. Start Server

```bash
# Development
npm run dev

# Production
npm start
```

The server will start on `http://localhost:3014`.

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `AUTH_ENCRYPTION_KEY` | Encryption key for tokens and secrets (minimum 32 characters) |
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

#### Server Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_PLUGIN_PORT` | `3014` | HTTP server port |
| `AUTH_PLUGIN_HOST` | `0.0.0.0` | HTTP server host |
| `AUTH_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `AUTH_APP_IDS` | `primary` | Comma-separated app IDs for multi-app mode |

#### OAuth - Google

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_GOOGLE_CLIENT_ID` | - | Google OAuth client ID |
| `AUTH_GOOGLE_CLIENT_SECRET` | - | Google OAuth client secret |
| `AUTH_GOOGLE_SCOPES` | `email,profile` | OAuth scopes |

#### OAuth - Apple

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_APPLE_CLIENT_ID` | - | Apple client ID (Services ID) |
| `AUTH_APPLE_TEAM_ID` | - | Apple Team ID |
| `AUTH_APPLE_KEY_ID` | - | Apple Key ID |
| `AUTH_APPLE_PRIVATE_KEY` | - | Apple private key (P8 file contents) |

#### OAuth - Facebook

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_FACEBOOK_APP_ID` | - | Facebook App ID |
| `AUTH_FACEBOOK_APP_SECRET` | - | Facebook App Secret |

#### OAuth - GitHub

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_GITHUB_CLIENT_ID` | - | GitHub OAuth client ID |
| `AUTH_GITHUB_CLIENT_SECRET` | - | GitHub OAuth client secret |

#### OAuth - Microsoft

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_MICROSOFT_CLIENT_ID` | - | Microsoft client ID |
| `AUTH_MICROSOFT_CLIENT_SECRET` | - | Microsoft client secret |

#### WebAuthn/Passkeys

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_WEBAUTHN_RP_NAME` | `nSelf` | Relying Party name |
| `AUTH_WEBAUTHN_RP_ID` | `localhost` | Relying Party ID (domain) |
| `AUTH_WEBAUTHN_ORIGIN` | `http://localhost:3014` | WebAuthn origin URL |

#### TOTP 2FA

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_TOTP_ISSUER` | `nSelf` | TOTP issuer name |
| `AUTH_TOTP_ALGORITHM` | `SHA1` | TOTP algorithm |
| `AUTH_TOTP_DIGITS` | `6` | Number of digits |
| `AUTH_TOTP_PERIOD` | `30` | Time period (seconds) |
| `AUTH_TOTP_BACKUP_CODE_COUNT` | `10` | Number of backup codes |

#### Magic Links

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_MAGIC_LINK_EXPIRY_SECONDS` | `600` | Link expiry time (10 minutes) |
| `AUTH_MAGIC_LINK_BASE_URL` | - | Base URL for magic links |

#### Device Code Flow

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_DEVICE_CODE_EXPIRY_SECONDS` | `600` | Code expiry time (10 minutes) |
| `AUTH_DEVICE_CODE_POLL_INTERVAL` | `5` | Polling interval (seconds) |
| `AUTH_DEVICE_CODE_LENGTH` | `8` | User code length |

#### Sessions

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_SESSION_MAX_PER_USER` | `10` | Maximum concurrent sessions per user |
| `AUTH_SESSION_IDLE_TIMEOUT_HOURS` | `24` | Idle timeout (hours) |
| `AUTH_SESSION_ABSOLUTE_TIMEOUT_HOURS` | `720` | Absolute timeout (30 days) |

#### Security

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_LOGIN_MAX_ATTEMPTS` | `5` | Max failed login attempts before lockout |
| `AUTH_LOGIN_LOCKOUT_MINUTES` | `15` | Lockout duration (minutes) |

#### Cleanup

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_CLEANUP_CRON` | `0 */6 * * *` | Cron schedule for cleanup (every 6 hours) |

---

## CLI Commands

### Initialize Database

```bash
nself plugin auth init
```

Initializes the auth database schema (7 tables).

### Start Server

```bash
nself plugin auth server
```

Starts the auth HTTP server on the configured port.

### List Sessions

```bash
nself plugin auth sessions --user user-123
```

Lists all active sessions for a user.

### Revoke Session

```bash
nself plugin auth revoke-session --session-id <uuid> --reason "Security"
```

Revokes a specific session (remote logout).

### Revoke All Sessions

```bash
nself plugin auth revoke-all --user user-123 --except <session-id> --reason "Password reset"
```

Revokes all sessions for a user, optionally excluding the current session.

### Check MFA Status

```bash
nself plugin auth mfa-status --user user-123
```

Checks TOTP 2FA enrollment status for a user.

### View Login Attempts

```bash
nself plugin auth login-attempts --user user-123 --limit 20
```

Shows recent login attempts for a user.

### List OAuth Connections

```bash
nself plugin auth oauth-connections --user user-123
```

Lists all OAuth provider connections for a user.

### Cleanup Expired Data

```bash
nself plugin auth cleanup-expired
```

Cleans up expired device codes, magic links, sessions, and old login attempts.

### Show Statistics

```bash
nself plugin auth stats
```

Displays plugin statistics (OAuth providers, passkeys, MFA enrollments, active sessions, etc.).

---

## REST API

### Health & Status

#### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "auth",
  "timestamp": "2026-02-10T12:00:00Z",
  "version": "1.0.0"
}
```

#### GET /ready

Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "database": "ok",
  "timestamp": "2026-02-10T12:00:00Z"
}
```

#### GET /live

Liveness check with detailed stats.

**Response:**
```json
{
  "alive": true,
  "uptime": 3600,
  "memory": {
    "used": 50000000,
    "total": 100000000
  },
  "stats": {
    "oauthProviders": 150,
    "passkeys": 45,
    "mfaEnrollments": 30,
    "activeSessions": 200,
    "activeDeviceCodes": 5,
    "pendingMagicLinks": 10,
    "recentLoginAttempts": 50
  }
}
```

### OAuth Endpoints

#### GET /api/oauth/providers

List configured OAuth providers for current app.

**Response:**
```json
{
  "providers": [
    { "name": "google", "displayName": "Google", "enabled": true },
    { "name": "apple", "displayName": "Apple", "enabled": true }
  ]
}
```

#### GET /api/oauth/:provider/start

Initiate OAuth flow.

**Query Parameters:**
- `redirectUri` (required): OAuth callback URL
- `state` (optional): CSRF state
- `scopes` (optional): Comma-separated scopes

**Response:**
```json
{
  "authorizationUrl": "https://accounts.google.com/o/oauth2/v2/auth?...",
  "state": "random-state-value"
}
```

#### GET /api/oauth/:provider/callback

OAuth callback handler.

**Query Parameters:**
- `code` (required): Authorization code
- `state` (required): CSRF state

**Response:**
```json
{
  "userId": "user-123",
  "provider": "google",
  "providerEmail": "user@example.com",
  "providerName": "John Doe",
  "providerAvatarUrl": "https://...",
  "accessToken": "...",
  "refreshToken": "..."
}
```

#### POST /api/oauth/:provider/link

Link OAuth account to existing user.

**Request Body:**
```json
{
  "userId": "user-123",
  "code": "authorization-code",
  "redirectUri": "https://app.com/callback"
}
```

**Response:**
```json
{
  "linked": true,
  "provider": "google",
  "providerEmail": "user@example.com"
}
```

#### DELETE /api/oauth/:provider/unlink

Unlink OAuth account.

**Request Body:**
```json
{
  "userId": "user-123"
}
```

**Response:**
```json
{
  "success": true
}
```

#### GET /api/oauth/connections/:userId

List user's OAuth connections.

**Response:**
```json
{
  "connections": [
    {
      "provider": "google",
      "providerEmail": "user@example.com",
      "providerName": "John Doe",
      "linkedAt": "2026-01-15T10:00:00Z",
      "lastUsedAt": "2026-02-10T08:30:00Z"
    }
  ]
}
```

### WebAuthn/Passkeys Endpoints

#### POST /api/passkeys/register/start

Start passkey registration.

**Request Body:**
```json
{
  "userId": "user-123",
  "userName": "john@example.com",
  "userDisplayName": "John Doe"
}
```

**Response:**
```json
{
  "options": {
    "challenge": "...",
    "rp": { "name": "nSelf", "id": "example.com" },
    "user": { "id": "...", "name": "john@example.com", "displayName": "John Doe" },
    "pubKeyCredParams": [...],
    "authenticatorSelection": {...},
    "timeout": 60000
  }
}
```

#### POST /api/passkeys/register/finish

Complete passkey registration.

**Request Body:**
```json
{
  "userId": "user-123",
  "credential": { /* WebAuthn attestation response */ },
  "friendlyName": "MacBook Pro TouchID"
}
```

**Response:**
```json
{
  "credentialId": "...",
  "deviceType": "platform"
}
```

#### POST /api/passkeys/authenticate/start

Start passkey authentication.

**Request Body:**
```json
{
  "userId": "user-123"
}
```

**Response:**
```json
{
  "options": {
    "challenge": "...",
    "rpId": "example.com",
    "allowCredentials": [...],
    "timeout": 60000
  }
}
```

#### POST /api/passkeys/authenticate/finish

Complete passkey authentication.

**Request Body:**
```json
{
  "credential": { /* WebAuthn assertion response */ }
}
```

**Response:**
```json
{
  "userId": "user-123",
  "accessToken": "...",
  "refreshToken": "..."
}
```

#### GET /api/passkeys/:userId

List registered passkeys.

**Response:**
```json
{
  "passkeys": [
    {
      "id": "uuid",
      "credentialId": "...",
      "deviceType": "platform",
      "friendlyName": "MacBook Pro TouchID",
      "lastUsedAt": "2026-02-10T08:00:00Z",
      "createdAt": "2026-01-15T10:00:00Z"
    }
  ]
}
```

#### DELETE /api/passkeys/:credentialId

Remove a passkey.

**Response:**
```json
{
  "success": true
}
```

### TOTP 2FA Endpoints

#### POST /api/mfa/totp/enroll

Start TOTP enrollment.

**Request Body:**
```json
{
  "userId": "user-123"
}
```

**Response:**
```json
{
  "secret": "JBSWY3DPEHPK3PXP",
  "qrCodeDataUrl": "data:image/png;base64,...",
  "otpauthUrl": "otpauth://totp/nSelf:user@example.com?secret=...",
  "backupCodes": ["12345678", "87654321", ...]
}
```

#### POST /api/mfa/totp/verify

Verify TOTP code (completes enrollment or validates login).

**Request Body:**
```json
{
  "userId": "user-123",
  "code": "123456"
}
```

**Response:**
```json
{
  "valid": true,
  "enrolled": true
}
```

#### POST /api/mfa/totp/validate

Validate TOTP code during login.

**Request Body:**
```json
{
  "userId": "user-123",
  "code": "123456"
}
```

**Response:**
```json
{
  "valid": true
}
```

#### POST /api/mfa/backup-code/validate

Validate a backup code.

**Request Body:**
```json
{
  "userId": "user-123",
  "code": "12345678"
}
```

**Response:**
```json
{
  "valid": true,
  "remainingBackupCodes": 9
}
```

#### DELETE /api/mfa/totp/:userId

Disable TOTP for user.

**Response:**
```json
{
  "success": true
}
```

#### GET /api/mfa/status/:userId

Check MFA enrollment status.

**Response:**
```json
{
  "enrolled": true,
  "method": "totp",
  "backupCodesRemaining": 8,
  "verified": true
}
```

### Magic Link Endpoints

#### POST /api/magic-link/send

Send a magic link email.

**Request Body:**
```json
{
  "email": "user@example.com",
  "purpose": "login",
  "redirectUrl": "https://app.com/auth/verify"
}
```

**Response:**
```json
{
  "sent": true,
  "expiresIn": 600
}
```

#### POST /api/magic-link/verify

Verify a magic link token.

**Request Body:**
```json
{
  "token": "random-token-value"
}
```

**Response:**
```json
{
  "valid": true,
  "userId": "user-123",
  "email": "user@example.com",
  "purpose": "login"
}
```

### Device Code Endpoints

#### POST /api/device-code/initiate

Start device code flow (TV login).

**Request Body:**
```json
{
  "deviceId": "tv-device-001",
  "deviceName": "Living Room TV",
  "deviceType": "android_tv"
}
```

**Response:**
```json
{
  "deviceCode": "xxxx-xxxx-xxxx",
  "userCode": "ABCD-1234",
  "verificationUrl": "https://app.com/activate",
  "expiresIn": 600,
  "pollInterval": 5
}
```

#### GET /api/device-code/poll

Device polls for authorization.

**Query Parameters:**
- `deviceCode` (required): Device code from initiate

**Response (pending):**
```json
{
  "status": "pending"
}
```

**Response (authorized):**
```json
{
  "status": "authorized",
  "userId": "user-123",
  "accessToken": "...",
  "refreshToken": "..."
}
```

#### POST /api/device-code/authorize

User authorizes device (from phone/web).

**Request Body:**
```json
{
  "userCode": "ABCD-1234",
  "userId": "user-123"
}
```

**Response:**
```json
{
  "authorized": true,
  "deviceName": "Living Room TV"
}
```

#### POST /api/device-code/deny

User denies device authorization.

**Request Body:**
```json
{
  "userCode": "ABCD-1234"
}
```

**Response:**
```json
{
  "denied": true
}
```

### Session Endpoints

#### GET /api/sessions/:userId

List active sessions for user.

**Response:**
```json
{
  "sessions": [
    {
      "id": "uuid",
      "deviceName": "iPhone 14 Pro",
      "deviceType": "mobile",
      "ipAddress": "192.168.1.100",
      "location": "San Francisco, US",
      "lastActivity": "2026-02-10T12:00:00Z",
      "authMethod": "oauth",
      "createdAt": "2026-02-10T08:00:00Z",
      "isCurrentSession": true
    }
  ]
}
```

#### DELETE /api/sessions/:sessionId

Revoke a specific session (remote logout).

**Request Body:**
```json
{
  "reason": "Security concern"
}
```

**Response:**
```json
{
  "revoked": true
}
```

#### DELETE /api/sessions/user/:userId

Revoke all sessions for user (force logout everywhere).

**Request Body:**
```json
{
  "exceptSessionId": "current-session-uuid",
  "reason": "Password reset"
}
```

**Response:**
```json
{
  "revoked": 5
}
```

### Login Attempts Endpoints

#### GET /api/login-attempts/:userId

Get recent login attempts.

**Query Parameters:**
- `limit` (optional, default 20): Number of attempts

**Response:**
```json
{
  "attempts": [
    {
      "id": "uuid",
      "method": "oauth",
      "outcome": "success",
      "failureReason": null,
      "ipAddress": "192.168.1.100",
      "userAgent": "Mozilla/5.0...",
      "createdAt": "2026-02-10T12:00:00Z"
    }
  ]
}
```

---

## Database Schema

### np_auth_oauth_providers

OAuth provider account connections.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | User ID |
| `provider` | VARCHAR(50) | Provider name (google, apple, etc.) |
| `provider_user_id` | VARCHAR(255) | Provider's user ID |
| `provider_email` | VARCHAR(255) | Email from provider |
| `provider_name` | VARCHAR(255) | Display name from provider |
| `provider_avatar_url` | TEXT | Avatar URL |
| `access_token_encrypted` | TEXT | Encrypted access token |
| `refresh_token_encrypted` | TEXT | Encrypted refresh token |
| `token_expires_at` | TIMESTAMPTZ | Token expiry |
| `scopes` | TEXT[] | OAuth scopes |
| `raw_profile` | JSONB | Full profile data |
| `linked_at` | TIMESTAMPTZ | When linked |
| `last_used_at` | TIMESTAMPTZ | Last authentication |

**Indexes:**
- `idx_auth_oauth_source_app` on `(source_account_id)`
- `idx_auth_oauth_user` on `(source_account_id, user_id)`

**Unique Constraints:**
- `(source_account_id, provider, provider_user_id)`
- `(source_account_id, user_id, provider)`

### np_auth_passkeys

WebAuthn/FIDO2 passkey credentials.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | User ID |
| `credential_id` | TEXT | WebAuthn credential ID |
| `public_key` | TEXT | Public key |
| `counter` | BIGINT | Signature counter (anti-replay) |
| `device_type` | VARCHAR(50) | platform or cross-platform |
| `backed_up` | BOOLEAN | Cloud backup status |
| `transports` | TEXT[] | Available transports |
| `friendly_name` | VARCHAR(255) | User-assigned name |
| `last_used_at` | TIMESTAMPTZ | Last authentication |
| `created_at` | TIMESTAMPTZ | Registration time |

**Indexes:**
- `idx_auth_passkeys_source_app` on `(source_account_id)`
- `idx_auth_passkeys_user` on `(source_account_id, user_id)`

**Unique Constraints:**
- `(source_account_id, credential_id)`

### np_auth_mfa_enrollments

TOTP 2FA enrollments.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | User ID |
| `method` | VARCHAR(50) | totp, sms, or email |
| `secret_encrypted` | TEXT | Encrypted TOTP secret |
| `algorithm` | VARCHAR(10) | SHA1, SHA256, SHA512 |
| `digits` | INTEGER | Code length (6 or 8) |
| `period` | INTEGER | Time period (seconds) |
| `verified` | BOOLEAN | Enrollment verified |
| `np_backup_codes_encrypted` | TEXT | Encrypted backup codes |
| `np_backup_codes_remaining` | INTEGER | Remaining codes |
| `enabled` | BOOLEAN | Currently enabled |
| `last_used_at` | TIMESTAMPTZ | Last verification |
| `created_at` | TIMESTAMPTZ | Enrollment time |

**Indexes:**
- `idx_auth_mfa_source_app` on `(source_account_id)`
- `idx_auth_mfa_user` on `(source_account_id, user_id)`

**Unique Constraints:**
- `(source_account_id, user_id, method)`

### np_auth_device_codes

Device code flow (TV login).

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `device_code` | VARCHAR(255) | Long device code |
| `user_code` | VARCHAR(20) | Short user-friendly code |
| `device_id` | VARCHAR(255) | Device identifier |
| `device_name` | VARCHAR(255) | Device name |
| `device_type` | VARCHAR(50) | android_tv, fire_tv, etc. |
| `scopes` | TEXT[] | Requested scopes |
| `status` | VARCHAR(20) | pending, authorized, denied, expired |
| `user_id` | VARCHAR(255) | User who authorized |
| `authorized_at` | TIMESTAMPTZ | Authorization time |
| `expires_at` | TIMESTAMPTZ | Expiry time |
| `poll_interval` | INTEGER | Polling interval (seconds) |
| `created_at` | TIMESTAMPTZ | Creation time |

**Indexes:**
- `idx_auth_device_codes_source_app` on `(source_account_id)`
- `idx_auth_device_codes_user_code` on `(source_account_id, user_code)`

**Unique Constraints:**
- `(source_account_id, device_code)`
- `(source_account_id, user_code)`

### np_auth_magic_links

Magic link tokens.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `email` | VARCHAR(255) | Email address |
| `token_hash` | VARCHAR(128) | Token hash |
| `purpose` | VARCHAR(50) | login, verify, reset |
| `used` | BOOLEAN | Token used |
| `used_at` | TIMESTAMPTZ | Usage time |
| `expires_at` | TIMESTAMPTZ | Expiry time |
| `ip_address` | VARCHAR(45) | Request IP |
| `created_at` | TIMESTAMPTZ | Creation time |

**Indexes:**
- `idx_auth_magic_links_source_app` on `(source_account_id)`
- `idx_auth_magic_links_email` on `(source_account_id, email)`

**Unique Constraints:**
- `(source_account_id, token_hash)`

### np_auth_sessions

User sessions.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | User ID |
| `device_id` | VARCHAR(255) | Device identifier |
| `device_name` | VARCHAR(255) | Device name |
| `device_type` | VARCHAR(50) | mobile, desktop, tv, etc. |
| `ip_address` | VARCHAR(45) | IP address |
| `user_agent` | TEXT | User agent string |
| `location_city` | VARCHAR(128) | City (from IP) |
| `location_country` | VARCHAR(10) | Country code |
| `np_auth_method` | VARCHAR(50) | password, oauth, passkey, magic_link, device_code |
| `token_hash` | VARCHAR(128) | Session token hash |
| `is_active` | BOOLEAN | Currently active |
| `last_activity_at` | TIMESTAMPTZ | Last activity |
| `expires_at` | TIMESTAMPTZ | Expiry time |
| `revoked_at` | TIMESTAMPTZ | Revocation time |
| `revoked_reason` | VARCHAR(255) | Revocation reason |
| `created_at` | TIMESTAMPTZ | Creation time |

**Indexes:**
- `idx_auth_sessions_source_app` on `(source_account_id)`
- `idx_auth_sessions_user` on `(source_account_id, user_id)`
- `idx_auth_sessions_active` on `(source_account_id, user_id, is_active)`

### np_auth_login_attempts

Login attempt tracking.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `email` | VARCHAR(255) | Email attempted |
| `user_id` | VARCHAR(255) | User ID (if known) |
| `ip_address` | VARCHAR(45) | IP address |
| `method` | VARCHAR(50) | password, oauth, passkey, magic_link, device_code |
| `outcome` | VARCHAR(20) | success, failure, blocked |
| `failure_reason` | VARCHAR(255) | Failure reason |
| `user_agent` | TEXT | User agent string |
| `created_at` | TIMESTAMPTZ | Attempt time |

**Indexes:**
- `idx_auth_login_attempts_source_app` on `(source_account_id)`
- `idx_auth_login_attempts_email` on `(source_account_id, email, created_at)`
- `idx_auth_login_attempts_ip` on `(source_account_id, ip_address, created_at)`

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `auth.oauth.linked` | OAuth account linked to user |
| `auth.oauth.unlinked` | OAuth account unlinked |
| `auth.passkey.registered` | New passkey registered |
| `auth.passkey.used` | Passkey authentication succeeded |
| `auth.mfa.enrolled` | MFA enrollment completed |
| `auth.mfa.verified` | MFA code verified during login |
| `auth.magic_link.sent` | Magic link email sent |
| `auth.magic_link.used` | Magic link verified |
| `auth.device_code.initiated` | Device code flow started |
| `auth.device_code.authorized` | Device code authorized by user |
| `auth.device_code.denied` | Device code denied |
| `auth.session.created` | New session created |
| `auth.session.revoked` | Session revoked (remote logout) |
| `auth.login.success` | Successful login (any method) |
| `auth.login.failure` | Failed login attempt |

---

## Multi-App Support

The auth plugin fully supports multi-app deployments with per-app isolation and configuration.

### Configuration

```bash
# Enable multi-app mode
AUTH_APP_IDS=famtv,famapp

# Per-app OAuth overrides
AUTH_FAMTV_GOOGLE_CLIENT_ID=famtv-google-client-id
AUTH_FAMAPP_GOOGLE_CLIENT_ID=famapp-google-client-id

# Per-app WebAuthn
AUTH_FAMTV_WEBAUTHN_RP_ID=famtv.com
AUTH_FAMAPP_WEBAUTHN_RP_ID=famapp.com
```

### Subdomain Routing

```
auth.famtv.example.com → Auth server (appId=famtv)
auth.famapp.example.com → Auth server (appId=famapp)
```

All data is isolated by `source_account_id` column.

---

## Security

### Encryption

- OAuth tokens and TOTP secrets are encrypted at rest using `AUTH_ENCRYPTION_KEY`
- Minimum key length: 32 characters
- Uses AES-256-GCM encryption

### Rate Limiting

- Login attempts tracked per email and IP
- Default: 5 attempts per 15 minutes
- Automatic lockout after threshold

### Session Security

- Session tokens hashed before storage
- Idle timeout: 24 hours (configurable)
- Absolute timeout: 30 days (configurable)
- Maximum sessions per user: 10 (configurable)

### WebAuthn

- FIDO2-certified authentication
- Counter-based replay protection
- Support for platform and cross-platform authenticators

---

## Troubleshooting

### Database Connection Errors

**Error:** `Connection refused`

**Solution:**
```bash
# Verify DATABASE_URL is set
echo $DATABASE_URL

# Test PostgreSQL connection
psql $DATABASE_URL -c "SELECT 1"
```

### OAuth Configuration

**Error:** `OAuth provider not configured`

**Solution:**
```bash
# Verify provider credentials are set
echo $AUTH_GOOGLE_CLIENT_ID
echo $AUTH_GOOGLE_CLIENT_SECRET

# Check provider is enabled
curl http://localhost:3014/api/oauth/providers
```

### Encryption Key Error

**Error:** `AUTH_ENCRYPTION_KEY must be at least 32 characters`

**Solution:**
```bash
# Generate a random 32-character key
export AUTH_ENCRYPTION_KEY=$(openssl rand -hex 32)
```

### Port Already in Use

**Error:** `EADDRINUSE: address already in use :::3014`

**Solution:**
```bash
# Use different port
export AUTH_PLUGIN_PORT=3015

# Or kill process on port 3014
lsof -ti:3014 | xargs kill -9
```

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Auth
- **Issues**: https://github.com/acamarata/nself-plugins/issues
- **License**: Source-Available

---

**Last Updated**: February 10, 2026
