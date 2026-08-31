# ID.me Plugin for nself

OAuth authentication with government-grade identity verification for military, veterans, first responders, government employees, teachers, students, and nurses.

## Features

- **Complete OAuth 2.0 Flow** - Authorization, token exchange, refresh
- **7 Verification Groups** - Military, Veteran, First Responder, Government, Teacher, Student, Nurse
- **Badge Management** - Visual badges for verified groups
- **Attribute Tracking** - Store verified user attributes (branch, rank, affiliation, etc.)
- **Webhook Support** - Real-time verification updates
- **Sandbox Mode** - Test with ID.me sandbox environment
- **CSRF Protection** - State parameter for secure OAuth flow
- **TypeScript Implementation** - Full type safety
- **REST API** - Query verification status via HTTP

## Installation

### Prerequisites

- Node.js >= 20.0.0
- PostgreSQL database
- ID.me developer account

### Setup

1. **Register your application at ID.me**

   Visit [developers.id.me](https://developers.id.me) and create a new OAuth application.

2. **Install the plugin**

   ```bash
   cd plugins/idme
   ./install.sh
   ```

3. **Configure environment variables**

   ```bash
   cp .env.example .env
   # Edit .env with your credentials
   ```

4. **Install TypeScript dependencies**

   ```bash
   cd ts
   npm install
   npm run build
   ```

## Configuration

Create a `.env` file with the following variables:

```bash
# Required
IDME_CLIENT_ID=your_client_id
IDME_CLIENT_SECRET=your_client_secret
IDME_REDIRECT_URI=https://your-domain.com/callback/idme
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Optional
IDME_SCOPES=openid,email,profile,military,veteran
IDME_SANDBOX=false
IDME_WEBHOOK_SECRET=your_webhook_secret
PORT=3010
```

### OAuth Scopes

The plugin supports the following verification scopes:

| Scope | Description |
|-------|-------------|
| `openid` | OpenID Connect (required) |
| `email` | User's email address (required) |
| `profile` | Basic profile information (required) |
| `military` | Active duty military verification |
| `veteran` | Military veteran verification |
| `first_responder` | First responder verification (police, fire, EMT) |
| `government` | Government employee verification |
| `teacher` | Teacher/educator verification |
| `student` | Student verification |
| `nurse` | Nurse/healthcare worker verification |

**Example**: To verify military and veterans, use:
```bash
IDME_SCOPES=openid,email,profile,military,veteran
```

## Usage

### CLI Commands

```bash
# Initialize OAuth configuration
nself plugin idme init

# Generate authorization URL
nself plugin idme init auth

# Check verification status
nself plugin idme verify user@example.com

# List all groups
nself plugin idme groups list

# Show users in a specific group
nself plugin idme groups type military

# Test configuration
nself plugin idme test
```

### TypeScript/Node.js

```bash
# Start HTTP server
cd ts
npm run dev

# Or use the CLI
npx nself-idme server --port 3010
```

### HTTP Server

The plugin provides an HTTP server for OAuth callbacks and webhooks:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/auth/idme` | Start OAuth flow (redirects to ID.me) |
| GET | `/callback/idme` | OAuth callback handler |
| POST | `/webhook/idme` | Webhook receiver |
| GET | `/api/verifications/:userId` | Get verification status |

### OAuth Flow

1. **Redirect user to authorization URL**

   ```bash
   # Generate URL
   nself plugin idme init auth

   # Or start server and visit
   http://localhost:3010/auth/idme
   ```

2. **User authenticates with ID.me** and grants permissions

3. **ID.me redirects back** to your callback URL with code

4. **Exchange code for tokens** and fetch user data

   The callback handler automatically:
   - Exchanges authorization code for access token
   - Fetches user profile
   - Fetches verification groups
   - Stores data in database

### Programmatic Usage

```typescript
import { createIDmeClient, createDatabase } from '@nself/plugin-idme';

// Create client
const client = createIDmeClient({
  clientId: process.env.IDME_CLIENT_ID!,
  clientSecret: process.env.IDME_CLIENT_SECRET!,
  redirectUri: process.env.IDME_REDIRECT_URI!,
  scopes: ['openid', 'email', 'profile', 'military', 'veteran'],
  sandbox: false,
});

// Start OAuth flow
const state = 'random-csrf-token';
const authUrl = client.getAuthorizationUrl(state);
console.log('Visit:', authUrl);

// Exchange code for tokens
const tokens = await client.exchangeCode(code);

// Get user profile
const profile = await client.getUserProfile(tokens.accessToken);

// Get verifications
const verification = await client.getVerifications(tokens.accessToken);

// Store in database
const db = createDatabase();
await db.upsertVerification(userId, idmeUserId, profile, tokens, verification);
```

## Database Schema

### Tables

| Table | Description |
|-------|-------------|
| `idme_verifications` | Main verification records with OAuth tokens |
| `idme_groups` | Verification groups (military, veteran, etc.) |
| `idme_badges` | Visual badges for verified groups |
| `idme_attributes` | Additional verified attributes |
| `idme_webhook_events` | Webhook event log |

### Views

| View | Description |
|------|-------------|
| `idme_verified_users` | All verified users with groups and badges |
| `idme_group_summary` | Group verification counts |
| `idme_recent_verifications` | Recent verifications (last 30 days) |

### Example Queries

```sql
-- Get all verified users
SELECT * FROM idme_verified_users;

-- Count verifications by group
SELECT * FROM idme_group_summary;

-- Find military veterans
SELECT * FROM idme_groups
WHERE group_type IN ('military', 'veteran')
  AND verified = TRUE;

-- Get user's badges
SELECT * FROM idme_badges
WHERE user_id = 'user-uuid-here'
  AND active = TRUE
ORDER BY display_order;
```

## Webhooks

ID.me can send webhooks for verification updates. The plugin supports:

| Event | Description |
|-------|-------------|
| `verification.created` | New verification created |
| `verification.updated` | Verification status updated |
| `verification.completed` | Verification completed successfully |
| `verification.failed` | Verification failed |
| `group.verified` | User verified for a specific group |
| `group.revoked` | Group verification revoked |
| `attribute.updated` | User attribute updated |

### Webhook Setup

1. **Set webhook secret** in your `.env`:
   ```bash
   IDME_WEBHOOK_SECRET=your_secret_here
   ```

2. **Configure webhook in ID.me dashboard**:
   - URL: `https://your-domain.com/webhook/idme`
   - Events: Select all verification events

3. **Webhooks are automatically verified** using HMAC-SHA256 signature

## Verification Groups

The plugin supports 7 verification groups:

### Military
- **Icon**: 🪖
- **Color**: Green (#2E7D32)
- **Attributes**: branch, rank, status, affiliation
- **Use case**: Active duty military discounts/access

### Veteran
- **Icon**: 🎖️
- **Color**: Blue (#1565C0)
- **Attributes**: branch, service_era, rank, affiliation
- **Use case**: Veteran benefits and services

### First Responder
- **Icon**: 🚨
- **Color**: Red (#C62828)
- **Attributes**: department, role
- **Use case**: First responder discounts

### Government
- **Icon**: 🏛️
- **Color**: Purple (#6A1B9A)
- **Attributes**: agency, level
- **Use case**: Government employee services

### Teacher
- **Icon**: 📚
- **Color**: Orange (#F57C00)
- **Attributes**: school, subject
- **Use case**: Teacher discounts and resources

### Student
- **Icon**: 🎓
- **Color**: Teal (#00897B)
- **Attributes**: school, graduation_year
- **Use case**: Student discounts

### Nurse
- **Icon**: ⚕️
- **Color**: Pink (#AD1457)
- **Attributes**: specialty, license
- **Use case**: Healthcare worker benefits

## Security

### CSRF Protection
- State parameter validates OAuth callbacks
- Stored in secure HTTP-only cookie

### Token Storage
- Access tokens encrypted at rest (in production)
- Refresh tokens stored securely
- Token expiration tracked

### Webhook Verification
- HMAC-SHA256 signature verification
- Prevents unauthorized webhook calls

### Environment Separation
- Sandbox mode for testing
- Production mode for live data

## Development

### Build

```bash
cd ts
npm run build
```

### Watch Mode

```bash
npm run watch
```

### Type Check

```bash
npm run typecheck
```

### Start Dev Server

```bash
npm run dev
```

## Testing

### Sandbox Mode

Enable sandbox mode for testing:

```bash
IDME_SANDBOX=true
```

This uses ID.me's test environment at `api.idmelabs.com`.

### Test Commands

```bash
# Test configuration
nself plugin idme test config

# Test database
nself plugin idme test database

# Test API connectivity
nself plugin idme test api

# Run all tests
nself plugin idme test
```

## Troubleshooting

### OAuth Errors

**Problem**: "Invalid redirect_uri"
- **Solution**: Ensure `IDME_REDIRECT_URI` matches exactly what's registered in ID.me dashboard

**Problem**: "Invalid client credentials"
- **Solution**: Verify `IDME_CLIENT_ID` and `IDME_CLIENT_SECRET` are correct

### Database Errors

**Problem**: "relation idme_verifications does not exist"
- **Solution**: Run `./install.sh` to create tables

**Problem**: "Connection refused"
- **Solution**: Check `DATABASE_URL` is correct and PostgreSQL is running

### Webhook Errors

**Problem**: "Invalid signature"
- **Solution**: Ensure `IDME_WEBHOOK_SECRET` matches ID.me webhook configuration

## Support

- **Documentation**: [ID.me API Docs](https://developers.id.me)
- **Plugin Issues**: [GitHub Issues](https://github.com/acamarata/nself-plugins/issues)
- **ID.me Support**: support@id.me

## License

MIT

## Related

- [ID.me Developer Portal](https://developers.id.me)
- [OAuth 2.0 Specification](https://oauth.net/2/)
- [nself CLI](https://github.com/acamarata/nself)
