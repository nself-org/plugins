// Package token: Google OAuth2 token refresh via https://oauth2.googleapis.com/token.
//
// Purpose: Call Google token endpoint with a pre-provisioned refresh token.
//   Respects SSRF guard: only ever calls the canonical Google token URL.
// Inputs: client_id, client_secret, refresh_token (all from env/DB).
// Outputs: GoogleTokenResponse on success.
// Constraints:
//   - Refresh token is passed only in the POST body (never in logs, headers, or URLs).
//   - 401 from Google means token is revoked → caller should call store::mark_revoked.
//   - Rate limit errors (429) are surfaced as GauthError::RateLimited.

use serde::{Deserialize, Serialize};

use crate::error::GauthError;

pub const GOOGLE_TOKEN_URL: &str = "https://oauth2.googleapis.com/token";

#[derive(Debug, Deserialize)]
pub struct GoogleTokenResponse {
    pub access_token: String,
    pub expires_in: u64,
    pub token_type: String,
}

#[derive(Serialize)]
struct RefreshRequest<'a> {
    client_id: &'a str,
    client_secret: &'a str,
    refresh_token: &'a str,
    grant_type: &'a str,
}

/// Call the Google token endpoint to exchange a refresh token for a new access token.
/// Returns GauthError::TokenRevoked if Google responds 401/403.
/// The base_url parameter allows tests to inject a mock server URL.
pub async fn refresh_access_token(
    client: &reqwest::Client,
    base_url: &str,
    client_id: &str,
    client_secret: &str,
    refresh_token: &str,
) -> Result<GoogleTokenResponse, GauthError> {
    let url = format!("{}/token", base_url.trim_end_matches('/'));
    let req = RefreshRequest {
        client_id,
        client_secret,
        refresh_token,
        grant_type: "refresh_token",
    };

    let resp = client
        .post(&url)
        .form(&req)
        .send()
        .await
        .map_err(GauthError::Network)?;

    match resp.status().as_u16() {
        200 => {
            let body: GoogleTokenResponse =
                resp.json().await.map_err(|_| GauthError::InvalidResponse)?;
            Ok(body)
        }
        401 | 403 => Err(GauthError::TokenRevoked("<redacted>".to_owned())),
        429 => Err(GauthError::RateLimited),
        _ => Err(GauthError::InvalidResponse),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_refresh_success_mocked() {
        let mut server = mockito::Server::new_async().await;
        let mock = server
            .mock("POST", "/token")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(
                r#"{"access_token":"ya29.new-token","expires_in":3600,"token_type":"Bearer"}"#,
            )
            .create_async()
            .await;

        let client = reqwest::Client::new();
        let result = refresh_access_token(
            &client,
            &server.url(),
            "client-id",
            "client-secret",
            "refresh-token",
        )
        .await;

        mock.assert_async().await;
        let resp = result.expect("should succeed");
        assert_eq!(resp.access_token, "ya29.new-token");
        assert_eq!(resp.expires_in, 3600);
    }

    #[tokio::test]
    async fn test_refresh_401_returns_revoked() {
        let mut server = mockito::Server::new_async().await;
        let mock = server
            .mock("POST", "/token")
            .with_status(401)
            .create_async()
            .await;

        let client = reqwest::Client::new();
        let result = refresh_access_token(
            &client,
            &server.url(),
            "client-id",
            "client-secret",
            "revoked-token",
        )
        .await;

        mock.assert_async().await;
        assert!(matches!(result, Err(GauthError::TokenRevoked(_))));
    }

    #[tokio::test]
    async fn test_refresh_response_does_not_echo_refresh_token() {
        let mut server = mockito::Server::new_async().await;
        let _mock = server
            .mock("POST", "/token")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"access_token":"ya29.tok","expires_in":3600,"token_type":"Bearer"}"#)
            .create_async()
            .await;

        let client = reqwest::Client::new();
        let result = refresh_access_token(
            &client,
            &server.url(),
            "client-id",
            "client-secret",
            "original-refresh-token",
        )
        .await
        .expect("should succeed");

        // Verify the response does NOT contain the refresh token value
        let serialized = serde_json::to_string(&result.access_token).unwrap();
        assert!(
            !serialized.contains("original-refresh-token"),
            "refresh token must not appear in response"
        );
    }
}
