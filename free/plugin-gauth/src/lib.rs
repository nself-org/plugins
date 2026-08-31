// Package plugin-gauth: headless server-side Google OAuth token refresh service.
//
// Purpose: Manages refresh tokens stored encrypted at rest. Refreshes access tokens
//   on demand via Google OAuth2 token endpoint. No browser interaction at refresh time.
// Inputs: account_id identifying a pre-provisioned refresh token secret.
// Outputs: {access_token, expires_at, account_id} JSON; or 401/404 errors.
// Constraints:
//   - Refresh tokens are NEVER logged or returned in any response.
//   - Tokens stored encrypted using AES-256-GCM; key from GAUTH_ENCRYPTION_KEY env.
//   - In-memory cache with TTL from Google expires_in field.
//   - SSRF guard: only calls https://oauth2.googleapis.com/token.
//   - source_account_id convention for multi-app isolation.

pub mod cache;
pub mod crypto;
pub mod error;
pub mod store;
pub mod token;
