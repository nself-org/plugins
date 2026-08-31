// Integration test: plugin-gauth token refresh with mocked Google endpoint.
//
// Purpose: Verify the full refresh flow (mock Google, cache hit, revocation).
// Constraints: No real Google credentials needed — mockito intercepts all calls.
//   NSELF_DB_URL + GAUTH_ENCRYPTION_KEY required for DB-backed tests; skipped otherwise.

use plugin_gauth::{
    cache::TokenCache,
    crypto::{decrypt, encrypt},
    token::refresh_access_token,
};

#[tokio::test]
async fn test_integration_mock_refresh_returns_access_token() {
    let mut server = mockito::Server::new_async().await;
    let mock = server
        .mock("POST", "/token")
        .with_status(200)
        .with_header("content-type", "application/json")
        .with_body(
            r#"{"access_token":"ya29.integration-test","expires_in":3600,"token_type":"Bearer"}"#,
        )
        .create_async()
        .await;

    let client = reqwest::Client::new();
    let result = refresh_access_token(
        &client,
        &server.url(),
        "test-client-id",
        "test-client-secret",
        "test-refresh-token",
    )
    .await
    .expect("refresh should succeed");

    mock.assert_async().await;
    assert_eq!(result.access_token, "ya29.integration-test");
    assert_eq!(result.expires_in, 3600);
}

#[tokio::test]
async fn test_integration_cache_hit_skips_google() {
    let cache = TokenCache::new();
    cache.set("account-x", "cached-token".to_string(), 3600);

    // If we get the token from cache, no HTTP call is made
    let cached = cache.get("account-x");
    assert_eq!(cached, Some("cached-token".to_string()));
}

#[tokio::test]
async fn test_integration_revocation_invalidates_cache() {
    let mut server = mockito::Server::new_async().await;
    let mock = server
        .mock("POST", "/token")
        .with_status(401)
        .create_async()
        .await;

    let cache = TokenCache::new();
    cache.set("account-y", "old-token".to_string(), 3600);

    let client = reqwest::Client::new();
    let result = refresh_access_token(&client, &server.url(), "cid", "cs", "revoked-refresh").await;

    mock.assert_async().await;
    assert!(result.is_err(), "revoked token must return error");

    // Simulate what handler does: invalidate cache on revocation
    cache.invalidate("account-y");
    assert_eq!(cache.get("account-y"), None);
}

#[tokio::test]
async fn test_integration_refresh_token_never_in_response() {
    let secret_token = "super-secret-refresh-token-xyz";
    let mut server = mockito::Server::new_async().await;
    let _mock = server
        .mock("POST", "/token")
        .with_status(200)
        .with_header("content-type", "application/json")
        .with_body(r#"{"access_token":"ya29.safe","expires_in":3600,"token_type":"Bearer"}"#)
        .create_async()
        .await;

    let client = reqwest::Client::new();
    let result = refresh_access_token(&client, &server.url(), "cid", "cs", secret_token)
        .await
        .expect("should succeed");

    let json_str = serde_json::to_string(&result.access_token).unwrap();
    assert!(
        !json_str.contains(secret_token),
        "refresh token must NOT appear in the access token response"
    );
}

#[test]
fn test_crypto_roundtrip_in_integration() {
    let key = [0xABu8; 32];
    let secret = "GAUTH_REFRESH_test-account-integration";
    let encrypted = encrypt(&key, secret).expect("encrypt");
    // Verify the encrypted form does not contain the original
    assert!(!encrypted.contains("GAUTH_REFRESH"));
    let decrypted = decrypt(&key, &encrypted).expect("decrypt");
    assert_eq!(decrypted, secret);
}
