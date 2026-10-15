package internal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/nself-org/nself-geocoding/internal/provider"
	"github.com/nself-org/nself-geocoding/internal/ratelimit"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

func readJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-Id"); v != "" {
		return v
	}
	return "primary"
}

func hashQuery(queryType, text string) string {
	h := sha256.Sum256([]byte(queryType + ":" + strings.ToLower(strings.TrimSpace(text))))
	return fmt.Sprintf("%x", h)[:64]
}

// haversine returns distance in meters between two lat/lng points.
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000 // Earth radius in metres
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// ─── Handlers struct ────────────────────────────────────────────────────────

// Handlers holds the DB pool, config, provider, and rate limiter.
type Handlers struct {
	DB      *DB
	Cfg     *Config
	Prov    provider.Provider
	Limiter *ratelimit.Limiter
}

// cacheResult writes a geocoding result into geo_cache.
func (h *Handlers) cacheResult(ctx context.Context, account, queryType, queryText string, r provider.GeocodeResult) {
	if !h.Cfg.CacheEnabled {
		return
	}
	hash := hashQuery(queryType, queryText)
	ttlHours := h.Cfg.CacheTTLHours
	if ttlHours == 0 {
		ttlHours = h.Cfg.CacheTTLDays * 24
	}
	expiresAt := time.Now().Add(time.Duration(ttlHours) * time.Hour)

	_, _ = h.DB.Pool.Exec(ctx, `
		INSERT INTO geo_cache
		  (source_account_id, query_type, query_hash, query_text, provider,
		   lat, lng, formatted_address, country_code, state, city, postal_code,
		   expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (source_account_id, query_type, query_hash, provider)
		DO UPDATE SET
		  lat=EXCLUDED.lat, lng=EXCLUDED.lng,
		  formatted_address=EXCLUDED.formatted_address,
		  updated_at=NOW(), hit_count=geo_cache.hit_count+1,
		  expires_at=EXCLUDED.expires_at`,
		account, queryType, hash, queryText, r.Provider,
		r.Lat, r.Lng, r.DisplayName, r.CountryCode, r.Region, r.City, r.Postcode,
		expiresAt,
	)
}

// clientIP extracts the real client IP for rate limiting.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.SplitN(fwd, ",", 2)[0]
	}
	return r.RemoteAddr
}

// checkRateLimit writes 429 and returns false when the caller is over limit.
func (h *Handlers) checkRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if h.Limiter == nil {
		return true
	}
	if !h.Limiter.Allow(clientIP(r)) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error":   "rate_limited",
			"message": "too many requests — please slow down",
		})
		return false
	}
	return true
}

// ─── Health ──────────────────────────────────────────────────────────────────

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "geocoding",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	if _, err := h.DB.Pool.Exec(r.Context(), "SELECT 1"); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ready":     false,
			"plugin":    "geocoding",
			"error":     "Database unavailable",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":     true,
		"plugin":    "geocoding",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// nilIfEmpty returns nil for an empty JSON raw message (so COALESCE keeps existing value).
func nilIfEmpty(v json.RawMessage) any {
	if len(v) == 0 || string(v) == "null" {
		return nil
	}
	return v
}
