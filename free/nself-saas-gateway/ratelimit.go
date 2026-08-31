package main

// ratelimit.go — fixed-window per-IP rate limiter for the unauthenticated
// auth endpoints (/v1/signup, /v1/login).
//
// Purpose: defence-in-depth against credential stuffing / signup floods.
//   The fronting nginx also rate-limits (W2 box config); this keeps the
//   gateway safe when reached directly on loopback or via a misconfigured
//   proxy. In-memory only — a single-box deployment (PRD §4) needs no
//   distributed state.
// Constraints: chi middleware.RealIP runs first, so r.RemoteAddr is the
//   client IP behind nginx. Windows are pruned lazily; memory is bounded
//   by (unique IPs in the current window).

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter counts requests per key in fixed windows.
type rateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	seen   map[string]*rateWindow
}

type rateWindow struct {
	start time.Time
	count int
}

// newRateLimiter returns a limiter allowing `limit` requests per `window`.
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{window: window, limit: limit, seen: map[string]*rateWindow{}}
}

// allow records one request for key and reports whether it is within limit.
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Lazy prune: drop expired windows so the map stays bounded.
	if len(l.seen) > 4096 {
		for k, w := range l.seen {
			if now.Sub(w.start) >= l.window {
				delete(l.seen, k)
			}
		}
	}

	w, ok := l.seen[key]
	if !ok || now.Sub(w.start) >= l.window {
		l.seen[key] = &rateWindow{start: now, count: 1}
		return true
	}
	w.count++
	return w.count <= l.limit
}

// clientIP extracts the client IP (RealIP middleware already resolved
// X-Forwarded-For into RemoteAddr).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimited wraps a handler with the per-IP limiter, replying 429 with
// the contract envelope when exceeded.
func rateLimited(l *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r), time.Now()) {
			writeErr(w, http.StatusTooManyRequests, "rate_limited",
				"too many requests — try again shortly")
			return
		}
		next(w, r)
	}
}
