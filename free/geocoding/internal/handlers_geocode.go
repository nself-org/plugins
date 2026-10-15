package internal

import (
	"fmt"
	"net/http"
	"strings"
)

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
