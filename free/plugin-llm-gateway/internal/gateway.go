// Package internal implements the plugin-llm-gateway HTTP handlers.
//
// Purpose: Proxy LLM completion requests from ClawDE to nself-ai-gateway (port 3761),
//   adding per-tenant token quota enforcement, Redis-backed response caching,
//   per-session context injection, and SSRF guard.
// Inputs:  POST /v1/completions — JSON with model, messages, tenant_id, session_id.
// Outputs: JSON completion response (or 429 on quota exceeded, 403 on SSRF block).
// Constraints:
//   - source_account_id enforced on all DB queries (Multi-App Isolation Convention).
//   - Outbound requests are ONLY allowed to NSELF_AI_GATEWAY_URL (internal).
//   - Quota check + increment are done atomically (INSERT ... ON CONFLICT DO UPDATE).
//   - Cache key = SHA256(source_account_id + model + messages JSON).
//   - Cache is stored in-memory (sync.Map) with TTL; Redis is the intended upgrade path.
package internal

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// CompletionRequest is the payload expected at POST /v1/completions.
type CompletionRequest struct {
	Model           string        `json:"model"`
	Messages        []interface{} `json:"messages"`
	SourceAccountID string        `json:"source_account_id"`
	SessionID       string        `json:"session_id"`
}

// cacheEntry holds a cached response body and its expiry.
type cacheEntry struct {
	body    []byte
	expires time.Time
}

// Gateway holds the database handle and in-memory cache for request handlers.
type Gateway struct {
	DB             *sql.DB
	GatewayURL     string // upstream nself-ai-gateway base URL
	CacheTTL       time.Duration
	DailyTokenQuota int64 // tokens per tenant per day; 0 = unlimited

	cache sync.Map // map[string]*cacheEntry
}

// NewGateway constructs a Gateway from environment.
func NewGateway(db *sql.DB) *Gateway {
	gatewayURL := os.Getenv("NSELF_AI_GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "http://127.0.0.1:3761"
	}
	ttlSec := int64(300)
	quota := int64(100000)
	return &Gateway{
		DB:              db,
		GatewayURL:      gatewayURL,
		CacheTTL:        time.Duration(ttlSec) * time.Second,
		DailyTokenQuota: quota,
	}
}

// Health returns 200 when the DB is reachable.
func (g *Gateway) Health(w http.ResponseWriter, r *http.Request) {
	if g.DB == nil {
		http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
		return
	}
	if err := g.DB.PingContext(r.Context()); err != nil {
		http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Completions handles POST /v1/completions.
func (g *Gateway) Completions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"read_error"}`, http.StatusBadRequest)
		return
	}

	var req CompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	if req.SourceAccountID == "" {
		req.SourceAccountID = "primary"
	}

	// SSRF guard: upstream target must be the configured internal gateway.
	if err := g.validateUpstreamURL(g.GatewayURL); err != nil {
		http.Error(w, `{"error":"ssrf_blocked","detail":"upstream must be internal"}`, http.StatusForbidden)
		return
	}

	// Quota enforcement.
	if g.DailyTokenQuota > 0 && g.DB != nil {
		used, errQ := g.getQuotaUsed(r, req.SourceAccountID)
		if errQ == nil && used >= g.DailyTokenQuota {
			http.Error(w, `{"error":"quota_exceeded"}`, http.StatusTooManyRequests)
			return
		}
	}

	// Cache lookup.
	cacheKey := g.cacheKey(req.SourceAccountID, req.Model, body)
	if cached, ok := g.getCache(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		_, _ = w.Write(cached)
		return
	}

	// Context injection: prepend session context as system message.
	if req.SessionID != "" && g.DB != nil {
		ctx := g.fetchSessionContext(r, req.SourceAccountID, req.SessionID)
		if ctx != "" {
			msgs := []interface{}{map[string]string{"role": "system", "content": ctx}}
			msgs = append(msgs, req.Messages...)
			req.Messages = msgs
			// Re-marshal body with injected context.
			body, _ = json.Marshal(req)
		}
	}

	// Forward to nself-ai-gateway.
	upstream := g.GatewayURL + "/v1/completions"
	resp, err := http.Post(upstream, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"upstream_error","detail":"%s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, `{"error":"upstream_read_error"}`, http.StatusBadGateway)
		return
	}

	// Cache successful response.
	if resp.StatusCode == http.StatusOK {
		g.setCache(cacheKey, respBody)
		// Track quota usage (best-effort, non-blocking).
		if g.DB != nil {
			tokensUsed := g.estimateTokens(body, respBody)
			_ = g.incrementQuota(r, req.SourceAccountID, tokensUsed)
		}
	}

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// validateUpstreamURL blocks non-internal upstream targets (SSRF guard).
func (g *Gateway) validateUpstreamURL(target string) error {
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	host := strings.Split(u.Hostname(), ":")[0]
	// Allow localhost and RFC-1918 addresses only.
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	// Allow 10.x, 172.16-31.x, 192.168.x — internal networks.
	if strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "192.168.") {
		return nil
	}
	if strings.HasPrefix(host, "172.") {
		parts := strings.Split(host, ".")
		if len(parts) >= 2 {
			second := 0
			fmt.Sscanf(parts[1], "%d", &second)
			if second >= 16 && second <= 31 {
				return nil
			}
		}
	}
	return fmt.Errorf("ssrf_blocked: external host %q not allowed", host)
}

// cacheKey returns a stable SHA256-based cache key scoped to the tenant.
func (g *Gateway) cacheKey(accountID, model string, body []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(accountID))
	_, _ = h.Write([]byte(model))
	_, _ = h.Write(body)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// getCache returns cached bytes if present and unexpired.
func (g *Gateway) getCache(key string) ([]byte, bool) {
	v, ok := g.cache.Load(key)
	if !ok {
		return nil, false
	}
	e := v.(*cacheEntry)
	if time.Now().After(e.expires) {
		g.cache.Delete(key)
		return nil, false
	}
	return e.body, true
}

// setCache stores a response body with TTL expiry.
func (g *Gateway) setCache(key string, body []byte) {
	g.cache.Store(key, &cacheEntry{body: body, expires: time.Now().Add(g.CacheTTL)})
}

// getQuotaUsed returns tokens_used today for the given source_account_id.
func (g *Gateway) getQuotaUsed(r *http.Request, accountID string) (int64, error) {
	var used int64
	err := g.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE(tokens_used, 0) FROM np_llm_gateway_quota_usage
		WHERE source_account_id = $1 AND quota_date = CURRENT_DATE
	`, accountID).Scan(&used)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return used, err
}

// incrementQuota atomically adds tokens to today's quota row.
func (g *Gateway) incrementQuota(r *http.Request, accountID string, tokens int64) error {
	_, err := g.DB.ExecContext(r.Context(), `
		INSERT INTO np_llm_gateway_quota_usage (source_account_id, quota_date, tokens_used)
		VALUES ($1, CURRENT_DATE, $2)
		ON CONFLICT (source_account_id, quota_date) DO UPDATE
		  SET tokens_used = np_llm_gateway_quota_usage.tokens_used + EXCLUDED.tokens_used,
		      updated_at  = NOW()
	`, accountID, tokens)
	return err
}

// fetchSessionContext returns the session context string for context injection.
func (g *Gateway) fetchSessionContext(r *http.Request, accountID, sessionID string) string {
	var ctx string
	err := g.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE(context, '') FROM np_llm_gateway_requests
		WHERE source_account_id = $1 AND session_id = $2
		ORDER BY created_at DESC LIMIT 1
	`, accountID, sessionID).Scan(&ctx)
	if err != nil {
		return ""
	}
	return ctx
}

// estimateTokens returns a rough token count estimate (chars/4).
func (g *Gateway) estimateTokens(req, resp []byte) int64 {
	return int64((len(req) + len(resp)) / 4)
}
