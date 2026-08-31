package internal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// sa returns the source_account_id from the X-Source-Account-ID header, defaulting to "primary".
func sa(r *http.Request) string {
	v := r.Header.Get("X-Source-Account-ID")
	if v == "" {
		return "primary"
	}
	v = strings.ToLower(v)
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	v = re.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return "primary"
	}
	return v
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errJSON writes a JSON error response.
func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// generateSignedURL builds a HMAC-SHA256 signed URL.
func generateSignedURL(rawURL, signingKey string, ttl int, ip string) string {
	expires := time.Now().Unix() + int64(ttl)
	data := fmt.Sprintf("%s%d%s", rawURL, expires, ip)
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(data))
	sig := hex.EncodeToString(mac.Sum(nil))

	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	result := fmt.Sprintf("%s%sexpires=%d&sig=%s", rawURL, sep, expires, sig)
	if ip != "" {
		result += "&ip=" + ip
	}
	return result
}

// ─── Health ───────────────────────────────────────────────────────────────────

// HandleHealth returns 200 ok.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "cdn",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleReady checks database connectivity.
func HandleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := db.Pool.Exec(r.Context(), "SELECT 1"); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ready":     false,
				"plugin":    "cdn",
				"error":     "Database unavailable",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ready":     true,
			"plugin":    "cdn",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ─── Zones ────────────────────────────────────────────────────────────────────

// HandleListZones returns all zones for the source account, optionally filtered by provider.
func HandleListZones(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAcct := sa(r)
		provider := r.URL.Query().Get("provider")

		var rows pgx.Rows
		var err error
		if provider != "" {
			rows, err = db.Pool.Query(r.Context(),
				`SELECT id, source_account_id, provider, zone_id, name, domain, origin_url,
				        ssl_enabled, cache_ttl, status, config, metadata, created_at, updated_at
				 FROM cdn_zones WHERE source_account_id = $1 AND provider = $2 ORDER BY name`,
				sourceAcct, provider)
		} else {
			rows, err = db.Pool.Query(r.Context(),
				`SELECT id, source_account_id, provider, zone_id, name, domain, origin_url,
				        ssl_enabled, cache_ttl, status, config, metadata, created_at, updated_at
				 FROM cdn_zones WHERE source_account_id = $1 ORDER BY name`,
				sourceAcct)
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		zones := make([]Zone, 0)
		for rows.Next() {
			var z Zone
			if err := rows.Scan(&z.ID, &z.SourceAccountID, &z.Provider, &z.ZoneID, &z.Name,
				&z.Domain, &z.OriginURL, &z.SSLEnabled, &z.CacheTTL, &z.Status,
				&z.Config, &z.Metadata, &z.CreatedAt, &z.UpdatedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			zones = append(zones, z)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": zones, "total": len(zones)})
	}
}

// HandleCreateZone creates a new CDN zone (upserts on provider+zone_id conflict).
func HandleCreateZone(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAcct := sa(r)
		var body struct {
			Provider   string          `json:"provider"`
			ZoneID     string          `json:"zone_id"`
			Name       string          `json:"name"`
			Domain     string          `json:"domain"`
			OriginURL  *string         `json:"origin_url"`
			SSLEnabled *bool           `json:"ssl_enabled"`
			CacheTTL   *int            `json:"cache_ttl"`
			Config     json.RawMessage `json:"config"`
			Metadata   json.RawMessage `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.Provider == "" || body.ZoneID == "" || body.Name == "" || body.Domain == "" {
			errJSON(w, http.StatusBadRequest, "provider, zone_id, name, and domain are required")
			return
		}

		sslEnabled := true
		if body.SSLEnabled != nil {
			sslEnabled = *body.SSLEnabled
		}
		cacheTTL := 86400
		if body.CacheTTL != nil {
			cacheTTL = *body.CacheTTL
		}
		configJSON := json.RawMessage("{}")
		if body.Config != nil {
			configJSON = body.Config
		}
		metaJSON := json.RawMessage("{}")
		if body.Metadata != nil {
			metaJSON = body.Metadata
		}

		var z Zone
		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO cdn_zones (source_account_id, provider, zone_id, name, domain, origin_url,
			  ssl_enabled, cache_ttl, config, metadata)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			 ON CONFLICT (source_account_id, provider, zone_id) DO UPDATE SET
			   name       = EXCLUDED.name,
			   domain     = EXCLUDED.domain,
			   origin_url = COALESCE(EXCLUDED.origin_url, cdn_zones.origin_url),
			   ssl_enabled = EXCLUDED.ssl_enabled,
			   cache_ttl  = EXCLUDED.cache_ttl,
			   config     = EXCLUDED.config,
			   metadata   = EXCLUDED.metadata,
			   updated_at = NOW()
			 RETURNING id, source_account_id, provider, zone_id, name, domain, origin_url,
			           ssl_enabled, cache_ttl, status, config, metadata, created_at, updated_at`,
			sourceAcct, body.Provider, body.ZoneID, body.Name, body.Domain, body.OriginURL,
			sslEnabled, cacheTTL, configJSON, metaJSON,
		).Scan(&z.ID, &z.SourceAccountID, &z.Provider, &z.ZoneID, &z.Name,
			&z.Domain, &z.OriginURL, &z.SSLEnabled, &z.CacheTTL, &z.Status,
			&z.Config, &z.Metadata, &z.CreatedAt, &z.UpdatedAt)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, z)
	}
}

// HandleGetZone returns a single zone by ID.
func HandleGetZone(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sourceAcct := sa(r)
		var z Zone
		err := db.Pool.QueryRow(r.Context(),
			`SELECT id, source_account_id, provider, zone_id, name, domain, origin_url,
			        ssl_enabled, cache_ttl, status, config, metadata, created_at, updated_at
			 FROM cdn_zones WHERE id = $1 AND source_account_id = $2`,
			id, sourceAcct,
		).Scan(&z.ID, &z.SourceAccountID, &z.Provider, &z.ZoneID, &z.Name,
			&z.Domain, &z.OriginURL, &z.SSLEnabled, &z.CacheTTL, &z.Status,
			&z.Config, &z.Metadata, &z.CreatedAt, &z.UpdatedAt)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "Zone not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, z)
	}
}

// HandleUpdateZone updates mutable fields of a zone.
func HandleUpdateZone(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sourceAcct := sa(r)

		var body struct {
			Name      *string         `json:"name"`
			Domain    *string         `json:"domain"`
			OriginURL *string         `json:"origin_url"`
			Status    *string         `json:"status"`
			CacheTTL  *int            `json:"cache_ttl"`
			Config    json.RawMessage `json:"config"`
			Metadata  json.RawMessage `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		// Fetch existing
		var z Zone
		err := db.Pool.QueryRow(r.Context(),
			`SELECT id, source_account_id, provider, zone_id, name, domain, origin_url,
			        ssl_enabled, cache_ttl, status, config, metadata, created_at, updated_at
			 FROM cdn_zones WHERE id = $1 AND source_account_id = $2`,
			id, sourceAcct,
		).Scan(&z.ID, &z.SourceAccountID, &z.Provider, &z.ZoneID, &z.Name,
			&z.Domain, &z.OriginURL, &z.SSLEnabled, &z.CacheTTL, &z.Status,
			&z.Config, &z.Metadata, &z.CreatedAt, &z.UpdatedAt)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "Zone not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		if body.Name != nil {
			z.Name = *body.Name
		}
		if body.Domain != nil {
			z.Domain = *body.Domain
		}
		if body.OriginURL != nil {
			z.OriginURL = body.OriginURL
		}
		if body.Status != nil {
			z.Status = *body.Status
		}
		if body.CacheTTL != nil {
			z.CacheTTL = *body.CacheTTL
		}
		if body.Config != nil {
			z.Config = body.Config
		}
		if body.Metadata != nil {
			z.Metadata = body.Metadata
		}

		err = db.Pool.QueryRow(r.Context(),
			`UPDATE cdn_zones SET name=$3, domain=$4, origin_url=$5, status=$6,
			   cache_ttl=$7, config=$8, metadata=$9, updated_at=NOW()
			 WHERE id=$1 AND source_account_id=$2
			 RETURNING id, source_account_id, provider, zone_id, name, domain, origin_url,
			           ssl_enabled, cache_ttl, status, config, metadata, created_at, updated_at`,
			id, sourceAcct, z.Name, z.Domain, z.OriginURL, z.Status,
			z.CacheTTL, z.Config, z.Metadata,
		).Scan(&z.ID, &z.SourceAccountID, &z.Provider, &z.ZoneID, &z.Name,
			&z.Domain, &z.OriginURL, &z.SSLEnabled, &z.CacheTTL, &z.Status,
			&z.Config, &z.Metadata, &z.CreatedAt, &z.UpdatedAt)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, z)
	}
}

// HandleDeleteZone removes a zone by ID.
func HandleDeleteZone(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sourceAcct := sa(r)
		tag, err := db.Pool.Exec(r.Context(),
			`DELETE FROM cdn_zones WHERE id=$1 AND source_account_id=$2`, id, sourceAcct)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tag.RowsAffected() == 0 {
			errJSON(w, http.StatusNotFound, "Zone not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── Purge ────────────────────────────────────────────────────────────────────

// HandlePurge creates a purge request and immediately marks it completed.
func HandlePurge(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAcct := sa(r)
		var body struct {
			ZoneID      string   `json:"zone_id"`
			PurgeType   string   `json:"purge_type"`
			URLs        []string `json:"urls"`
			Tags        []string `json:"tags"`
			Prefixes    []string `json:"prefixes"`
			RequestedBy *string  `json:"requested_by"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.ZoneID == "" {
			errJSON(w, http.StatusBadRequest, "zone_id is required")
			return
		}

		// Verify zone exists
		var zoneExists bool
		err := db.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM cdn_zones WHERE id=$1 AND source_account_id=$2)`,
			body.ZoneID, sourceAcct).Scan(&zoneExists)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !zoneExists {
			errJSON(w, http.StatusNotFound, "Zone not found")
			return
		}

		// Determine purge type
		purgeType := body.PurgeType
		if purgeType == "" {
			switch {
			case len(body.URLs) > 0:
				purgeType = "urls"
			case len(body.Tags) > 0:
				purgeType = "tags"
			case len(body.Prefixes) > 0:
				purgeType = "prefixes"
			default:
				errJSON(w, http.StatusBadRequest, "specify urls, tags, or prefixes to purge")
				return
			}
		}

		// Validate batch size
		if purgeType == "urls" && len(body.URLs) > cfg.PurgeBatchSize {
			errJSON(w, http.StatusBadRequest,
				fmt.Sprintf("URL count exceeds maximum batch size of %d", cfg.PurgeBatchSize))
			return
		}

		urlsJSON, _ := json.Marshal(body.URLs)
		tagsJSON, _ := json.Marshal(body.Tags)
		prefixesJSON, _ := json.Marshal(body.Prefixes)

		var purgeID string
		err = db.Pool.QueryRow(r.Context(),
			`INSERT INTO cdn_purge_requests
			   (source_account_id, zone_id, purge_type, urls, tags, prefixes, requested_by, status)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,'pending')
			 RETURNING id`,
			sourceAcct, body.ZoneID, purgeType, urlsJSON, tagsJSON, prefixesJSON, body.RequestedBy,
		).Scan(&purgeID)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Mark completed immediately (actual CDN API calls would be here)
		_, err = db.Pool.Exec(r.Context(),
			`UPDATE cdn_purge_requests SET status='completed', completed_at=NOW() WHERE id=$1`, purgeID)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"purge_request_id": purgeID,
			"status":           "completed",
			"purge_type":       purgeType,
		})
	}
}

// HandleListPurgeRequests returns purge request history.
func HandleListPurgeRequests(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAcct := sa(r)
		q := r.URL.Query()

		conditions := []string{"source_account_id = $1"}
		args := []any{sourceAcct}
		idx := 2

		if zid := q.Get("zone_id"); zid != "" {
			conditions = append(conditions, fmt.Sprintf("zone_id = $%d", idx))
			args = append(args, zid)
			idx++
		}
		if status := q.Get("status"); status != "" {
			conditions = append(conditions, fmt.Sprintf("status = $%d", idx))
			args = append(args, status)
			idx++
		}

		limit := 50
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		offset := 0
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		args = append(args, limit, offset)

		where := strings.Join(conditions, " AND ")
		rows, err := db.Pool.Query(r.Context(),
			fmt.Sprintf(`SELECT id, source_account_id, zone_id, purge_type, urls, tags, prefixes,
			              status, provider_request_id, requested_by, completed_at, error, created_at
			             FROM cdn_purge_requests WHERE %s
			             ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
				where, idx, idx+1),
			args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		result := make([]PurgeRequest, 0)
		for rows.Next() {
			var pr PurgeRequest
			if err := rows.Scan(&pr.ID, &pr.SourceAccountID, &pr.ZoneID, &pr.PurgeType,
				&pr.URLs, &pr.Tags, &pr.Prefixes, &pr.Status, &pr.ProviderRequestID,
				&pr.RequestedBy, &pr.CompletedAt, &pr.Error, &pr.CreatedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			result = append(result, pr)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
	}
}

// ─── Analytics ────────────────────────────────────────────────────────────────

// HandleListAnalytics returns analytics records with optional filtering.
func HandleListAnalytics(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAcct := sa(r)
		q := r.URL.Query()

		conditions := []string{"source_account_id = $1"}
		args := []any{sourceAcct}
		idx := 2

		if zid := q.Get("zone_id"); zid != "" {
			conditions = append(conditions, fmt.Sprintf("zone_id = $%d", idx))
			args = append(args, zid)
			idx++
		}
		if from := q.Get("start_date"); from != "" {
			conditions = append(conditions, fmt.Sprintf("date >= $%d", idx))
			args = append(args, from)
			idx++
		}
		if to := q.Get("end_date"); to != "" {
			conditions = append(conditions, fmt.Sprintf("date <= $%d", idx))
			args = append(args, to)
			idx++
		}

		limit := 30
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		offset := 0
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		where := strings.Join(conditions, " AND ")

		// Count
		var total int64
		if err := db.Pool.QueryRow(r.Context(),
			fmt.Sprintf("SELECT COUNT(*) FROM cdn_analytics WHERE %s", where), args...,
		).Scan(&total); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		args = append(args, limit, offset)
		rows, err := db.Pool.Query(r.Context(),
			fmt.Sprintf(`SELECT id, source_account_id, zone_id, date, requests_total, requests_cached,
			              bandwidth_total, bandwidth_cached, unique_visitors, threats_blocked,
			              status_2xx, status_3xx, status_4xx, status_5xx,
			              top_paths, top_countries, metadata, created_at
			             FROM cdn_analytics WHERE %s
			             ORDER BY date DESC LIMIT $%d OFFSET $%d`,
				where, idx, idx+1),
			args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		result := make([]Analytics, 0)
		for rows.Next() {
			var a Analytics
			if err := rows.Scan(&a.ID, &a.SourceAccountID, &a.ZoneID, &a.Date,
				&a.RequestsTotal, &a.RequestsCached, &a.BandwidthTotal, &a.BandwidthCached,
				&a.UniqueVisitors, &a.ThreatsBlocked, &a.Status2xx, &a.Status3xx, &a.Status4xx, &a.Status5xx,
				&a.TopPaths, &a.TopCountries, &a.Metadata, &a.CreatedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			result = append(result, a)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": result, "total": total})
	}
}

// ─── Signed URLs ──────────────────────────────────────────────────────────────

// HandleCreateSignedURL generates and stores a signed URL.
func HandleCreateSignedURL(db *DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAcct := sa(r)
		var body struct {
			ZoneID      string  `json:"zone_id"`
			Path        string  `json:"path"`
			ExpiresIn   *int    `json:"expires_in"`
			Method      string  `json:"method"`
			IPRestriction *string `json:"ip_restriction"`
			MaxAccess   *int    `json:"max_access"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.ZoneID == "" || body.Path == "" {
			errJSON(w, http.StatusBadRequest, "zone_id and path are required")
			return
		}
		if cfg.SigningKey == "" {
			errJSON(w, http.StatusInternalServerError, "CDN_SIGNING_KEY is not configured")
			return
		}

		// Verify zone
		var zoneExists bool
		if err := db.Pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM cdn_zones WHERE id=$1 AND source_account_id=$2)`,
			body.ZoneID, sourceAcct).Scan(&zoneExists); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !zoneExists {
			errJSON(w, http.StatusNotFound, "Zone not found")
			return
		}

		ttl := cfg.SignedURLTTL
		if body.ExpiresIn != nil {
			ttl = *body.ExpiresIn
		}
		ip := ""
		if body.IPRestriction != nil {
			ip = *body.IPRestriction
		}

		signed := generateSignedURL(body.Path, cfg.SigningKey, ttl, ip)
		expiresAt := time.Now().UTC().Add(time.Duration(ttl) * time.Second)

		var su SignedURL
		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO cdn_signed_urls
			   (source_account_id, zone_id, original_url, signed_url, expires_at, ip_restriction, max_access)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)
			 RETURNING id, source_account_id, zone_id, original_url, signed_url, expires_at,
			           ip_restriction, access_count, max_access, created_at`,
			sourceAcct, body.ZoneID, body.Path, signed, expiresAt, body.IPRestriction, body.MaxAccess,
		).Scan(&su.ID, &su.SourceAccountID, &su.ZoneID, &su.OriginalURL, &su.SignedURLValue,
			&su.ExpiresAt, &su.IPRestriction, &su.AccessCount, &su.MaxAccess, &su.CreatedAt)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":           su.ID,
			"signed_url":   su.SignedURLValue,
			"original_url": su.OriginalURL,
			"expires_at":   su.ExpiresAt.Format(time.RFC3339),
			"ttl":          ttl,
		})
	}
}

// HandleListSignedURLs returns signed URL records for the source account.
func HandleListSignedURLs(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAcct := sa(r)
		q := r.URL.Query()

		conditions := []string{"source_account_id = $1"}
		args := []any{sourceAcct}
		idx := 2

		if zid := q.Get("zone_id"); zid != "" {
			conditions = append(conditions, fmt.Sprintf("zone_id = $%d", idx))
			args = append(args, zid)
			idx++
		}

		limit := 50
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		offset := 0
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		args = append(args, limit, offset)

		where := strings.Join(conditions, " AND ")
		rows, err := db.Pool.Query(r.Context(),
			fmt.Sprintf(`SELECT id, source_account_id, zone_id, original_url, signed_url,
			              expires_at, ip_restriction, access_count, max_access, created_at
			             FROM cdn_signed_urls WHERE %s
			             ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
				where, idx, idx+1),
			args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		result := make([]SignedURL, 0)
		for rows.Next() {
			var su SignedURL
			if err := rows.Scan(&su.ID, &su.SourceAccountID, &su.ZoneID, &su.OriginalURL, &su.SignedURLValue,
				&su.ExpiresAt, &su.IPRestriction, &su.AccessCount, &su.MaxAccess, &su.CreatedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			result = append(result, su)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
	}
}

// HandleDeleteSignedURL removes a signed URL record.
func HandleDeleteSignedURL(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sourceAcct := sa(r)
		tag, err := db.Pool.Exec(r.Context(),
			`DELETE FROM cdn_signed_urls WHERE id=$1 AND source_account_id=$2`, id, sourceAcct)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tag.RowsAffected() == 0 {
			errJSON(w, http.StatusNotFound, "Signed URL not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── Stats ────────────────────────────────────────────────────────────────────

// HandleStats returns aggregate plugin statistics.
func HandleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAcct := sa(r)
		ctx := r.Context()

		var stats PluginStats
		err := db.Pool.QueryRow(ctx, `
			WITH zones AS (
			  SELECT COUNT(*) as total,
			         COUNT(*) FILTER (WHERE status = 'active') as active
			  FROM cdn_zones WHERE source_account_id = $1
			),
			purges AS (
			  SELECT COUNT(*) as total,
			         COUNT(*) FILTER (WHERE status = 'pending') as pending
			  FROM cdn_purge_requests WHERE source_account_id = $1
			),
			signed AS (
			  SELECT COUNT(*) as total,
			         COUNT(*) FILTER (WHERE expires_at > NOW()) as active
			  FROM cdn_signed_urls WHERE source_account_id = $1
			),
			analytics AS (
			  SELECT COUNT(DISTINCT date) as days,
			         COALESCE(SUM(requests_total), 0) as requests,
			         COALESCE(SUM(bandwidth_total), 0) as bandwidth
			  FROM cdn_analytics WHERE source_account_id = $1
			)
			SELECT z.total, z.active, p.total, p.pending, s.total, s.active,
			       a.days, a.requests, a.bandwidth
			FROM zones z CROSS JOIN purges p CROSS JOIN signed s CROSS JOIN analytics a`,
			sourceAcct,
		).Scan(&stats.TotalZones, &stats.ActiveZones, &stats.TotalPurgeRequests, &stats.PendingPurges,
			&stats.TotalSignedURLs, &stats.ActiveSignedURLs, &stats.AnalyticsDaysTracked,
			&stats.TotalRequestsTracked, &stats.TotalBandwidthTracked)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		// By provider
		rows, err := db.Pool.Query(ctx,
			`SELECT provider, COUNT(*) FROM cdn_zones WHERE source_account_id = $1 GROUP BY provider`,
			sourceAcct)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		stats.ByProvider = make(map[string]int64)
		for rows.Next() {
			var provider string
			var count int64
			if err := rows.Scan(&provider, &count); err == nil {
				stats.ByProvider[provider] = count
			}
		}

		writeJSON(w, http.StatusOK, stats)
	}
}

// getZoneByID is a helper used within handler context.
func getZoneByID(ctx context.Context, db *DB, id, sourceAcct string) (*Zone, error) {
	var z Zone
	err := db.Pool.QueryRow(ctx,
		`SELECT id, source_account_id, provider, zone_id, name, domain, origin_url,
		        ssl_enabled, cache_ttl, status, config, metadata, created_at, updated_at
		 FROM cdn_zones WHERE id = $1 AND source_account_id = $2`,
		id, sourceAcct,
	).Scan(&z.ID, &z.SourceAccountID, &z.Provider, &z.ZoneID, &z.Name,
		&z.Domain, &z.OriginURL, &z.SSLEnabled, &z.CacheTTL, &z.Status,
		&z.Config, &z.Metadata, &z.CreatedAt, &z.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &z, err
}
