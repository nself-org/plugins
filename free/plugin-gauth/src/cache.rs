// Package cache: in-memory access token cache with TTL.
//
// Purpose: Cache Google access tokens to avoid redundant refresh calls.
//   Entries expire after the TTL returned by Google (expires_in seconds).
// Inputs: account_id, access_token, expires_in (from Google token response).
// Outputs: Option<String> — Some(access_token) if still valid, None if expired/missing.
// Constraints: Thread-safe via DashMap. Never cache refresh tokens.

use dashmap::DashMap;
use std::time::{Duration, Instant};

struct CacheEntry {
    access_token: String,
    expires_at: Instant,
}

/// TokenCache holds in-memory access tokens keyed by account_id.
pub struct TokenCache {
    inner: DashMap<String, CacheEntry>,
}

impl TokenCache {
    pub fn new() -> Self {
        Self {
            inner: DashMap::new(),
        }
    }

    /// Insert or update a cached access token.
    pub fn set(&self, account_id: &str, access_token: String, expires_in_secs: u64) {
        let expires_at = Instant::now() + Duration::from_secs(expires_in_secs.saturating_sub(30));
        self.inner.insert(
            account_id.to_owned(),
            CacheEntry {
                access_token,
                expires_at,
            },
        );
    }

    /// Retrieve a valid cached access token, or None if expired/missing.
    pub fn get(&self, account_id: &str) -> Option<String> {
        let entry = self.inner.get(account_id)?;
        if Instant::now() < entry.expires_at {
            Some(entry.access_token.clone())
        } else {
            drop(entry);
            self.inner.remove(account_id);
            None
        }
    }

    /// Invalidate a cached token (e.g. after revocation).
    pub fn invalidate(&self, account_id: &str) {
        self.inner.remove(account_id);
    }
}

impl Default for TokenCache {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_cache_hit_within_ttl() {
        let cache = TokenCache::new();
        cache.set("user1", "tok-abc".to_string(), 3600);
        assert_eq!(cache.get("user1"), Some("tok-abc".to_string()));
    }

    #[test]
    fn test_cache_miss_unknown_account() {
        let cache = TokenCache::new();
        assert_eq!(cache.get("unknown"), None);
    }

    #[test]
    fn test_cache_invalidate() {
        let cache = TokenCache::new();
        cache.set("user2", "tok-xyz".to_string(), 3600);
        cache.invalidate("user2");
        assert_eq!(cache.get("user2"), None);
    }

    #[test]
    fn test_cache_expired_entry_evicted() {
        let cache = TokenCache::new();
        // TTL 0 means expires_at = now - 30s (saturating_sub) which is already past
        cache.set("user3", "tok-old".to_string(), 0);
        assert_eq!(cache.get("user3"), None);
    }
}
