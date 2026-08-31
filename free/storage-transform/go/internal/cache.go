package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache handles Redis LRU + on-disk fallback for transformed image variants.
type Cache struct {
	rdb     *redis.Client
	ttl     time.Duration
	diskDir string
}

// Redis stat keys.
const (
	keyHits      = "st:cache:hits"
	keyMisses    = "st:cache:misses"
	keyEvictions = "st:cache:evictions"
)

// NewCache connects to Redis and prepares the disk cache directory.
// Falls back to disk-only if Redis is unavailable.
func NewCache(redisURL, diskDir string, ttlDays int) *Cache {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("cache: redis URL parse error (%v) — disk-only mode", err)
		return &Cache{diskDir: diskDir, ttl: time.Duration(ttlDays) * 24 * time.Hour}
	}
	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("cache: redis unavailable (%v) — disk-only mode", err)
		rdb.Close()
		return &Cache{diskDir: diskDir, ttl: time.Duration(ttlDays) * 24 * time.Hour}
	}
	if err := os.MkdirAll(diskDir, 0o755); err != nil {
		log.Printf("cache: cannot create disk cache dir %s: %v", diskDir, err)
	}
	return &Cache{
		rdb:     rdb,
		ttl:     time.Duration(ttlDays) * 24 * time.Hour,
		diskDir: diskDir,
	}
}

// Get returns cached bytes for key, or (nil, false) on miss.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	// Try Redis first.
	if c.rdb != nil {
		b, err := c.rdb.Get(ctx, redisKey(key)).Bytes()
		if err == nil {
			c.incr(ctx, keyHits)
			return b, true
		}
		if !errors.Is(err, redis.Nil) {
			log.Printf("cache: redis GET error: %v", err)
		}
	}

	// Fall back to disk.
	path := c.diskPath(key)
	b, err := os.ReadFile(path)
	if err == nil {
		c.incr(ctx, keyHits)
		return b, true
	}

	c.incr(ctx, keyMisses)
	return nil, false
}

// Set stores bytes in Redis (with TTL) and on disk.
func (c *Cache) Set(ctx context.Context, key string, data []byte) {
	if c.rdb != nil {
		if err := c.rdb.Set(ctx, redisKey(key), data, c.ttl).Err(); err != nil {
			log.Printf("cache: redis SET error: %v", err)
		}
	}

	path := c.diskPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			log.Printf("cache: disk write error: %v", err)
		}
	}
}

// Stats returns current hit/miss/eviction counters from Redis.
func (c *Cache) Stats(ctx context.Context) map[string]int64 {
	stats := map[string]int64{"hits": 0, "misses": 0, "evictions": 0}
	if c.rdb == nil {
		return stats
	}
	for label, rk := range map[string]string{
		"hits":      keyHits,
		"misses":    keyMisses,
		"evictions": keyEvictions,
	} {
		v, err := c.rdb.Get(ctx, rk).Int64()
		if err == nil {
			stats[label] = v
		}
	}
	return stats
}

// Purge deletes all cache entries whose key starts with prefix.
// Pass "" to purge all.
func (c *Cache) Purge(ctx context.Context, prefix string) error {
	pattern := "st:img:" + prefix + "*"
	if c.rdb != nil {
		iter := c.rdb.Scan(ctx, 0, pattern, 0).Iterator()
		for iter.Next(ctx) {
			if err := c.rdb.Del(ctx, iter.Val()).Err(); err != nil {
				log.Printf("cache: purge del error: %v", err)
			}
		}
		if err := iter.Err(); err != nil {
			return fmt.Errorf("cache: purge scan: %w", err)
		}
		// Reset stats.
		c.rdb.Set(ctx, keyHits, 0, 0)
		c.rdb.Set(ctx, keyMisses, 0, 0)
		c.rdb.Set(ctx, keyEvictions, 0, 0)
	}
	return nil
}

// Close releases the Redis connection.
func (c *Cache) Close() {
	if c.rdb != nil {
		c.rdb.Close()
	}
}

// incr increments a Redis counter without blocking.
func (c *Cache) incr(ctx context.Context, key string) {
	if c.rdb == nil {
		return
	}
	c.rdb.Incr(ctx, key)
}

// redisKey namespaces the cache key.
func redisKey(key string) string {
	return "st:img:" + key
}

// diskPath converts a cache key to an on-disk file path, using a SHA256 prefix
// as the directory shard to avoid too many files in one directory.
func (c *Cache) diskPath(key string) string {
	h := sha256.Sum256([]byte(key))
	hex := hex.EncodeToString(h[:])
	// Sanitise key to avoid path traversal.
	safe := strings.NewReplacer("/", "_", ":", "_", "?", "_", "&", "_").Replace(key)
	if len(safe) > 200 {
		safe = safe[:200]
	}
	return filepath.Join(c.diskDir, hex[:2], hex[2:4], safe)
}
