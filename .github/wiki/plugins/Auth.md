# Auth Plugin

Advanced authentication plugin supporting OAuth (Google, Apple, Facebook, GitHub, Microsoft), WebAuthn/passkeys, TOTP 2FA with backup codes, magic links, device code flow, session management, and login attempt tracking.

| Property | Value |
|----------|-------|
| **Port** | `3014` |
| **Category** | `authentication` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run auth init
nself plugin run auth server
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `AUTH_ENCRYPTION_KEY` | Encryption key for tokens and secrets (AES-256) |

### Optional Environment Variables

#### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_PLUGIN_PORT` | `3014` | Server port |
| `AUTH_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `AUTH_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `AUTH_APP_IDS` | - | Comma-separated app IDs for multi-app |

#### OAuth Providers

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_GOOGLE_CLIENT_ID` | - | Google OAuth client ID |
| `AUTH_GOOGLE_CLIENT_SECRET` | - | Google OAuth client secret |
| `AUTH_GOOGLE_SCOPES` | `openid,email,profile` | Google OAuth scopes |
| `AUTH_APPLE_CLIENT_ID` | - | Apple Sign-In client ID |
| `AUTH_APPLE_TEAM_ID` | - | Apple Developer team ID |
| `AUTH_APPLE_KEY_ID` | - | Apple Sign-In key ID |
| `AUTH_APPLE_PRIVATE_KEY` | - | Apple Sign-In private key (PEM) |
| `AUTH_FACEBOOK_APP_ID` | - | Facebook app ID |
| `AUTH_FACEBOOK_APP_SECRET` | - | Facebook app secret |
| `AUTH_GITHUB_CLIENT_ID` | - | GitHub OAuth app client ID |
| `AUTH_GITHUB_CLIENT_SECRET` | - | GitHub OAuth app client secret |
| `AUTH_MICROSOFT_CLIENT_ID` | - | Microsoft (Azure AD) client ID |
| `AUTH_MICROSOFT_CLIENT_SECRET` | - | Microsoft (Azure AD) client secret |

#### WebAuthn

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_WEBAUTHN_RP_NAME` | - | Relying party name (your app name) |
| `AUTH_WEBAUTHN_RP_ID` | - | Relying party ID (domain, e.g., `example.com`) |
| `AUTH_WEBAUTHN_ORIGIN` | - | Expected origin (e.g., `https://example.com`) |

#### TOTP 2FA

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_TOTP_ISSUER` | - | TOTP issuer name (shown in authenticator apps) |
| `AUTH_TOTP_ALGORITHM` | `SHA1` | TOTP hash algorithm |
| `AUTH_TOTP_DIGITS` | `6` | Number of digits in TOTP code |
| `AUTH_TOTP_PERIOD` | `30` | TOTP time step in seconds |
| `AUTH_TOTP_BACKUP_CODE_COUNT` | `10` | Number of backup codes to generate |

#### Magic Links

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_MAGIC_LINK_EXPIRY_SECONDS` | `600` | Magic link token expiry (10 minutes) |
| `AUTH_MAGIC_LINK_BASE_URL` | - | Base URL for magic link verification page |

#### Device Code Flow

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_DEVICE_CODE_EXPIRY_SECONDS` | `600` | Device code expiry (10 minutes) |
| `AUTH_DEVICE_CODE_POLL_INTERVAL` | `5` | Minimum poll interval in seconds |
| `AUTH_DEVICE_CODE_LENGTH` | `8` | Length of user-visible code |

#### Sessions

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_SESSION_MAX_PER_USER` | `10` | Maximum concurrent sessions per user |
| `AUTH_SESSION_IDLE_TIMEOUT_HOURS` | `24` | Idle session timeout |
| `AUTH_SESSION_ABSOLUTE_TIMEOUT_HOURS` | `720` | Absolute session timeout (30 days) |

#### Security

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_LOGIN_MAX_ATTEMPTS` | `5` | Max failed login attempts before lockout |
| `AUTH_LOGIN_LOCKOUT_MINUTES` | `15` | Lockout duration after max failures |
| `AUTH_CLEANUP_CRON` | - | Cron expression for automatic cleanup |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (7 tables) |
| `server` | Start the HTTP API server |
| `sessions` | List active sessions for a user (`--user`) |
| `revoke-session` | Revoke a specific session (`--session-id`, `--reason?`) |
| `revoke-all` | Revoke all sessions for a user (`--user`, `--except?`, `--reason?`) |
| `mfa-status` | Check MFA enrollment status (`--user`) |
| `login-attempts` | View recent login attempts (`--user`, `--limit?`) |
| `oauth-connections` | List OAuth connections for a user (`--user`) |
| `cleanup-expired` | Clean up expired device codes, magic links, sessions, and old login attempts |
| `stats` | Show auth plugin statistics |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |
| `GET` | `/live` | Liveness with memory/uptime/stats |

### OAuth

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/oauth/providers` | List configured OAuth providers |
| `GET` | `/api/oauth/:provider/start` | Start OAuth flow (query: `redirectUri`, `state?`, `scopes?`) -- returns `authorizationUrl` |
| `GET` | `/api/oauth/:provider/callback` | Handle OAuth callback (query: `code`, `state`) -- returns user info and tokens |
| `POST` | `/api/oauth/:provider/link` | Link OAuth account to existing user (body: `userId`, `code`, `redirectUri`) |
| `DELETE` | `/api/oauth/:provider/unlink` | Unlink OAuth account (body: `userId`) |
| `GET` | `/api/oauth/connections/:userId` | List all OAuth connections for a user |

### WebAuthn / Passkeys

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/passkeys/register/start` | Start passkey registration (body: `userId`, `userName`, `userDisplayName`) -- returns `PublicKeyCredentialCreationOptions` |
| `POST` | `/api/passkeys/register/finish` | Complete passkey registration (body: `userId`, `credential`, `friendlyName?`) |
| `POST` | `/api/passkeys/authenticate/start` | Start passkey authentication (body: `userId?`) -- returns `PublicKeyCredentialRequestOptions` |
| `POST` | `/api/passkeys/authenticate/finish` | Complete passkey authentication (body: `credential`) -- returns `userId`, tokens |
| `GET` | `/api/passkeys/:userId` | List registered passkeys for a user |
| `DELETE` | `/api/passkeys/:credentialId` | Delete a passkey |

### MFA / TOTP

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/mfa/totp/enroll` | Enroll in TOTP (body: `userId`) -- returns `secret`, `qrCodeDataUrl`, `otpauthUrl`, `backupCodes[]` |
| `POST` | `/api/mfa/totp/verify` | Verify TOTP code to complete enrollment (body: `userId`, `code`) |
| `POST` | `/api/mfa/totp/validate` | Validate TOTP code during login (body: `userId`, `code`) |
| `POST` | `/api/mfa/backup-code/validate` | Validate a backup code (body: `userId`, `code`) -- returns `remainingBackupCodes` |
| `DELETE` | `/api/mfa/totp/:userId` | Remove TOTP enrollment |
| `GET` | `/api/mfa/status/:userId` | Get MFA enrollment status (`enrolled`, `method`, `backupCodesRemaining`, `verified`) |

### Magic Links

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/magic-link/send` | Send magic link (body: `email`, `purpose`, `redirectUrl?`) -- purpose: `login`, `verify`, `reset` |
| `POST` | `/api/magic-link/verify` | Verify magic link token (body: `token`) -- returns `valid`, `userId`, `email`, `purpose` |

### Device Code Flow

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/device-code/initiate` | Start device code flow (body: `deviceId?`, `deviceName?`, `deviceType?`) -- returns `deviceCode`, `userCode`, `verificationUrl`, `expiresIn`, `pollInterval` |
| `GET` | `/api/device-code/poll` | Poll for authorization (query: `deviceCode`) -- returns `status` (`pending`, `authorized`, `expired`, `denied`) |
| `POST` | `/api/device-code/authorize` | Authorize device code (body: `userCode`, `userId`) |
| `POST` | `/api/device-code/deny` | Deny device code (body: `userCode`) |

### Sessions

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/sessions/:userId` | List active sessions (returns device, IP, location, auth method) |
| `DELETE` | `/api/sessions/:sessionId` | Revoke a session (body: `reason?`) |
| `DELETE` | `/api/sessions/user/:userId` | Revoke all sessions (body: `exceptSessionId?`, `reason?`) |

### Login Attempts

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/login-attempts/:userId` | List login attempts (query: `limit?`) |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `auth.oauth.linked` | OAuth account linked |
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
| `auth.device_code.expired` | Device code expired |
| `auth.session.created` | New session created |
| `auth.session.revoked` | Session revoked (remote logout) |
| `auth.login.success` | Successful login (any method) |
| `auth.login.failure` | Failed login attempt |
| `auth.login.blocked` | Login blocked (rate limit or policy) |

---

## Authentication Methods

### OAuth

Supports five providers with encrypted token storage. Each provider stores `access_token`, `refresh_token`, `provider_user_id`, profile data, and scopes. Tokens are encrypted at rest using `AUTH_ENCRYPTION_KEY`.

| Provider | Config Required |
|----------|----------------|
| Google | `AUTH_GOOGLE_CLIENT_ID`, `AUTH_GOOGLE_CLIENT_SECRET` |
| Apple | `AUTH_APPLE_CLIENT_ID`, `AUTH_APPLE_TEAM_ID`, `AUTH_APPLE_KEY_ID`, `AUTH_APPLE_PRIVATE_KEY` |
| Facebook | `AUTH_FACEBOOK_APP_ID`, `AUTH_FACEBOOK_APP_SECRET` |
| GitHub | `AUTH_GITHUB_CLIENT_ID`, `AUTH_GITHUB_CLIENT_SECRET` |
| Microsoft | `AUTH_MICROSOFT_CLIENT_ID`, `AUTH_MICROSOFT_CLIENT_SECRET` |

### WebAuthn / Passkeys

FIDO2-compatible passwordless authentication. Stores credential public keys with counter values for replay protection. Tracks device type, backup status, and transport methods.

### TOTP 2FA

Time-based one-time passwords compatible with Google Authenticator, Authy, and similar apps. Enrollment generates a secret, QR code data URL, and `otpauth://` URL. Backup codes provide emergency access if the authenticator device is lost.

### Magic Links

Passwordless authentication via email. Tokens are SHA-256 hashed before storage. Supports three purposes: `login` (sign in), `verify` (email verification), `reset` (password reset). Links expire after `AUTH_MAGIC_LINK_EXPIRY_SECONDS`.

### Device Code Flow

For devices without a browser (smart TVs, CLI tools, IoT). The device displays a short `userCode` and `verificationUrl`. The user enters the code on a browser-equipped device to authorize. The original device polls `GET /api/device-code/poll` until authorization completes or the code expires.

---

## Database Schema

### `np_auth_oauth_providers`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | Application user ID |
| `provider` | `VARCHAR(50)` | `google`, `apple`, `facebook`, `github`, `microsoft` |
| `provider_user_id` | `VARCHAR(255)` | User ID from the provider |
| `provider_email` | `VARCHAR(255)` | Email from the provider |
| `provider_name` | `VARCHAR(255)` | Display name from the provider |
| `provider_avatar_url` | `TEXT` | Avatar URL from the provider |
| `access_token_encrypted` | `TEXT` | Encrypted OAuth access token |
| `refresh_token_encrypted` | `TEXT` | Encrypted OAuth refresh token |
| `token_expires_at` | `TIMESTAMPTZ` | Token expiration |
| `scopes` | `TEXT[]` | Granted OAuth scopes |
| `raw_profile` | `JSONB` | Full provider profile response |
| `linked_at` | `TIMESTAMPTZ` | When the account was linked |
| `last_used_at` | `TIMESTAMPTZ` | Last OAuth login |

### `np_auth_passkeys`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | Application user ID |
| `credential_id` | `VARCHAR(512)` | WebAuthn credential ID (Base64URL) |
| `public_key` | `TEXT` | COSE public key |
| `counter` | `INTEGER` | Signature counter (replay protection) |
| `device_type` | `VARCHAR(100)` | `platform` or `cross-platform` |
| `backed_up` | `BOOLEAN` | Whether key is backed up (e.g., iCloud Keychain) |
| `transports` | `TEXT[]` | `usb`, `ble`, `nfc`, `internal`, `hybrid` |
| `friendly_name` | `VARCHAR(255)` | User-assigned name |
| `last_used_at` | `TIMESTAMPTZ` | Last authentication |
| `created_at` | `TIMESTAMPTZ` | Registration time |

### `np_auth_mfa_enrollments`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Enrollment ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | Application user ID |
| `method` | `VARCHAR(50)` | `totp`, `sms`, `email` |
| `secret_encrypted` | `TEXT` | Encrypted TOTP secret |
| `algorithm` | `VARCHAR(20)` | Hash algorithm (default: SHA1) |
| `digits` | `INTEGER` | Code length (default: 6) |
| `period` | `INTEGER` | Time step in seconds (default: 30) |
| `verified` | `BOOLEAN` | Whether enrollment is verified |
| `backup_codes_encrypted` | `TEXT` | Encrypted backup codes (JSON array) |
| `backup_codes_remaining` | `INTEGER` | Number of unused backup codes |
| `enabled` | `BOOLEAN` | Whether MFA is active |
| `last_used_at` | `TIMESTAMPTZ` | Last TOTP validation |
| `created_at` | `TIMESTAMPTZ` | Enrollment time |

### `np_auth_device_codes`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `device_code` | `VARCHAR(255)` | Device-side code (long, random) |
| `user_code` | `VARCHAR(20)` | User-visible short code |
| `device_id` | `VARCHAR(255)` | Device identifier |
| `device_name` | `VARCHAR(255)` | Device display name |
| `device_type` | `VARCHAR(100)` | Device type (TV, CLI, IoT) |
| `scopes` | `TEXT[]` | Requested scopes |
| `status` | `VARCHAR(20)` | `pending`, `authorized`, `denied`, `expired` |
| `user_id` | `VARCHAR(255)` | User who authorized (set on authorization) |
| `authorized_at` | `TIMESTAMPTZ` | Authorization timestamp |
| `expires_at` | `TIMESTAMPTZ` | Code expiration |
| `poll_interval` | `INTEGER` | Minimum poll interval (seconds) |
| `created_at` | `TIMESTAMPTZ` | Creation time |

### `np_auth_magic_links`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `email` | `VARCHAR(255)` | Recipient email |
| `token_hash` | `VARCHAR(128)` | SHA-256 hash of the token |
| `purpose` | `VARCHAR(20)` | `login`, `verify`, `reset` |
| `used` | `BOOLEAN` | Whether the link has been used |
| `used_at` | `TIMESTAMPTZ` | When the link was used |
| `expires_at` | `TIMESTAMPTZ` | Link expiration |
| `ip_address` | `VARCHAR(45)` | IP address of the requester |
| `created_at` | `TIMESTAMPTZ` | Creation time |

### `np_auth_sessions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Session ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | Session owner |
| `device_id` | `VARCHAR(255)` | Device identifier |
| `device_name` | `VARCHAR(255)` | Device display name |
| `device_type` | `VARCHAR(100)` | Device type |
| `ip_address` | `VARCHAR(45)` | Client IP |
| `user_agent` | `TEXT` | Client user agent |
| `location_city` | `VARCHAR(128)` | Geo city |
| `location_country` | `VARCHAR(128)` | Geo country |
| `auth_method` | `VARCHAR(50)` | `password`, `oauth`, `passkey`, `magic_link`, `device_code`, `mfa` |
| `token_hash` | `VARCHAR(128)` | Session token hash |
| `is_active` | `BOOLEAN` | Whether session is active |
| `last_activity_at` | `TIMESTAMPTZ` | Last activity timestamp |
| `expires_at` | `TIMESTAMPTZ` | Absolute expiration |
| `revoked_at` | `TIMESTAMPTZ` | When revoked |
| `revoked_reason` | `TEXT` | Revocation reason |
| `created_at` | `TIMESTAMPTZ` | Session creation time |

### `np_auth_login_attempts`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Attempt ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `email` | `VARCHAR(255)` | Email used in attempt |
| `user_id` | `VARCHAR(255)` | Resolved user ID (if any) |
| `ip_address` | `VARCHAR(45)` | Client IP |
| `method` | `VARCHAR(50)` | Auth method attempted |
| `outcome` | `VARCHAR(20)` | `success`, `failure`, `blocked` |
| `failure_reason` | `TEXT` | Reason for failure |
| `user_agent` | `TEXT` | Client user agent |
| `created_at` | `TIMESTAMPTZ` | Attempt timestamp |

---

## Session Management

Sessions track device, IP, location, and authentication method. The plugin enforces:

- **Max concurrent sessions**: `AUTH_SESSION_MAX_PER_USER` (default: 10). Oldest sessions are evicted when the limit is exceeded.
- **Idle timeout**: Sessions with no activity for `AUTH_SESSION_IDLE_TIMEOUT_HOURS` are expired.
- **Absolute timeout**: Sessions older than `AUTH_SESSION_ABSOLUTE_TIMEOUT_HOURS` are expired regardless of activity.

Use `cleanup-expired` CLI command or set `AUTH_CLEANUP_CRON` for automatic expiration.

---

## Troubleshooting

**"Encryption key not configured"** -- Set `AUTH_ENCRYPTION_KEY` environment variable. This is required for encrypting OAuth tokens, TOTP secrets, and backup codes.

**OAuth provider not listed** -- Set the corresponding `AUTH_<PROVIDER>_CLIENT_ID` and secret. Only configured providers appear in `GET /api/oauth/providers`.

**Passkey registration fails** -- Verify `AUTH_WEBAUTHN_RP_ID` matches your domain and `AUTH_WEBAUTHN_ORIGIN` matches the page origin exactly (including scheme and port).

**TOTP codes rejected** -- Check system clock synchronization. TOTP is time-sensitive with a 30-second window. Verify the user completed enrollment verification via `POST /api/mfa/totp/verify`.

**Device code expired before authorization** -- Increase `AUTH_DEVICE_CODE_EXPIRY_SECONDS`. Default is 600 seconds (10 minutes).

**Sessions not expiring** -- Run `cleanup-expired` manually or configure `AUTH_CLEANUP_CRON`. The `idle_timeout` and `absolute_timeout` are checked on cleanup, not on every request.
