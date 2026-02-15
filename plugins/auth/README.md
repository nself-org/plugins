# auth

Authentication plugin with sessions and login tracking.

## Current Features

### ✅ Session Management
- List active sessions for a user
- Revoke specific session (remote logout)
- Revoke all sessions for a user
- Session tracking with device info, IP, location

### ✅ Login Tracking
- Login attempt monitoring
- Failed login detection
- Rate limiting and lockout policies
- Login history per user

### ✅ Database Schema
- All tables created and ready for planned features
- Multi-tenant isolation support
- Webhook event storage

### ✅ TOTP 2FA
- TOTP enrollment with QR codes
- TOTP verification during enrollment
- TOTP validation during login
- Backup code generation (10 codes)
- Backup code validation with auto-removal

**Endpoints:** `/api/mfa/totp/enroll`, `/api/mfa/totp/verify`, `/api/mfa/totp/validate`, `/api/mfa/backup-code/validate`

## Planned Features

The following features have database schema and API endpoints prepared, but return HTTP 501 (Not Implemented) until external dependencies are integrated:

### 🔄 OAuth Authentication (Planned)
**Status:** Requires provider SDKs (passport.js or similar)

Providers ready to integrate:
- Google OAuth 2.0
- Apple Sign In
- Facebook Login
- GitHub OAuth
- Microsoft Azure AD

**Endpoints:** `/api/oauth/start`, `/api/oauth/callback`, `/api/oauth/link`

### 🔄 WebAuthn/Passkeys (Planned)
**Status:** Requires @simplewebauthn/server library

Features:
- Passkey registration
- Passkey authentication
- Device management

**Endpoints:** `/api/passkeys/register/start`, `/api/passkeys/register/finish`, `/api/passkeys/auth/start`, `/api/passkeys/auth/finish`

### 🔄 Magic Links (Planned)
**Status:** Requires crypto module and notifications plugin integration

Features:
- Email-based magic link authentication
- Configurable expiry times
- One-time use tokens

**Endpoints:** `/api/magic-link/send`, `/api/magic-link/verify`

### 🔄 Device Code Flow (Planned)
**Status:** Requires crypto module for secure code generation

Features:
- OAuth 2.0 device authorization grant
- TV/device authentication
- Polling and authorization

**Endpoints:** `/api/device-code/initiate`, `/api/device-code/poll`, `/api/device-code/authorize`

## Installation

```bash
nself plugin install auth
```

## Configuration

### Required Environment Variables

```bash
AUTH_ENCRYPTION_KEY=<random-32-byte-key>
DATABASE_URL=postgresql://...
```

### Optional OAuth Configuration (when implemented)

```bash
# Google OAuth
AUTH_GOOGLE_CLIENT_ID=...
AUTH_GOOGLE_CLIENT_SECRET=...

# Apple Sign In
AUTH_APPLE_CLIENT_ID=...
AUTH_APPLE_TEAM_ID=...
AUTH_APPLE_KEY_ID=...
AUTH_APPLE_PRIVATE_KEY=...

# Facebook Login
AUTH_FACEBOOK_APP_ID=...
AUTH_FACEBOOK_APP_SECRET=...

# GitHub OAuth
AUTH_GITHUB_CLIENT_ID=...
AUTH_GITHUB_CLIENT_SECRET=...

# Microsoft Azure AD
AUTH_MICROSOFT_CLIENT_ID=...
AUTH_MICROSOFT_CLIENT_SECRET=...
```

### Production-Required Configuration

**IMPORTANT:** The following environment variables are REQUIRED for production deployments. The default `localhost` values only work for local development.

#### WebAuthn/Passkeys Configuration

```bash
# REQUIRED for production - must match your public domain
AUTH_WEBAUTHN_RP_ID=yourdomain.com
AUTH_WEBAUTHN_ORIGIN=https://yourdomain.com

# Example for staging
AUTH_WEBAUTHN_RP_ID=staging.yourdomain.com
AUTH_WEBAUTHN_ORIGIN=https://staging.yourdomain.com
```

**Why required:** WebAuthn security requires the Relying Party ID (RP ID) and origin to match your publicly accessible domain. Passkeys will not work with `localhost` in production.

#### Magic Link Configuration

```bash
# REQUIRED for production - must be publicly accessible
AUTH_MAGIC_LINK_BASE_URL=https://yourdomain.com

# Example for staging
AUTH_MAGIC_LINK_BASE_URL=https://staging.yourdomain.com
```

**Why required:** Magic links are sent in emails and clicked by users from their email clients. They must point to your publicly accessible URL, not `localhost`.

#### Docker Deployment Notes

When deploying in Docker:
- **Internal service URLs** use Docker service names (e.g., `postgres:5432`, `redis:6379`)
- **External user-facing URLs** must use public domains (WebAuthn, Magic Links, OAuth redirects)
- Set these environment variables in your `.env` file or deployment configuration

See `plugin.json` for complete environment variable reference.

## Usage

### Current CLI Commands

```bash
# Session management (works now)
nself plugin auth sessions --user-id <userId>
nself plugin auth revoke-session --session-id <sessionId>
nself plugin auth revoke-all --user-id <userId>

# Login tracking (works now)
nself plugin auth login-attempts --user-id <userId>

# Utilities
nself plugin auth stats
nself plugin auth cleanup-expired
```

### Planned CLI Commands

These commands exist but require feature implementation:

```bash
# OAuth (planned)
nself plugin auth oauth-connections --user-id <userId>

# MFA (planned)
nself plugin auth mfa-status --user-id <userId>
```

## API Endpoints

### Sessions (Available Now)

```http
GET /api/sessions/:userId
# Returns active sessions for user

DELETE /api/sessions/:sessionId
# Revoke specific session

DELETE /api/sessions/user/:userId
# Revoke all sessions for user
```

### Login Attempts (Available Now)

```http
GET /api/login-attempts/:userId
# Returns recent login attempts
```

### Planned Endpoints

All other endpoints return HTTP 501 until their respective features are implemented. See "Planned Features" section above for details.

## Webhooks

The plugin defines 15 webhook events (see `plugin.json`), but these will only fire once their corresponding features are implemented:

### Currently Active Webhooks
- `auth.session.created` - Fires when session is created
- `auth.session.revoked` - Fires when session is revoked
- `auth.login.success` - Fires on successful login
- `auth.login.failure` - Fires on failed login attempt
- `auth.login.blocked` - Fires when login is blocked
- `auth.mfa.enrolled` - Fires when MFA is enrolled
- `auth.mfa.verified` - Fires when MFA enrollment is verified

### Planned Webhooks
- OAuth events (linked, unlinked)
- Passkey events (registered, used)
- Magic link events (sent, used)
- Device code events (initiated, authorized, denied, expired)

## Development Roadmap

### Phase 1: Current State ✅
- Session management
- Login tracking
- Database schema
- Webhook infrastructure

### Phase 2: OAuth Integration (Planned)
**Estimated Effort:** 8-12 hours

- Add passport.js or OAuth provider SDKs
- Implement 5 OAuth providers
- Add OAuth account linking
- Test with real provider credentials

### Phase 3: WebAuthn/Passkeys (Planned)
**Estimated Effort:** 6-8 hours

- Add @simplewebauthn/server library
- Implement registration flow
- Implement authentication flow
- Test with hardware keys and platform authenticators

### Phase 4: TOTP 2FA ✅
**Status:** Implemented

- ✅ Added otplib library
- ✅ QR code generation for enrollment
- ✅ TOTP verification flow
- ✅ Backup code system (10 codes with auto-removal)

### Phase 5: Magic Links (Planned)
**Estimated Effort:** 3-4 hours

- Integrate with notifications plugin
- Generate secure tokens
- Implement verification
- Add expiry and rate limiting

### Phase 6: Device Code Flow (Planned)
**Estimated Effort:** 4-5 hours

- Implement RFC 8628 spec
- Generate user-friendly codes
- Add polling mechanism
- Build authorization UI

**Remaining Estimated Effort:** 21-30 hours (TOTP 2FA completed)

## Migration Guide

When OAuth, WebAuthn, TOTP, Magic Links, or Device Code features are implemented:

1. Database schema is already in place (no migrations needed)
2. Environment variables will need to be added (see Configuration section)
3. API endpoints will change from 501 to functional responses
4. Webhooks will begin firing for those events
5. CLI commands will become functional

No breaking changes are expected - features will transparently upgrade from "not implemented" to working.

## Testing

Current features can be tested with:

```bash
# Session management
curl http://localhost:3014/api/sessions/user123

# Login tracking
curl http://localhost:3014/api/login-attempts/user123
```

Planned features will return:

```json
{
  "error": "OAuth start not implemented - requires provider SDKs"
}
```

## License

See LICENSE file in repository root.

## Contributing

See CONTRIBUTING.md in repository root.
