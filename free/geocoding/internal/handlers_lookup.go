package internal

import (
	"fmt"
	"github.com/nself-org/nself-geocoding/internal/provider"
	"net/http"
)

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
