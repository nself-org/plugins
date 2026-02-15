# OAuth Provider Setup Guide

This guide shows how to configure OAuth authentication for all supported providers.

## Supported Providers

1. **Google** - Google Sign-In
2. **Apple** - Sign in with Apple
3. **Facebook** - Facebook Login
4. **GitHub** - GitHub OAuth
5. **Microsoft** - Microsoft Account (Azure AD)

---

## Configuration Format

All OAuth providers are configured in the `.env` file:

```bash
# Google OAuth
OAUTH_GOOGLE_CLIENT_ID=your-client-id
OAUTH_GOOGLE_CLIENT_SECRET=your-client-secret
OAUTH_GOOGLE_SCOPES=profile,email

# Apple Sign In
OAUTH_APPLE_CLIENT_ID=your-service-id
OAUTH_APPLE_TEAM_ID=your-team-id
OAUTH_APPLE_KEY_ID=your-key-id
OAUTH_APPLE_PRIVATE_KEY=your-private-key

# Facebook OAuth
OAUTH_FACEBOOK_APP_ID=your-app-id
OAUTH_FACEBOOK_APP_SECRET=your-app-secret

# GitHub OAuth
OAUTH_GITHUB_CLIENT_ID=your-client-id
OAUTH_GITHUB_CLIENT_SECRET=your-client-secret

# Microsoft OAuth
OAUTH_MICROSOFT_CLIENT_ID=your-client-id
OAUTH_MICROSOFT_CLIENT_SECRET=your-client-secret
```

---

## Provider-Specific Setup

### Google OAuth

**Step 1: Create OAuth Credentials**

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select existing
3. Navigate to **APIs & Services > Credentials**
4. Click **Create Credentials > OAuth 2.0 Client ID**
5. Select **Web application**
6. Add authorized redirect URIs:
   ```
   http://localhost:8002/api/oauth/google/callback
   https://yourdomain.com/api/oauth/google/callback
   ```
7. Copy the **Client ID** and **Client Secret**

**Step 2: Configure in `.env`**

```bash
OAUTH_GOOGLE_CLIENT_ID=123456789-abcdefg.apps.googleusercontent.com
OAUTH_GOOGLE_CLIENT_SECRET=GOCSPX-abc123def456
OAUTH_GOOGLE_SCOPES=profile,email
```

**Available Scopes:**
- `profile` - Basic profile information
- `email` - Email address
- `openid` - OpenID Connect

**Documentation:** https://developers.google.com/identity/protocols/oauth2

---

### Apple Sign In

**Step 1: Create App ID and Service ID**

1. Go to [Apple Developer](https://developer.apple.com/account/)
2. Navigate to **Certificates, Identifiers & Profiles**
3. Create an **App ID** (if you don't have one)
4. Create a **Services ID**:
   - Enable **Sign in with Apple**
   - Configure domains and redirect URLs:
     ```
     Domain: yourdomain.com
     Redirect URL: https://yourdomain.com/api/oauth/apple/callback
     ```
5. Create a **Key** for Sign in with Apple:
   - Enable **Sign in with Apple**
   - Download the `.p8` file (you can only download once!)

**Step 2: Configure in `.env`**

```bash
OAUTH_APPLE_CLIENT_ID=com.yourcompany.yourservice
OAUTH_APPLE_TEAM_ID=ABC123DEF4
OAUTH_APPLE_KEY_ID=XYZ123ABCD
OAUTH_APPLE_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----
MIGTAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBHkwdwIBAQQg...
-----END PRIVATE KEY-----"
```

**Notes:**
- Client ID is your Service ID
- Team ID is found in your Apple Developer account
- Key ID is shown when you create the key
- Private key is the contents of the `.p8` file

**Documentation:** https://developer.apple.com/sign-in-with-apple/get-started/

---

### Facebook Login

**Step 1: Create Facebook App**

1. Go to [Facebook Developers](https://developers.facebook.com/)
2. Click **My Apps > Create App**
3. Select **Consumer** app type
4. Add **Facebook Login** product
5. Configure **OAuth Redirect URIs**:
   ```
   http://localhost:8002/api/oauth/facebook/callback
   https://yourdomain.com/api/oauth/facebook/callback
   ```
6. Get **App ID** and **App Secret** from Settings > Basic

**Step 2: Configure in `.env`**

```bash
OAUTH_FACEBOOK_APP_ID=123456789012345
OAUTH_FACEBOOK_APP_SECRET=abc123def456ghi789jkl012mno345pq
```

**Available Scopes:**
- `email` - Email address
- `public_profile` - Name, profile picture, age range
- `user_friends` - Friend list (requires app review)

**Documentation:** https://developers.facebook.com/docs/facebook-login/

---

### GitHub OAuth

**Step 1: Create OAuth App**

1. Go to [GitHub Settings > Developer Settings](https://github.com/settings/developers)
2. Click **OAuth Apps > New OAuth App**
3. Fill in the form:
   - **Application name:** Your app name
   - **Homepage URL:** `https://yourdomain.com`
   - **Authorization callback URL:** `https://yourdomain.com/api/oauth/github/callback`
4. Copy **Client ID** and generate a **Client Secret**

**Step 2: Configure in `.env`**

```bash
OAUTH_GITHUB_CLIENT_ID=Iv1.abc123def456
OAUTH_GITHUB_CLIENT_SECRET=abc123def456ghi789jkl012mno345pqrst678uvw
```

**Available Scopes:**
- `user:email` - Email address
- `read:user` - User profile data
- `repo` - Repository access (requires approval)

**Documentation:** https://docs.github.com/en/apps/oauth-apps/building-oauth-apps

---

### Microsoft OAuth (Azure AD)

**Step 1: Register Application**

1. Go to [Azure Portal](https://portal.azure.com/)
2. Navigate to **Azure Active Directory > App registrations**
3. Click **New registration**
4. Fill in the form:
   - **Name:** Your app name
   - **Supported account types:** Accounts in any organizational directory and personal Microsoft accounts
   - **Redirect URI:** `https://yourdomain.com/api/oauth/microsoft/callback`
5. Go to **Certificates & secrets** and create a new client secret
6. Copy **Application (client) ID** and the **Client secret value**

**Step 2: Configure in `.env`**

```bash
OAUTH_MICROSOFT_CLIENT_ID=12345678-1234-1234-1234-123456789012
OAUTH_MICROSOFT_CLIENT_SECRET=abc~123.def456-ghi789_jkl012
```

**Available Scopes:**
- `openid` - OpenID Connect
- `profile` - User profile
- `email` - Email address
- `User.Read` - Read user profile from Microsoft Graph

**Documentation:** https://learn.microsoft.com/en-us/azure/active-directory/develop/v2-oauth2-auth-code-flow

---

## Callback URLs

All providers require you to register callback URLs. Use these formats:

**Development:**
```
http://localhost:8002/api/oauth/{provider}/callback
```

**Production:**
```
https://yourdomain.com/api/oauth/{provider}/callback
```

Replace `{provider}` with: `google`, `apple`, `facebook`, `github`, or `microsoft`

---

## OAuth Flow

### 1. Start OAuth Flow

**Endpoint:** `GET /api/oauth/{provider}/start`

**Query Parameters:**
- `redirectUri` (optional) - Override default callback URL
- `state` (optional) - Custom state parameter for CSRF protection
- `scopes` (optional) - Comma-separated scopes (overrides defaults)

**Response:**
```json
{
  "authorizationUrl": "https://accounts.google.com/o/oauth2/v2/auth?...",
  "state": "abc123def456"
}
```

**Usage:**
```javascript
// Get authorization URL
const response = await fetch('/api/oauth/google/start');
const { authorizationUrl, state } = await response.json();

// Redirect user to provider
window.location.href = authorizationUrl;
```

### 2. Handle OAuth Callback

**Endpoint:** `GET /api/oauth/{provider}/callback`

**Query Parameters:**
- `code` - Authorization code from provider
- `state` - State parameter (for CSRF validation)
- `error` - Error message (if authorization failed)

**Response (existing user):**
```json
{
  "userId": "user-id",
  "provider": "google",
  "providerEmail": "user@example.com",
  "providerName": "John Doe",
  "providerAvatarUrl": "https://...",
  "isNewUser": false
}
```

**Response (new user):**
```json
{
  "provider": "google",
  "providerUserId": "google-user-id",
  "providerEmail": "user@example.com",
  "providerName": "John Doe",
  "providerAvatarUrl": "https://...",
  "isNewUser": true
}
```

**Usage:**
```javascript
// For new users, create account first
if (data.isNewUser) {
  // Create user account
  const user = await createUser({ email: data.providerEmail });

  // Then link OAuth provider
  await fetch(`/api/oauth/${data.provider}/link`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      userId: user.id,
      code: authCode,
    }),
  });
}
```

### 3. Link OAuth Provider to Existing User

**Endpoint:** `POST /api/oauth/{provider}/link`

**Body:**
```json
{
  "userId": "user-id",
  "code": "authorization-code",
  "redirectUri": "optional-override"
}
```

**Response:**
```json
{
  "success": true,
  "provider": "google",
  "providerEmail": "user@example.com",
  "providerName": "John Doe"
}
```

### 4. List OAuth Connections

**Endpoint:** `GET /api/oauth/connections/{userId}`

**Response:**
```json
{
  "connections": [
    {
      "provider": "google",
      "providerEmail": "user@example.com",
      "providerName": "John Doe",
      "linkedAt": "2024-01-15T10:30:00Z",
      "lastUsedAt": "2024-02-15T14:20:00Z"
    }
  ]
}
```

### 5. Unlink OAuth Provider

**Endpoint:** `DELETE /api/oauth/{provider}/unlink`

**Body:**
```json
{
  "userId": "user-id"
}
```

**Response:**
```json
{
  "success": true
}
```

---

## Security Considerations

### State Parameter

Always use the `state` parameter to prevent CSRF attacks:

```javascript
// Generate random state
const state = crypto.randomUUID();
sessionStorage.setItem('oauth_state', state);

// Start OAuth flow with state
const response = await fetch(`/api/oauth/google/start?state=${state}`);

// Verify state in callback
const params = new URLSearchParams(window.location.search);
const returnedState = params.get('state');
if (returnedState !== sessionStorage.getItem('oauth_state')) {
  throw new Error('Invalid state parameter');
}
```

### Token Storage

OAuth tokens are encrypted before being stored in the database using the `SECURITY_ENCRYPTION_KEY`:

- Access tokens are encrypted with AES-256-GCM
- Refresh tokens are encrypted separately
- Tokens are only decrypted when needed for API calls

### Scopes

Only request the minimum scopes needed for your application:

- **Email + Profile** - Most common use case
- **Additional permissions** - Only if absolutely necessary

Users can see and revoke permissions at any time through their provider's settings.

---

## Testing

### Local Development

1. Set up OAuth apps in development mode
2. Use `http://localhost:8002` as callback domain
3. Test each provider independently
4. Check database for stored OAuth connections

### Production Checklist

- [ ] OAuth apps configured for production domain
- [ ] Callback URLs use HTTPS
- [ ] Client secrets stored securely in `.env` (not in code)
- [ ] State parameter validation enabled
- [ ] Token encryption key is strong (32+ characters)
- [ ] Scopes limited to minimum required
- [ ] Error handling for denied permissions
- [ ] Token refresh implemented (if using refresh tokens)

---

## Troubleshooting

### "OAuth not configured" error

**Cause:** Provider credentials missing or incomplete in `.env`

**Solution:** Verify all required environment variables are set for the provider

### "Invalid redirect_uri" error

**Cause:** Callback URL not registered with provider

**Solution:** Add the exact callback URL to provider's allowed redirect URIs

### "Invalid client" error

**Cause:** Wrong client ID or client secret

**Solution:** Double-check credentials from provider console

### "Insufficient scope" error

**Cause:** Requested data requires additional scopes

**Solution:** Add required scopes to provider configuration and re-authorize

### Tokens not refreshing

**Cause:** Refresh token not stored or expired

**Solution:** Check database for `refresh_token_encrypted` field and implement token refresh logic

---

## Advanced Usage

### Custom Scopes

Override default scopes per request:

```javascript
const response = await fetch('/api/oauth/google/start?scopes=profile,email,calendar');
```

### Multi-tenant OAuth

Each app can have its own OAuth credentials:

```typescript
// In index.ts
const appConfigs: AppAuthConfig[] = [
  {
    id: 'app1',
    oauth: {
      google: {
        clientId: process.env.APP1_GOOGLE_CLIENT_ID!,
        clientSecret: process.env.APP1_GOOGLE_CLIENT_SECRET!,
        scopes: ['profile', 'email'],
      },
    },
  },
  {
    id: 'app2',
    oauth: {
      google: {
        clientId: process.env.APP2_GOOGLE_CLIENT_ID!,
        clientSecret: process.env.APP2_GOOGLE_CLIENT_SECRET!,
        scopes: ['profile', 'email', 'calendar'],
      },
    },
  },
];
```

### Token Refresh (Coming Soon)

OAuth service will support automatic token refresh:

```typescript
// Future implementation
const validToken = await oauthService.getValidAccessToken(userId, provider);
```

---

## References

- [OAuth 2.0 Specification](https://oauth.net/2/)
- [OpenID Connect](https://openid.net/connect/)
- [PKCE Extension](https://oauth.net/2/pkce/)
- [Security Best Practices](https://oauth.net/2/oauth-best-practice/)
