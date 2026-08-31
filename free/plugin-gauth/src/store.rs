// Package store: encrypted refresh token storage in Postgres.
//
// Purpose: Load and save encrypted refresh tokens per account_id.
//   Tokens are AES-256-GCM encrypted before storage; decrypted on load.
// Inputs: sqlx PgPool, 32-byte encryption key, account_id.
// Outputs: Option<String> decrypted refresh token, or GauthError.
// Constraints:
//   - Never log or return the plaintext refresh token in error messages.
//   - source_account_id is 'primary' for single-tenant deployments.
//   - revoked flag is stored as status='revoked' in np_gauth_tokens.

use sqlx::PgPool;
use uuid::Uuid;

use crate::crypto::{decrypt, encrypt};
use crate::error::GauthError;

/// Load the decrypted refresh token for an account from Postgres.
/// Returns None if not found, GauthError::TokenRevoked if revoked.
pub async fn load_refresh_token(
    pool: &PgPool,
    key: &[u8; 32],
    account_id: &str,
) -> Result<Option<String>, GauthError> {
    let row: Option<(String, String)> = sqlx::query_as(
        "SELECT encrypted_token, status FROM np_gauth_tokens WHERE account_id = $1 LIMIT 1",
    )
    .bind(account_id)
    .fetch_optional(pool)
    .await?;

    match row {
        None => Ok(None),
        Some((_, status)) if status == "revoked" => {
            Err(GauthError::TokenRevoked(account_id.to_owned()))
        }
        Some((encrypted, _)) => {
            let token = decrypt(key, &encrypted)?;
            Ok(Some(token))
        }
    }
}

/// Save a refresh token for an account (encrypted). Upserts by account_id.
pub async fn save_refresh_token(
    pool: &PgPool,
    key: &[u8; 32],
    account_id: &str,
    refresh_token: &str,
) -> Result<(), GauthError> {
    let encrypted = encrypt(key, refresh_token)?;
    let id = Uuid::new_v4().to_string();
    sqlx::query(
        "INSERT INTO np_gauth_tokens (id, account_id, encrypted_token, status)
         VALUES ($1, $2, $3, 'active')
         ON CONFLICT (account_id) DO UPDATE SET encrypted_token = $3, status = 'active'",
    )
    .bind(id)
    .bind(account_id)
    .bind(encrypted)
    .execute(pool)
    .await?;
    Ok(())
}

/// Mark a refresh token as revoked in the DB.
pub async fn mark_revoked(pool: &PgPool, account_id: &str) -> Result<(), GauthError> {
    sqlx::query("UPDATE np_gauth_tokens SET status = 'revoked' WHERE account_id = $1")
        .bind(account_id)
        .execute(pool)
        .await?;
    Ok(())
}

/// List all accounts with token status and expiry metadata.
pub async fn list_accounts(pool: &PgPool) -> Result<Vec<AccountStatus>, GauthError> {
    let rows: Vec<(String, String, Option<chrono::DateTime<chrono::Utc>>)> = sqlx::query_as(
        "SELECT account_id, status, expires_hint FROM np_gauth_tokens ORDER BY account_id",
    )
    .fetch_all(pool)
    .await?;

    Ok(rows
        .into_iter()
        .map(|(account_id, status, expires_hint)| AccountStatus {
            account_id,
            status,
            expires_hint,
        })
        .collect())
}

/// Lightweight account status record (no token values).
#[derive(serde::Serialize)]
pub struct AccountStatus {
    pub account_id: String,
    pub status: String,
    pub expires_hint: Option<chrono::DateTime<chrono::Utc>>,
}
