// Package ratelimit provides per-tenant rate limiting for the /provision endpoint.
// Redis is used when REDIS_URL is set; otherwise an in-memory store is used.
// Limit: 5 provisions per hour per tenant.
package ratelimit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrRateLimitExceeded is returned when the tenant has exceeded the provision rate limit.
var ErrRateLimitExceeded = errors.New("rate limit exceeded: max 5 provisions per hour per tenant")

const (
	provisionLimit = 5
	windowDuration = time.Hour
)

// ProvisionLimiter enforces per-tenant provision rate limits.
type ProvisionLimiter interface {
	// Allow returns nil if the tenant is permitted to provision, or ErrRateLimitExceeded.
	Allow(ctx context.Context, tenantID string) error
}

// New returns a ProvisionLimiter backed by Redis if REDIS_URL is set,
// otherwise falls back to an in-memory store.
func New() ProvisionLimiter {
	if u := os.Getenv("REDIS_URL"); u != "" {
		return newRedisLimiter(u)
	}
	return newMemoryLimiter()
}

// ─── In-memory implementation ────────────────────────────────────────────────

type windowEntry struct {
	count     int
	windowEnd time.Time
}

type memoryLimiter struct {
	mu      sync.Mutex
	windows map[string]*windowEntry
}

func newMemoryLimiter() *memoryLimiter {
	return &memoryLimiter{windows: make(map[string]*windowEntry)}
}

func (m *memoryLimiter) Allow(_ context.Context, tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	entry, ok := m.windows[tenantID]
	if !ok || now.After(entry.windowEnd) {
		m.windows[tenantID] = &windowEntry{count: 1, windowEnd: now.Add(windowDuration)}
		return nil
	}
	if entry.count >= provisionLimit {
		return ErrRateLimitExceeded
	}
	entry.count++
	return nil
}

// ─── Redis implementation ────────────────────────────────────────────────────

// redisLimiter uses Redis INCR + EXPIRE NX for fixed-window rate limiting.
// Key format: nself:provision_rl:{tenantID}:{YYYYMMDDHR}
type redisLimiter struct {
	url string
}

func newRedisLimiter(rawURL string) *redisLimiter {
	return &redisLimiter{url: rawURL}
}

func (r *redisLimiter) Allow(ctx context.Context, tenantID string) error {
	hour := time.Now().UTC().Format("2006010215")
	key := fmt.Sprintf("nself:provision_rl:%s:%s", tenantID, hour)

	count, err := redisIncrEx(ctx, r.url, key, windowDuration)
	if err != nil {
		// Fail open: Redis unavailable does not block provisioning.
		fmt.Fprintf(os.Stderr, "nself-cloud ratelimit: redis error (fail-open): %v\n", err)
		return nil
	}
	if count > int64(provisionLimit) {
		return ErrRateLimitExceeded
	}
	return nil
}

// redisIncrEx dials Redis, sends INCR <key>, then EXPIRE <key> <ttl> NX,
// and returns the new counter value. This is a minimal RESP implementation
// that avoids adding a Redis client dependency.
// NOTE: INCR + EXPIRE is not atomic. For strict accuracy use a Lua script or
// Redis 7+ GETDEL/SETEX; for rate limiting the occasional off-by-one on crash
// is acceptable.
func redisIncrEx(ctx context.Context, rawURL, key string, ttl time.Duration) (int64, error) {
	conn, err := dialRedis(ctx, rawURL)
	if err != nil {
		return 0, fmt.Errorf("redis dial: %w", err)
	}
	defer conn.close()

	// INCR
	if err := conn.writeCmd("INCR", key); err != nil {
		return 0, err
	}
	count, err := conn.readInt()
	if err != nil {
		return 0, err
	}

	// EXPIRE ... NX — set TTL only on first increment so the window is fixed.
	ttlSec := strconv.FormatInt(int64(ttl.Seconds()), 10)
	if err := conn.writeCmd("EXPIRE", key, ttlSec, "NX"); err != nil {
		return 0, err
	}
	if _, err := conn.readInt(); err != nil {
		// Non-fatal — EXPIRE NX unsupported on very old Redis falls back to unconditional expire.
		_ = conn.writeCmd("EXPIRE", key, ttlSec)
		_, _ = conn.readInt()
	}

	return count, nil
}

// ─── Minimal RESP client ──────────────────────────────────────────────────────

type redisConn struct {
	raw net.Conn
	rw  *bufio.ReadWriter
}

func dialRedis(ctx context.Context, rawURL string) (*redisConn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL %q: %w", rawURL, err)
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":6379"
	}
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	rc := &redisConn{
		raw: raw,
		rw:  bufio.NewReadWriter(bufio.NewReader(raw), bufio.NewWriter(raw)),
	}
	if u.User != nil {
		if pw, ok := u.User.Password(); ok {
			if err := rc.writeCmd("AUTH", pw); err != nil {
				raw.Close()
				return nil, err
			}
			_, _ = rc.readInt() // "+OK" or ":1"
		}
	}
	return rc, nil
}

func (rc *redisConn) close() error { return rc.raw.Close() }

func (rc *redisConn) writeCmd(args ...string) error {
	fmt.Fprintf(rc.rw, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(rc.rw, "$%d\r\n%s\r\n", len(a), a)
	}
	return rc.rw.Flush()
}

func (rc *redisConn) readInt() (int64, error) {
	line, err := rc.rw.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 {
		return 0, fmt.Errorf("empty redis reply")
	}
	switch line[0] {
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '+':
		return 1, nil // simple string ("+OK") counts as 1 for EXPIRE reply
	case '-':
		return 0, fmt.Errorf("redis error: %s", line[1:])
	default:
		return 0, fmt.Errorf("unexpected redis reply type %q", string(line[0]))
	}
}
