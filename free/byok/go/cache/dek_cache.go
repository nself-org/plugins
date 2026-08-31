// Package cache provides an optional in-memory DEK cache for the BYOK plugin.
// Caching reduces KMS API calls at the cost of DEKs living in process memory
// longer than a single request. Enabled via NSELF_BYOK_DEK_CACHE=true (Enterprise).
//
// Security properties:
//   - Cache entries expire after TTL (default 300s, configurable).
//   - Cache is invalidated on a KMS UnwrapKey error (revoked-key path).
//   - DEKs are zeroed before eviction.
//   - The cache is keyed by (tenantID, encryptedDEK hash) so different wrapped
//     blobs for the same tenant do not collide.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// entry holds a plaintext DEK and its expiry.
type entry struct {
	dek       []byte
	expiresAt time.Time
}

// zero overwrites the DEK bytes.
func (e *entry) zero() {
	for i := range e.dek {
		e.dek[i] = 0
	}
}

// DEKCache is a bounded, TTL-expiring, in-memory cache for plaintext DEKs.
// It is safe for concurrent use.
type DEKCache struct {
	mu      sync.Mutex
	entries map[string]*entry
	ttl     time.Duration
}

// NewDEKCache creates a DEKCache with the given TTL. Pass 0 to use the default
// of 5 minutes. A nil cache pointer is a valid no-op (cache disabled path).
func NewDEKCache(ttl time.Duration) *DEKCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &DEKCache{
		entries: make(map[string]*entry),
		ttl:     ttl,
	}
}

// cacheKey returns a stable key derived from the encrypted DEK bytes.
// Using a hash means we never store the wrapped DEK bytes as the map key.
func cacheKey(wrappedDEK []byte) string {
	sum := sha256.Sum256(wrappedDEK)
	return hex.EncodeToString(sum[:])
}

// Get retrieves a cached DEK. Returns (nil, false) on miss or expiry.
func (c *DEKCache) Get(wrappedDEK []byte) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(wrappedDEK)
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		e.zero()
		delete(c.entries, key)
		return nil, false
	}
	// Return a copy so the caller cannot corrupt the cached slice.
	out := make([]byte, len(e.dek))
	copy(out, e.dek)
	return out, true
}

// Put stores a plaintext DEK in the cache under the wrappedDEK key.
// The cache owns a copy of the plaintext DEK and zeros it on eviction.
func (c *DEKCache) Put(wrappedDEK, plainDEK []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(wrappedDEK)
	dek := make([]byte, len(plainDEK))
	copy(dek, plainDEK)
	c.entries[key] = &entry{
		dek:       dek,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes a cache entry and zeros the DEK. Call this when the
// KMS provider returns a permission error (revoked key) to prevent stale
// DEK usage. No-op on miss.
func (c *DEKCache) Invalidate(wrappedDEK []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(wrappedDEK)
	if e, ok := c.entries[key]; ok {
		e.zero()
		delete(c.entries, key)
	}
}

// Flush clears and zeros all cached DEKs. Call on shutdown or tenant removal.
func (c *DEKCache) Flush() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		e.zero()
		delete(c.entries, k)
	}
}

// Size returns the number of cached entries (for metrics).
func (c *DEKCache) Size() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
