// Package ratelimit provides a simple per-key token bucket rate limiter.
package ratelimit

import (
	"sync"
	"time"
)

// Bucket is a token bucket for a single key.
type Bucket struct {
	tokens    float64
	capacity  float64
	rate      float64 // tokens per second
	lastRefill time.Time
	mu        sync.Mutex
}

func newBucket(capacity, ratePerSec float64) *Bucket {
	return &Bucket{
		tokens:    capacity,
		capacity:  capacity,
		rate:      ratePerSec,
		lastRefill: time.Now(),
	}
}

// Allow returns true if a token is available, consuming one on success.
func (b *Bucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Limiter manages per-key token buckets.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*Bucket
	capacity float64
	rate     float64
}

// New creates a Limiter where each key allows `maxPerMin` requests per minute.
func New(maxPerMin int) *Limiter {
	capacity := float64(maxPerMin)
	rate := float64(maxPerMin) / 60.0
	return &Limiter{
		buckets:  make(map[string]*Bucket),
		capacity: capacity,
		rate:     rate,
	}
}

// Allow returns true if the given key is within its rate limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	b, ok := l.buckets[key]
	if !ok {
		b = newBucket(l.capacity, l.rate)
		l.buckets[key] = b
	}
	l.mu.Unlock()
	return b.Allow()
}
