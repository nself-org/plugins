// storage-transform is the nSelf image transformation CDN service.
// It intercepts storage URL requests with transform query params (?w, ?h, ?q, ?fmt, ?fit, ?dpr),
// applies govips transforms, caches results in Redis + disk, and streams back with aggressive
// Cache-Control headers so Nginx (proxy_cache) can cache at the edge.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nself-org/nself-storage-transform/internal"
)

func main() {
	cfg := internal.LoadConfig()

	transformer := internal.NewTransformer()
	defer transformer.Shutdown()

	cache := internal.NewCache(cfg.RedisURL, cfg.DiskCacheDir, cfg.CacheTTLDays)
	defer cache.Close()

	rl := newRateLimiter(cfg.RateLimit)

	srv := &server{
		cfg:         cfg,
		transformer: transformer,
		cache:       cache,
		rl:          rl,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// Health check — no auth required.
	r.Get("/health", srv.handleHealth)

	// Management API — admin endpoints.
	r.Get("/api/storage-transform/cache/stats", srv.handleCacheStats)
	r.Post("/api/storage-transform/cache/purge", srv.handleCachePurge)
	r.Get("/api/storage-transform/health", srv.handleHealth)

	// Image transform endpoint — matches the storage URL pattern the spec forwards here.
	// Nginx rewrites /storage/v1/object/public/** to this service.
	r.Get("/storage/v1/object/public/*", srv.handleTransform)
	r.Get("/transform/*", srv.handleTransform)

	addr := "127.0.0.1:" + cfg.Port
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	go func() {
		log.Printf("storage-transform: listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("storage-transform: server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("storage-transform: shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		log.Printf("storage-transform: shutdown error: %v", err)
	}
}

// server holds shared service state.
type server struct {
	cfg         internal.Config
	transformer *internal.Transformer
	cache       *internal.Cache
	rl          *rateLimiter
	httpClient  *http.Client
}

// handleTransform is the main image transform handler.
func (s *server) handleTransform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Rate limiting by IP.
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	if !s.rl.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded: 100 requests/min")
		return
	}

	// Validate HMAC signature if a signing secret is configured.
	if s.cfg.SigningSecret != "" {
		if !verifyHMAC(r, s.cfg.SigningSecret) {
			writeError(w, http.StatusForbidden, "invalid or missing signature")
			return
		}
	}

	// Parse and validate transform params.
	params, err := internal.ParseParams(r, s.cfg.MaxWidth, s.cfg.MaxHeight)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Extract the image path from the URL (everything after the route prefix).
	imagePath := chi.URLParam(r, "*")
	if imagePath == "" {
		writeError(w, http.StatusBadRequest, "image path required")
		return
	}

	// Check cache.
	cacheKey := params.CacheKey(imagePath)
	if cached, ok := s.cache.Get(ctx, cacheKey); ok {
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
		w.Header().Set("Content-Type", mimeForParams(params))
		w.WriteHeader(http.StatusOK)
		w.Write(cached) //nolint:errcheck
		return
	}

	// Fetch source image from storage plugin.
	srcURL := strings.TrimRight(s.cfg.SourceStorageURL, "/") + "/storage/v1/object/public/" + imagePath
	srcBytes, err := s.fetchSource(ctx, srcURL)
	if err != nil {
		log.Printf("storage-transform: fetch error for %s: %v", srcURL, err)
		writeError(w, http.StatusBadGateway, "failed to fetch source image")
		return
	}

	// Run transform.
	result, mime, err := s.transformer.Transform(srcBytes, params)
	if err != nil {
		log.Printf("storage-transform: transform error: %v", err)
		writeError(w, http.StatusInternalServerError, "image transform failed")
		return
	}

	// Store in cache (async — don't block the response).
	go s.cache.Set(context.Background(), cacheKey, result)

	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Header().Set("Content-Type", mime)
	w.WriteHeader(http.StatusOK)
	w.Write(result) //nolint:errcheck
}

// fetchSource downloads the raw source image from the storage plugin.
func (s *server) fetchSource(ctx context.Context, srcURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024)) // 50 MB max
}

// handleHealth returns 200 OK with service status.
func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"status":  "ok",
		"service": "storage-transform",
		"version": "0.1.0",
	})
}

// handleCacheStats returns Redis cache hit/miss/eviction counters.
func (s *server) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := s.cache.Stats(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats) //nolint:errcheck
}

// handleCachePurge clears all or prefix-matched cache entries.
func (s *server) handleCachePurge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prefix string `json:"prefix"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := s.cache.Purge(r.Context(), body.Prefix); err != nil {
		writeError(w, http.StatusInternalServerError, "purge failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "purged"}) //nolint:errcheck
}

// verifyHMAC checks the X-Signature header = HMAC-SHA256(secret, path?query).
func verifyHMAC(r *http.Request, secret string) bool {
	sig := r.Header.Get("X-Signature")
	if sig == "" {
		// Also accept via query param for Nginx sub_filter compatibility.
		sig = r.URL.Query().Get("sig")
	}
	if sig == "" {
		return false
	}
	// Compute expected MAC over path + raw query (without the sig param itself).
	q := r.URL.Query()
	q.Del("sig")
	message := r.URL.Path + "?" + q.Encode()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

// mimeForParams returns the MIME type implied by the requested output format.
// Used when serving from cache where the format was baked into the bytes.
func mimeForParams(p internal.TransformParams) string {
	switch p.Format {
	case internal.FormatWebP:
		return "image/webp"
	case internal.FormatAVIF:
		return "image/avif"
	case internal.FormatPNG:
		return "image/png"
	default:
		return "image/jpeg"
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

// rateLimiter is a simple in-memory sliding-window rate limiter (per IP).
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	windows map[string]*window
}

type window struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(ratePerMin int) *rateLimiter {
	rl := &rateLimiter{
		limit:   ratePerMin,
		windows: make(map[string]*window),
	}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	w, ok := rl.windows[ip]
	if !ok || now.After(w.resetAt) {
		rl.windows[ip] = &window{count: 1, resetAt: now.Add(time.Minute)}
		return true
	}
	w.count++
	return w.count <= rl.limit
}

// cleanup removes stale windows every 5 minutes.
func (rl *rateLimiter) cleanup() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, w := range rl.windows {
			if now.After(w.resetAt) {
				delete(rl.windows, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Ensure os is imported (used for environment in startup error messages).
var _ = os.Getenv
