// Package error: plugin-gauth error types.
//
// Purpose: Typed errors for gauth operations; maps to HTTP status codes.
// Constraints: Never include token values in error messages.

use thiserror::Error;

#[derive(Debug, Error)]
pub enum GauthError {
    #[error("account not found: {0}")]
    AccountNotFound(String),

    #[error("token revoked by Google for account: {0}")]
    TokenRevoked(String),

    #[error("rate limit exceeded")]
    RateLimited,

    #[error("crypto error")]
    CryptoError,

    #[error("database error: {0}")]
    Database(#[from] sqlx::Error),

    #[error("network error calling Google token endpoint")]
    Network(#[from] reqwest::Error),

    #[error("invalid response from Google token endpoint")]
    InvalidResponse,
}
