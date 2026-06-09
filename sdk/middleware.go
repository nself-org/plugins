package sdk

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// CORS adds permissive CORS headers (Allow-Origin: *, standard methods and headers).
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequestID adds an X-Request-ID header to each request using chi's
// built-in request ID middleware.
func RequestID(next http.Handler) http.Handler {
	return middleware.RequestID(next)
}

// Logger logs each request's method, path, status, and duration.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		log.Printf("%s %s %d %s",
			r.Method,
			r.URL.Path,
			ww.Status(),
			time.Since(start).Round(time.Millisecond),
		)
	})
}

// Recovery catches panics in downstream handlers and returns a 500 response.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("plugin-sdk: panic recovered: %v\n%s", rec, debug.Stack())
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// ipBucket tracks token-bucket state for a single IP address.
type ipBucket struct {
	mu       sync.Mutex
	tokens   float64
	lastSeen time.Time
}

// RateLimiter is an in-process per-IP token-bucket rate limiter.
// It acts as a backstop behind the nginx rate-limit layer, ensuring
// individual plugin services enforce limits even when nginx is bypassed
// during local development or direct container access.
//
// Create one via NewRateLimiter and attach it as middleware via Middleware.
// The zero value is not usable; always use NewRateLimiter.
type RateLimiter struct {
	rpm    float64  // allowed requests per minute
	period time.Duration // token refill interval (always 1 minute)
	state  sync.Map // map[string]*ipBucket keyed by remote IP
}

// NewRateLimiter returns a RateLimiter that allows at most requestsPerMinute
// requests per unique client IP within any sliding minute window.
// requestsPerMinute must be greater than zero.
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	if requestsPerMinute <= 0 {
		requestsPerMinute = 60
	}
	return &RateLimiter{
		rpm:    float64(requestsPerMinute),
		period: time.Minute,
	}
}

// allow reports whether the request from ip should be permitted.
// It uses a token-bucket algorithm: tokens accumulate at rpm/minute up to
// a burst capacity equal to the full per-minute allowance.
func (rl *RateLimiter) allow(ip string) bool {
	now := time.Now()

	v, _ := rl.state.LoadOrStore(ip, &ipBucket{
		tokens:   rl.rpm,
		lastSeen: now,
	})
	b := v.(*ipBucket)

	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := now.Sub(b.lastSeen)
	// Refill tokens proportionally to elapsed time, capped at burst (= rpm).
	b.tokens += elapsed.Minutes() * rl.rpm
	if b.tokens > rl.rpm {
		b.tokens = rl.rpm
	}
	b.lastSeen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Middleware returns an http.Handler middleware that enforces the rate limit.
// When the limit is exceeded the handler writes HTTP 429 with a Retry-After
// header indicating how many seconds until the next token is available.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	// retryAfterSec is the number of seconds until one token refills.
	retryAfterSec := int(rl.period.Seconds() / rl.rpm)
	if retryAfterSec < 1 {
		retryAfterSec = 1
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := remoteIP(r)
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSec))
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimit returns middleware that limits requests to requestsPerMinute per
// client IP. It is a convenience wrapper around NewRateLimiter for inline use:
//
//	mux.Use(sdk.RateLimit(60))
func RateLimit(requestsPerMinute int) func(http.Handler) http.Handler {
	return NewRateLimiter(requestsPerMinute).Middleware
}

// remoteIP extracts the client IP from the request, preferring
// X-Forwarded-For when set by a trusted proxy (nginx in the nself stack).
func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (leftmost) address which is the original client.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	// Fall back to RemoteAddr, stripping the port.
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
