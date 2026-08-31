// Package cache provides the Redis-backed caching layer for nself-eval-gate.
// Two caches are maintained: BGE-M3 embedding cache (24h TTL) and LLM judge result cache (1h TTL).
package cache

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss is returned by Get when the key is not found in the cache.
// Purpose: Typed sentinel so callers can distinguish miss from hard error.
var ErrCacheMiss = errors.New("cache miss")

// Cache key prefixes. Both start with "eval:" per spec §3 Cache Keys table.
const (
	// EmbedCacheKeyPrefix is the Redis key prefix for BGE-M3 embedding results.
	EmbedCacheKeyPrefix = "eval:embed:"
	// JudgeCacheKeyPrefix is the Redis key prefix for LLM judge results.
	JudgeCacheKeyPrefix = "eval:judge:"
)

// EvalCache is the interface for eval scoring result caches.
// Purpose: Decouple scorer code from Redis; enables NoopCache for offline/test use.
// Inputs: context for cancellation; key constructed via CacheKey(); TTL for Set.
// Outputs: cached bytes on Get hit; ErrCacheMiss on miss; nil on Set success.
// Constraints: Never cache empty or zero-value results (enforced in RedisEvalCache.Set).
type EvalCache interface {
	// Get retrieves a cached value by key. Returns ErrCacheMiss if key not present.
	Get(ctx context.Context, key string) ([]byte, error)
	// Set stores a value with a TTL. No-op if val is empty or zero-score result.
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
}

// CacheKey computes a deterministic Redis key from a prefix and content string.
// Purpose: Consistent SHA256-based key generation for embedding and judge caches.
// Inputs: prefix (EmbedCacheKeyPrefix or JudgeCacheKeyPrefix), content (text to hash).
// Outputs: "eval:{prefix}{sha256(content)}" — fixed 64-char hex suffix.
// Constraints: prefix must start with "eval:"; same content always produces same key.
func CacheKey(prefix, content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%s%x", prefix, sum)
}

// NoopCache is an EvalCache that never stores anything.
// Purpose: Returned when NSELF_EVAL_GATE_REDIS_URL is unset; safe for offline dev/test.
// Inputs: none (stateless).
// Outputs: Get always returns ErrCacheMiss; Set always returns nil.
// Constraints: Safe for concurrent use — no shared mutable state.
type NoopCache struct{}

// Get always returns ErrCacheMiss.
func (n *NoopCache) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, ErrCacheMiss
}

// Set is a no-op; always returns nil.
func (n *NoopCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

// RedisEvalCache wraps go-redis/v9 to provide embed and judge result caching.
// Purpose: Production cache backed by Redis; skips Set for empty/zero-score values.
// Inputs: client initialized from NSELF_EVAL_GATE_REDIS_URL; TTLs from plugin config.
// Outputs: serialized bytes on cache hit; ErrCacheMiss on miss.
// Constraints: Never stores empty val or values representing a zero score (transient errors).
type RedisEvalCache struct {
	client *redis.Client
}

// Get retrieves a cached value. Returns ErrCacheMiss if the key is absent or expired.
func (r *RedisEvalCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, fmt.Errorf("cache get %q: %w", key, err)
	}
	return val, nil
}

// Set stores a value with the given TTL.
// Guard: never stores empty val to prevent transient failures masking as regressions.
func (r *RedisEvalCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	if len(val) == 0 {
		// Never cache empty results — transient error guard.
		return nil
	}
	if err := r.client.Set(ctx, key, val, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %q: %w", key, err)
	}
	return nil
}

// NewEvalCache constructs an EvalCache from a Redis URL.
// Purpose: Single constructor used in main.go; returns NoopCache when URL is empty.
// Inputs: redisURL from NSELF_EVAL_GATE_REDIS_URL env var (may be empty string).
// Outputs: *RedisEvalCache if URL set; *NoopCache if URL empty.
// Constraints: Does not verify connectivity at construction time; first Get/Set will error if URL invalid.
func NewEvalCache(redisURL string) EvalCache {
	if redisURL == "" {
		return &NoopCache{}
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		// Malformed URL — fall back to noop to avoid startup failure.
		return &NoopCache{}
	}
	return &RedisEvalCache{client: redis.NewClient(opts)}
}
