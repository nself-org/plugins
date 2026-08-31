package internal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/nself-org/nself-geocoding/internal/provider"
	"github.com/nself-org/nself-geocoding/internal/ratelimit"
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

// ─── Geocode (forward) — POST /api/v1/geocode and POST /geocode/forward ───────

func (h *Handlers) Geocode(w http.ResponseWriter, r *http.Request) {
	h.forwardHandler(w, r)
}

func (h *Handlers) ForwardGeocode(w http.ResponseWriter, r *http.Request) {
	h.forwardHandler(w, r)
}

func (h *Handlers) forwardHandler(w http.ResponseWriter, r *http.Request) {
	if !h.checkRateLimit(w, r) {
		return
	}
	var body struct {
		Address  string `json:"address"`
		Q        string `json:"q"`
		Language string `json:"language"`
		City     string `json:"city"`
		State    string `json:"state"`
		Country  string `json:"country"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "invalid JSON body"})
		return
	}
	query := body.Address
	if query == "" {
		query = body.Q
	}
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "address or q is required"})
		return
	}
	parts := []string{query}
	for _, p := range []string{body.City, body.State, body.Country} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	queryText := strings.Join(parts, ", ")
	queryHash := hashQuery("forward", queryText)
	account := sa(r)

	if h.Cfg.CacheEnabled {
		row := h.DB.Pool.QueryRow(r.Context(),
			`SELECT id, lat, lng, formatted_address, street_number, street_name, city,
			        state, state_code, country, country_code, postal_code, place_id, place_type, accuracy, provider
			   FROM geo_cache
			  WHERE source_account_id=$1 AND query_type='forward' AND query_hash=$2
			    AND (expires_at IS NULL OR expires_at > NOW())
			  LIMIT 1`,
			account, queryHash,
		)
		var c GeoCache
		err := row.Scan(&c.ID, &c.Lat, &c.Lng, &c.FormattedAddress, &c.StreetNumber, &c.StreetName,
			&c.City, &c.State, &c.StateCode, &c.Country, &c.CountryCode, &c.PostalCode,
			&c.PlaceID, &c.PlaceType, &c.Accuracy, &c.Provider)
		if err == nil {
			_, _ = h.DB.Pool.Exec(r.Context(),
				`UPDATE geo_cache SET hit_count=hit_count+1, updated_at=NOW() WHERE id=$1`, c.ID)
			writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{geoCacheToResult(c, true)}, "cached": true})
			return
		}
	}

	if h.Prov == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "provider_error",
			"message": "no geocoding provider configured — set GEOCODING_PROVIDERS",
		})
		return
	}

	results, err := h.Prov.Forward(r.Context(), queryText)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "provider_error", "message": err.Error()})
		return
	}
	if len(results) > 0 {
		h.cacheResult(r.Context(), account, "forward", queryText, results[0])
	}
	out := make([]map[string]any, 0, len(results))
	for _, res := range results {
		out = append(out, providerResultToMap(res, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "cached": false})
}

// ─── Reverse geocode — POST /api/v1/reverse and POST /geocode/reverse ─────────

func (h *Handlers) Reverse(w http.ResponseWriter, r *http.Request) {
	h.reverseHandler(w, r)
}

func (h *Handlers) ReverseGeocode(w http.ResponseWriter, r *http.Request) {
	h.reverseHandler(w, r)
}

func (h *Handlers) reverseHandler(w http.ResponseWriter, r *http.Request) {
	if !h.checkRateLimit(w, r) {
		return
	}
	var body struct {
		Lat      *float64 `json:"lat"`
		Lng      *float64 `json:"lng"`
		Language string   `json:"language"`
	}
	if err := readJSON(r, &body); err != nil || body.Lat == nil || body.Lng == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "lat and lng are required"})
		return
	}

	queryText := fmt.Sprintf("%.7f,%.7f", *body.Lat, *body.Lng)
	queryHash := hashQuery("reverse", queryText)
	account := sa(r)

	if h.Cfg.CacheEnabled {
		row := h.DB.Pool.QueryRow(r.Context(),
			`SELECT id, lat, lng, formatted_address, street_number, street_name, city,
			        state, state_code, country, country_code, postal_code, place_id, place_type, accuracy, provider
			   FROM geo_cache
			  WHERE source_account_id=$1 AND query_type='reverse' AND query_hash=$2
			    AND (expires_at IS NULL OR expires_at > NOW())
			  LIMIT 1`,
			account, queryHash,
		)
		var c GeoCache
		err := row.Scan(&c.ID, &c.Lat, &c.Lng, &c.FormattedAddress, &c.StreetNumber, &c.StreetName,
			&c.City, &c.State, &c.StateCode, &c.Country, &c.CountryCode, &c.PostalCode,
			&c.PlaceID, &c.PlaceType, &c.Accuracy, &c.Provider)
		if err == nil {
			_, _ = h.DB.Pool.Exec(r.Context(),
				`UPDATE geo_cache SET hit_count=hit_count+1, updated_at=NOW() WHERE id=$1`, c.ID)
			writeJSON(w, http.StatusOK, map[string]any{"data": geoCacheToResult(c, true), "cached": true})
			return
		}
	}

	if h.Prov == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "provider_error",
			"message": "no geocoding provider configured — set GEOCODING_PROVIDERS",
		})
		return
	}

	res, err := h.Prov.Reverse(r.Context(), *body.Lat, *body.Lng)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "provider_error", "message": err.Error()})
		return
	}
	h.cacheResult(r.Context(), account, "reverse", queryText, res)
	writeJSON(w, http.StatusOK, map[string]any{"data": providerResultToMap(res, false), "cached": false})
}

// ─── Batch forward — POST /geocode/batch-forward ─────────────────────────────

func (h *Handlers) BatchForward(w http.ResponseWriter, r *http.Request) {
	if !h.checkRateLimit(w, r) {
		return
	}
	var body struct {
		Queries []string `json:"queries"`
	}
	if err := readJSON(r, &body); err != nil || len(body.Queries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "queries array is required"})
		return
	}
	maxBatch := h.Cfg.MaxBatchSize
	if maxBatch <= 0 {
		maxBatch = 100
	}
	if len(body.Queries) > maxBatch {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": fmt.Sprintf("batch size %d exceeds maximum %d", len(body.Queries), maxBatch),
		})
		return
	}

	if h.Prov == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "provider_error",
			"message": "no geocoding provider configured",
		})
		return
	}

	account := sa(r)
	type batchItem struct {
		Query   string           `json:"query"`
		Results []map[string]any `json:"results"`
		Error   string           `json:"error,omitempty"`
	}

	items := make([]batchItem, len(body.Queries))
	var wg sync.WaitGroup
	for i, q := range body.Queries {
		wg.Add(1)
		go func(idx int, query string) {
			defer wg.Done()
			results, err := h.Prov.Forward(r.Context(), query)
			item := batchItem{Query: query}
			if err != nil {
				item.Error = err.Error()
			} else {
				out := make([]map[string]any, 0, len(results))
				for _, res := range results {
					out = append(out, providerResultToMap(res, false))
				}
				if len(results) > 0 {
					h.cacheResult(r.Context(), account, "forward", query, results[0])
				}
				item.Results = out
			}
			items[idx] = item
		}(i, q)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
}

// ─── Batch reverse — POST /geocode/batch-reverse ─────────────────────────────

func (h *Handlers) BatchReverse(w http.ResponseWriter, r *http.Request) {
	if !h.checkRateLimit(w, r) {
		return
	}
	var body struct {
		Points []struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"points"`
	}
	if err := readJSON(r, &body); err != nil || len(body.Points) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "points array is required"})
		return
	}
	maxBatch := h.Cfg.MaxBatchSize
	if maxBatch <= 0 {
		maxBatch = 100
	}
	if len(body.Points) > maxBatch {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": fmt.Sprintf("batch size %d exceeds maximum %d", len(body.Points), maxBatch),
		})
		return
	}

	if h.Prov == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "provider_error",
			"message": "no geocoding provider configured",
		})
		return
	}

	account := sa(r)
	type batchItem struct {
		Lat    float64        `json:"lat"`
		Lng    float64        `json:"lng"`
		Result map[string]any `json:"result,omitempty"`
		Error  string         `json:"error,omitempty"`
	}

	items := make([]batchItem, len(body.Points))
	var wg sync.WaitGroup
	for i, pt := range body.Points {
		wg.Add(1)
		go func(idx int, lat, lng float64) {
			defer wg.Done()
			item := batchItem{Lat: lat, Lng: lng}
			res, err := h.Prov.Reverse(r.Context(), lat, lng)
			if err != nil {
				item.Error = err.Error()
			} else {
				queryText := fmt.Sprintf("%.7f,%.7f", lat, lng)
				h.cacheResult(r.Context(), account, "reverse", queryText, res)
				item.Result = providerResultToMap(res, false)
			}
			items[idx] = item
		}(i, pt.Lat, pt.Lng)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
}

// ─── Autocomplete — GET /geocode/autocomplete ─────────────────────────────────

func (h *Handlers) Autocomplete(w http.ResponseWriter, r *http.Request) {
	if !h.checkRateLimit(w, r) {
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "q parameter is required"})
		return
	}
	if h.Prov == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "provider_error",
			"message": "no geocoding provider configured",
		})
		return
	}
	// Autocomplete is implemented as a forward geocode with partial query;
	// providers naturally return ranked suggestions for partial strings.
	results, err := h.Prov.Forward(r.Context(), q)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "provider_error", "message": err.Error()})
		return
	}
	type suggestion struct {
		DisplayName string  `json:"display_name"`
		Lat         float64 `json:"lat"`
		Lng         float64 `json:"lng"`
		Provider    string  `json:"provider"`
	}
	suggestions := make([]suggestion, 0, len(results))
	for _, res := range results {
		suggestions = append(suggestions, suggestion{
			DisplayName: res.DisplayName,
			Lat:         res.Lat,
			Lng:         res.Lng,
			Provider:    res.Provider,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": suggestions})
}

// ─── Timezone — GET /geocode/tz ───────────────────────────────────────────────

func (h *Handlers) Timezone(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	latStr := q.Get("lat")
	lngStr := q.Get("lng")
	if latStr == "" || lngStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "lat and lng query parameters are required"})
		return
	}
	lat, err := parseFloat(latStr)
	if err != nil || lat < -90 || lat > 90 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "lat must be in range [-90, 90]"})
		return
	}
	lng, err := parseFloat(lngStr)
	if err != nil || lng < -180 || lng > 180 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "lng must be in range [-180, 180]"})
		return
	}
	tz := provider.TZAtCoords(lat, lng)
	writeJSON(w, http.StatusOK, map[string]any{
		"lat": lat,
		"lng": lng,
		"tz":  tz,
	})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func providerResultToMap(r provider.GeocodeResult, cached bool) map[string]any {
	return map[string]any{
		"lat":          r.Lat,
		"lng":          r.Lng,
		"display_name": r.DisplayName,
		"country_code": r.CountryCode,
		"region":       r.Region,
		"city":         r.City,
		"postcode":     r.Postcode,
		"confidence":   r.Confidence,
		"provider":     r.Provider,
		"cached":       cached,
	}
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

// helper: convert GeoCache row to a GeoResult map
func geoCacheToResult(c GeoCache, cached bool) map[string]any {
	lat := 0.0
	if c.Lat != nil {
		lat = *c.Lat
	}
	lng := 0.0
	if c.Lng != nil {
		lng = *c.Lng
	}
	return map[string]any{
		"lat":               lat,
		"lng":               lng,
		"formatted_address": c.FormattedAddress,
		"street_number":     c.StreetNumber,
		"street_name":       c.StreetName,
		"city":              c.City,
		"state":             c.State,
		"state_code":        c.StateCode,
		"country":           c.Country,
		"country_code":      c.CountryCode,
		"postal_code":       c.PostalCode,
		"place_id":          c.PlaceID,
		"place_type":        c.PlaceType,
		"accuracy":          c.Accuracy,
		"provider":          c.Provider,
		"cached":            cached,
	}
}

// ─── Geofences ───────────────────────────────────────────────────────────────

func (h *Handlers) ListGeofences(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	q := r.URL.Query()

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, name, description, fence_type, center_lat, center_lng,
		        radius_meters, polygon, active, notify_on_enter, notify_on_exit, notify_url,
		        metadata, created_by, created_at, updated_at
		   FROM geo_geofences
		  WHERE source_account_id=$1 AND deleted_at IS NULL
		  ORDER BY created_at DESC`,
		account,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	fences := []Geofence{}
	for rows.Next() {
		var f Geofence
		if err := rows.Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &f.FenceType,
			&f.CenterLat, &f.CenterLng, &f.RadiusMeters, &f.Polygon, &f.Active,
			&f.NotifyOnEnter, &f.NotifyOnExit, &f.NotifyURL, &f.Metadata,
			&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			continue
		}
		// optional active filter
		if active := q.Get("active"); active != "" {
			want := active == "true"
			if f.Active != want {
				continue
			}
		}
		fences = append(fences, f)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": fences, "total": len(fences)})
}

func (h *Handlers) CreateGeofence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string          `json:"name"`
		Description   *string         `json:"description"`
		FenceType     string          `json:"fence_type"`
		CenterLat     *float64        `json:"center_lat"`
		CenterLng     *float64        `json:"center_lng"`
		RadiusMeters  *float64        `json:"radius_meters"`
		Polygon       json.RawMessage `json:"polygon"`
		NotifyOnEnter bool            `json:"notify_on_enter"`
		NotifyOnExit  bool            `json:"notify_on_exit"`
		NotifyURL     *string         `json:"notify_url"`
		Metadata      json.RawMessage `json:"metadata"`
		CreatedBy     *string         `json:"created_by"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.CenterLat == nil || body.CenterLng == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, center_lat, and center_lng are required"})
		return
	}
	fenceType := body.FenceType
	if fenceType == "" {
		fenceType = "circle"
	}
	meta := body.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	account := sa(r)

	var f Geofence
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO geo_geofences
		   (source_account_id, name, description, fence_type, center_lat, center_lng,
		    radius_meters, polygon, notify_on_enter, notify_on_exit, notify_url, metadata, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING id, source_account_id, name, description, fence_type, center_lat, center_lng,
		           radius_meters, polygon, active, notify_on_enter, notify_on_exit, notify_url,
		           metadata, created_by, created_at, updated_at`,
		account, body.Name, body.Description, fenceType, body.CenterLat, body.CenterLng,
		body.RadiusMeters, body.Polygon, body.NotifyOnEnter, body.NotifyOnExit,
		body.NotifyURL, meta, body.CreatedBy,
	).Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &f.FenceType,
		&f.CenterLat, &f.CenterLng, &f.RadiusMeters, &f.Polygon, &f.Active,
		&f.NotifyOnEnter, &f.NotifyOnExit, &f.NotifyURL, &f.Metadata,
		&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handlers) GetGeofence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var f Geofence
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, name, description, fence_type, center_lat, center_lng,
		        radius_meters, polygon, active, notify_on_enter, notify_on_exit, notify_url,
		        metadata, created_by, created_at, updated_at
		   FROM geo_geofences
		  WHERE id=$1 AND source_account_id=$2 AND deleted_at IS NULL`,
		id, account,
	).Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &f.FenceType,
		&f.CenterLat, &f.CenterLng, &f.RadiusMeters, &f.Polygon, &f.Active,
		&f.NotifyOnEnter, &f.NotifyOnExit, &f.NotifyURL, &f.Metadata,
		&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "geofence not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handlers) UpdateGeofence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)

	var body struct {
		Name          *string         `json:"name"`
		Description   *string         `json:"description"`
		CenterLat     *float64        `json:"center_lat"`
		CenterLng     *float64        `json:"center_lng"`
		RadiusMeters  *float64        `json:"radius_meters"`
		Polygon       json.RawMessage `json:"polygon"`
		Active        *bool           `json:"active"`
		NotifyOnEnter *bool           `json:"notify_on_enter"`
		NotifyOnExit  *bool           `json:"notify_on_exit"`
		NotifyURL     *string         `json:"notify_url"`
		Metadata      json.RawMessage `json:"metadata"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var f Geofence
	err := h.DB.Pool.QueryRow(r.Context(),
		`UPDATE geo_geofences SET
		   name          = COALESCE($3, name),
		   description   = COALESCE($4, description),
		   center_lat    = COALESCE($5, center_lat),
		   center_lng    = COALESCE($6, center_lng),
		   radius_meters = COALESCE($7, radius_meters),
		   polygon       = COALESCE($8, polygon),
		   active        = COALESCE($9, active),
		   notify_on_enter = COALESCE($10, notify_on_enter),
		   notify_on_exit  = COALESCE($11, notify_on_exit),
		   notify_url    = COALESCE($12, notify_url),
		   metadata      = COALESCE($13, metadata),
		   updated_at    = NOW()
		 WHERE id=$1 AND source_account_id=$2 AND deleted_at IS NULL
		 RETURNING id, source_account_id, name, description, fence_type, center_lat, center_lng,
		           radius_meters, polygon, active, notify_on_enter, notify_on_exit, notify_url,
		           metadata, created_by, created_at, updated_at`,
		id, account, body.Name, body.Description, body.CenterLat, body.CenterLng,
		body.RadiusMeters, nilIfEmpty(body.Polygon), body.Active,
		body.NotifyOnEnter, body.NotifyOnExit, body.NotifyURL, nilIfEmpty(body.Metadata),
	).Scan(&f.ID, &f.SourceAccountID, &f.Name, &f.Description, &f.FenceType,
		&f.CenterLat, &f.CenterLng, &f.RadiusMeters, &f.Polygon, &f.Active,
		&f.NotifyOnEnter, &f.NotifyOnExit, &f.NotifyURL, &f.Metadata,
		&f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "geofence not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handlers) DeleteGeofence(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE geo_geofences SET deleted_at=NOW() WHERE id=$1 AND source_account_id=$2 AND deleted_at IS NULL`,
		id, account,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "geofence not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// CheckGeofence checks whether a point is inside any active geofences.
func (h *Handlers) CheckGeofence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Lat        *float64 `json:"lat"`
		Lng        *float64 `json:"lng"`
		EntityID   string   `json:"entity_id"`
		EntityType string   `json:"entity_type"`
	}
	if err := readJSON(r, &body); err != nil || body.Lat == nil || body.Lng == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lat and lng are required"})
		return
	}
	account := sa(r)
	entityType := body.EntityType
	if entityType == "" {
		entityType = "user"
	}

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, name, center_lat, center_lng, radius_meters, fence_type
		   FROM geo_geofences
		  WHERE source_account_id=$1 AND active=TRUE AND deleted_at IS NULL`,
		account,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var results []GeofenceEvalResult
	for rows.Next() {
		var fid, fname, ftype string
		var clat, clng float64
		var radius *float64
		if err := rows.Scan(&fid, &fname, &clat, &clng, &radius, &ftype); err != nil {
			continue
		}
		dist := haversine(*body.Lat, *body.Lng, clat, clng)
		inside := false
		if ftype == "circle" && radius != nil {
			inside = dist <= *radius
		}
		if inside && body.EntityID != "" {
			_, _ = h.DB.Pool.Exec(r.Context(),
				`INSERT INTO geo_geofence_events
				   (source_account_id, geofence_id, event_type, entity_id, entity_type, lat, lng)
				 VALUES ($1,$2,'enter',$3,$4,$5,$6)`,
				account, fid, body.EntityID, entityType, *body.Lat, *body.Lng,
			)
		}
		results = append(results, GeofenceEvalResult{
			GeofenceID:     fid,
			GeofenceName:   fname,
			Inside:         inside,
			DistanceMeters: dist,
		})
	}
	if results == nil {
		results = []GeofenceEvalResult{}
	}
	insideCount := 0
	for _, r := range results {
		if r.Inside {
			insideCount++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": results, "inside_count": insideCount})
}

// ─── Places ──────────────────────────────────────────────────────────────────

func (h *Handlers) ListPlaces(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, provider, provider_place_id, name, category, lat, lng,
		        formatted_address, phone, website, rating, review_count, hours, photos, metadata, created_at, updated_at
		   FROM geo_places
		  WHERE source_account_id=$1
		  ORDER BY created_at DESC`,
		account,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	places := []Place{}
	for rows.Next() {
		var p Place
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.Provider, &p.ProviderPlaceID, &p.Name,
			&p.Category, &p.Lat, &p.Lng, &p.FormattedAddress, &p.Phone, &p.Website,
			&p.Rating, &p.ReviewCount, &p.Hours, &p.Photos, &p.Metadata,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		places = append(places, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": places, "total": len(places)})
}

func (h *Handlers) CreatePlace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider        string          `json:"provider"`
		ProviderPlaceID string          `json:"provider_place_id"`
		Name            string          `json:"name"`
		Category        *string         `json:"category"`
		Lat             *float64        `json:"lat"`
		Lng             *float64        `json:"lng"`
		FormattedAddress *string        `json:"formatted_address"`
		Phone           *string         `json:"phone"`
		Website         *string         `json:"website"`
		Rating          *float64        `json:"rating"`
		ReviewCount     *int            `json:"review_count"`
		Hours           json.RawMessage `json:"hours"`
		Photos          json.RawMessage `json:"photos"`
		Metadata        json.RawMessage `json:"metadata"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.Lat == nil || body.Lng == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, lat, and lng are required"})
		return
	}
	if body.Provider == "" {
		body.Provider = "manual"
	}
	if body.ProviderPlaceID == "" {
		body.ProviderPlaceID = fmt.Sprintf("manual-%d", time.Now().UnixNano())
	}
	photos := body.Photos
	if len(photos) == 0 {
		photos = json.RawMessage(`[]`)
	}
	meta := body.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	account := sa(r)

	var p Place
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO geo_places
		   (source_account_id, provider, provider_place_id, name, category, lat, lng,
		    formatted_address, phone, website, rating, review_count, hours, photos, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 RETURNING id, source_account_id, provider, provider_place_id, name, category, lat, lng,
		           formatted_address, phone, website, rating, review_count, hours, photos, metadata, created_at, updated_at`,
		account, body.Provider, body.ProviderPlaceID, body.Name, body.Category, body.Lat, body.Lng,
		body.FormattedAddress, body.Phone, body.Website, body.Rating, body.ReviewCount,
		body.Hours, photos, meta,
	).Scan(&p.ID, &p.SourceAccountID, &p.Provider, &p.ProviderPlaceID, &p.Name, &p.Category,
		&p.Lat, &p.Lng, &p.FormattedAddress, &p.Phone, &p.Website, &p.Rating, &p.ReviewCount,
		&p.Hours, &p.Photos, &p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handlers) GetPlace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var p Place
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, provider, provider_place_id, name, category, lat, lng,
		        formatted_address, phone, website, rating, review_count, hours, photos, metadata, created_at, updated_at
		   FROM geo_places WHERE id=$1 AND source_account_id=$2`,
		id, account,
	).Scan(&p.ID, &p.SourceAccountID, &p.Provider, &p.ProviderPlaceID, &p.Name, &p.Category,
		&p.Lat, &p.Lng, &p.FormattedAddress, &p.Phone, &p.Website, &p.Rating, &p.ReviewCount,
		&p.Hours, &p.Photos, &p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "place not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) DeletePlace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM geo_places WHERE id=$1 AND source_account_id=$2`, id, account)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "place not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ─── Cache ───────────────────────────────────────────────────────────────────

func (h *Handlers) ListCache(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, query_type, query_hash, query_text, provider,
		        lat, lng, formatted_address, hit_count, created_at, updated_at, expires_at
		   FROM geo_cache WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 200`,
		account,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	entries := []GeoCache{}
	for rows.Next() {
		var c GeoCache
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.QueryType, &c.QueryHash, &c.QueryText,
			&c.Provider, &c.Lat, &c.Lng, &c.FormattedAddress, &c.HitCount,
			&c.CreatedAt, &c.UpdatedAt, &c.ExpiresAt); err != nil {
			continue
		}
		entries = append(entries, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "total": len(entries)})
}

func (h *Handlers) DeleteCacheEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM geo_cache WHERE id=$1 AND source_account_id=$2`, id, account)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "cache entry not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ─── Stats ───────────────────────────────────────────────────────────────────

func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	ctx := r.Context()

	var stats PluginStats
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_cache WHERE source_account_id=$1`, account,
	).Scan(&stats.TotalCacheEntries)
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_geofences WHERE source_account_id=$1 AND deleted_at IS NULL`, account,
	).Scan(&stats.TotalGeofences)
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_geofences WHERE source_account_id=$1 AND active=TRUE AND deleted_at IS NULL`, account,
	).Scan(&stats.ActiveGeofences)
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_geofence_events WHERE source_account_id=$1`, account,
	).Scan(&stats.TotalGeofenceEvents)
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_places WHERE source_account_id=$1`, account,
	).Scan(&stats.TotalPlaces)

	var totalHits, totalEntries int
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(hit_count),0), COUNT(*) FROM geo_cache WHERE source_account_id=$1`, account,
	).Scan(&totalHits, &totalEntries)
	if totalEntries > 0 {
		stats.CacheHitRate = float64(totalHits) / float64(totalEntries)
	}

	writeJSON(w, http.StatusOK, stats)
}

// ─── Quota ───────────────────────────────────────────────────────────────────

func (h *Handlers) GetQuota(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	ctx := r.Context()

	getQuota := func(qtype string) map[string]int {
		var api, geo, hits int
		_ = h.DB.Pool.QueryRow(ctx,
			`SELECT COALESCE(api_calls,0), COALESCE(geocode_calls,0), COALESCE(cache_hits,0)
			   FROM geo_api_quotas
			  WHERE source_account_id=$1 AND quota_type=$2 AND period_start=CURRENT_DATE`,
			account, qtype,
		).Scan(&api, &geo, &hits)
		return map[string]int{"api_calls": api, "geocode_calls": geo, "cache_hits": hits}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"daily":   getQuota("daily"),
		"monthly": getQuota("monthly"),
	})
}

// ─── Utility ─────────────────────────────────────────────────────────────────

// incrementQuota upserts a quota row for the current day.
func incrementQuota(ctx context.Context, db *DB, account, quotaType string, apiCall, cacheHit bool) {
	geocodeCall := apiCall && !cacheHit
	_, _ = db.Pool.Exec(ctx,
		`INSERT INTO geo_api_quotas (source_account_id, quota_type, period_start, api_calls, geocode_calls, cache_hits)
		 VALUES ($1, $2, CURRENT_DATE,
		         CASE WHEN $3 THEN 1 ELSE 0 END,
		         CASE WHEN $4 THEN 1 ELSE 0 END,
		         CASE WHEN $5 THEN 1 ELSE 0 END)
		 ON CONFLICT (source_account_id, quota_type, period_start)
		 DO UPDATE SET
		   api_calls     = geo_api_quotas.api_calls     + CASE WHEN $3 THEN 1 ELSE 0 END,
		   geocode_calls = geo_api_quotas.geocode_calls + CASE WHEN $4 THEN 1 ELSE 0 END,
		   cache_hits    = geo_api_quotas.cache_hits    + CASE WHEN $5 THEN 1 ELSE 0 END,
		   updated_at    = NOW()`,
		account, quotaType, apiCall, geocodeCall, cacheHit,
	)
}

// nilIfEmpty returns nil for an empty JSON raw message (so COALESCE keeps existing value).
func nilIfEmpty(v json.RawMessage) any {
	if len(v) == 0 || string(v) == "null" {
		return nil
	}
	return v
}
