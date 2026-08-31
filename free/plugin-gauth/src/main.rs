// Package main: plugin-gauth HTTP service entry point.
//
// Purpose: Start the gauth Axum HTTP server on GAUTH_PORT (default 3762).
//   Exposes POST /refresh, GET /status, DELETE /token for headless Google OAuth.
// Inputs: NSELF_DB_URL, GAUTH_ENCRYPTION_KEY, GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET env vars.
// Outputs: JSON responses; access tokens transmitted in-response (not logged).
// Constraints:
//   - Refresh tokens never appear in logs or response bodies.
//   - Token cache is in-memory; restart clears all cached tokens (triggers re-refresh).
//   - SSRF: only calls GOOGLE_TOKEN_URL (https://oauth2.googleapis.com/token by default).

use std::{env, sync::Arc, time::Duration};

use axum::{
    extract::{Query, State},
    http::StatusCode,
    response::Json,
    routing::{delete, get, post},
    Router,
};
use serde::{Deserialize, Serialize};
use sqlx::PgPool;

use plugin_gauth::{cache::TokenCache, error::GauthError, store, token::refresh_access_token};

struct AppState {
    pool: PgPool,
    cache: TokenCache,
    enc_key: [u8; 32],
    http_client: reqwest::Client,
    google_token_url: String,
    client_id: String,
    client_secret: String,
}

#[derive(Deserialize)]
struct AccountQuery {
    account_id: String,
}

#[derive(Deserialize)]
struct ForceQuery {
    account_id: String,
    #[serde(default)]
    force: bool,
}

#[derive(Serialize)]
struct RefreshResponse {
    access_token: String,
    expires_at: String,
    account_id: String,
}

#[derive(Serialize)]
struct ErrorResponse {
    error: String,
}

/// POST /refresh?account_id=X[&force=true]
/// Returns a valid access token. Uses cache unless force=true.
async fn handle_refresh(
    State(state): State<Arc<AppState>>,
    Query(params): Query<ForceQuery>,
) -> Result<Json<RefreshResponse>, (StatusCode, Json<ErrorResponse>)> {
    let account_id = &params.account_id;

    // Check cache first unless force-refresh requested
    if !params.force {
        if let Some(cached_token) = state.cache.get(account_id) {
            return Ok(Json(RefreshResponse {
                access_token: cached_token,
                expires_at: "cached".to_owned(),
                account_id: account_id.clone(),
            }));
        }
    }

    // Load encrypted refresh token from DB
    let refresh_token =
        match store::load_refresh_token(&state.pool, &state.enc_key, account_id).await {
            Ok(Some(t)) => t,
            Ok(None) => {
                return Err((
                    StatusCode::NOT_FOUND,
                    Json(ErrorResponse {
                        error: format!("account not found: {account_id}"),
                    }),
                ))
            }
            Err(GauthError::TokenRevoked(_)) => {
                state.cache.invalidate(account_id);
                return Err((
                    StatusCode::UNAUTHORIZED,
                    Json(ErrorResponse {
                        error: "token_revoked".to_owned(),
                    }),
                ));
            }
            Err(e) => {
                tracing::error!("store error for account={account_id}: {e}");
                return Err((
                    StatusCode::INTERNAL_SERVER_ERROR,
                    Json(ErrorResponse {
                        error: "internal error".to_owned(),
                    }),
                ));
            }
        };

    // Call Google token endpoint
    match refresh_access_token(
        &state.http_client,
        &state.google_token_url,
        &state.client_id,
        &state.client_secret,
        &refresh_token,
    )
    .await
    {
        Ok(resp) => {
            state
                .cache
                .set(account_id, resp.access_token.clone(), resp.expires_in);
            let expires_at = chrono::Utc::now() + chrono::Duration::seconds(resp.expires_in as i64);
            Ok(Json(RefreshResponse {
                access_token: resp.access_token,
                expires_at: expires_at.to_rfc3339(),
                account_id: account_id.clone(),
            }))
        }
        Err(GauthError::TokenRevoked(_)) => {
            // Mark revoked in DB and invalidate cache
            let _ = store::mark_revoked(&state.pool, account_id).await;
            state.cache.invalidate(account_id);
            Err((
                StatusCode::UNAUTHORIZED,
                Json(ErrorResponse {
                    error: "token_revoked".to_owned(),
                }),
            ))
        }
        Err(GauthError::RateLimited) => Err((
            StatusCode::TOO_MANY_REQUESTS,
            Json(ErrorResponse {
                error: "rate_limit".to_owned(),
            }),
        )),
        Err(e) => {
            tracing::error!("google refresh error for account={account_id}: {e}");
            Err((
                StatusCode::BAD_GATEWAY,
                Json(ErrorResponse {
                    error: "google_error".to_owned(),
                }),
            ))
        }
    }
}

/// GET /status[?account_id=X]
/// Returns token metadata (no token values).
async fn handle_status(
    State(state): State<Arc<AppState>>,
    query: Option<Query<AccountQuery>>,
) -> Result<Json<serde_json::Value>, (StatusCode, Json<ErrorResponse>)> {
    let accounts = store::list_accounts(&state.pool).await.map_err(|e| {
        tracing::error!("list_accounts error: {e}");
        (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(ErrorResponse {
                error: "internal error".to_owned(),
            }),
        )
    })?;

    let filtered: Vec<_> = if let Some(Query(q)) = query {
        accounts
            .into_iter()
            .filter(|a| a.account_id == q.account_id)
            .collect()
    } else {
        accounts
    };

    // Annotate with cache presence
    let response: Vec<serde_json::Value> = filtered
        .into_iter()
        .map(|a| {
            let cached = state.cache.get(&a.account_id).is_some();
            serde_json::json!({
                "account_id": a.account_id,
                "status": a.status,
                "expires_hint": a.expires_hint,
                "cached": cached,
            })
        })
        .collect();

    Ok(Json(serde_json::json!({ "accounts": response })))
}

/// DELETE /token?account_id=X
/// Revokes and removes a stored refresh token.
async fn handle_revoke(
    State(state): State<Arc<AppState>>,
    Query(params): Query<AccountQuery>,
) -> Result<Json<serde_json::Value>, (StatusCode, Json<ErrorResponse>)> {
    store::mark_revoked(&state.pool, &params.account_id)
        .await
        .map_err(|e| {
            tracing::error!("mark_revoked error: {e}");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "internal error".to_owned(),
                }),
            )
        })?;
    state.cache.invalidate(&params.account_id);
    Ok(Json(serde_json::json!({ "revoked": params.account_id })))
}

/// GET /health
async fn handle_health(
    State(state): State<Arc<AppState>>,
) -> (StatusCode, Json<serde_json::Value>) {
    match state.pool.acquire().await {
        Ok(_) => (StatusCode::OK, Json(serde_json::json!({ "status": "ok" }))),
        Err(_) => (
            StatusCode::SERVICE_UNAVAILABLE,
            Json(serde_json::json!({ "status": "unhealthy" })),
        ),
    }
}

fn load_enc_key() -> anyhow::Result<[u8; 32]> {
    let hex_key = env::var("GAUTH_ENCRYPTION_KEY")
        .map_err(|_| anyhow::anyhow!("GAUTH_ENCRYPTION_KEY is required"))?;
    let bytes = hex::decode(&hex_key)
        .map_err(|_| anyhow::anyhow!("GAUTH_ENCRYPTION_KEY must be hex-encoded"))?;
    let arr: [u8; 32] = bytes.try_into().map_err(|_| {
        anyhow::anyhow!("GAUTH_ENCRYPTION_KEY must be exactly 32 bytes (64 hex chars)")
    })?;
    Ok(arr)
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("plugin_gauth=info".parse()?),
        )
        .init();

    let db_url = env::var("NSELF_DB_URL").expect("NSELF_DB_URL is required");
    let enc_key = load_enc_key()?;
    let client_id = env::var("GOOGLE_CLIENT_ID").expect("GOOGLE_CLIENT_ID is required");
    let client_secret = env::var("GOOGLE_CLIENT_SECRET").expect("GOOGLE_CLIENT_SECRET is required");
    let port: u16 = env::var("GAUTH_PORT")
        .unwrap_or_else(|_| "3762".to_owned())
        .parse()?;
    let google_token_url =
        env::var("GOOGLE_TOKEN_URL").unwrap_or_else(|_| "https://oauth2.googleapis.com".to_owned());

    let pool = sqlx::postgres::PgPoolOptions::new()
        .max_connections(5)
        .acquire_timeout(Duration::from_secs(5))
        .connect(&db_url)
        .await?;

    let state = Arc::new(AppState {
        pool,
        cache: TokenCache::new(),
        enc_key,
        http_client: reqwest::Client::new(),
        google_token_url,
        client_id,
        client_secret,
    });

    let app = Router::new()
        .route("/health", get(handle_health))
        .route("/refresh", post(handle_refresh))
        .route("/status", get(handle_status))
        .route("/token", delete(handle_revoke))
        .with_state(state);

    let addr = format!("0.0.0.0:{port}");
    tracing::info!("plugin-gauth listening on {addr}");
    let listener = tokio::net::TcpListener::bind(&addr).await?;
    axum::serve(listener, app).await?;
    Ok(())
}
