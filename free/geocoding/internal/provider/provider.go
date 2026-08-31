// Package provider defines the geocoding provider interface and adapters.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GeocodeResult is the normalised result returned by any provider.
type GeocodeResult struct {
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	DisplayName string  `json:"display_name"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Postcode    string  `json:"postcode"`
	Confidence  float64 `json:"confidence"`
	Provider    string  `json:"provider"`
}

// Provider is the interface every geocoding adapter must satisfy.
type Provider interface {
	Forward(ctx context.Context, query string) ([]GeocodeResult, error)
	Reverse(ctx context.Context, lat, lng float64) (GeocodeResult, error)
	Name() string
}

// ─── Nominatim adapter ────────────────────────────────────────────────────────

// Nominatim calls the OpenStreetMap Nominatim API (free, 1 req/sec).
// GEOCODING_NOMINATIM_UA must be set to a descriptive User-Agent string.
type Nominatim struct {
	BaseURL   string
	UserAgent string
	client    *http.Client
}

// NewNominatim creates a Nominatim adapter.
// userAgent should contain a project name and contact email (OSM policy).
func NewNominatim(baseURL, userAgent string) *Nominatim {
	if baseURL == "" {
		baseURL = "https://nominatim.openstreetmap.org"
	}
	return &Nominatim{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		UserAgent: userAgent,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *Nominatim) Name() string { return "nominatim" }

// nominatimResult is the Nominatim JSON shape for /search and /reverse.
type nominatimResult struct {
	PlaceID     int64   `json:"place_id"`
	Lat         string  `json:"lat"`
	Lon         string  `json:"lon"`
	DisplayName string  `json:"display_name"`
	Importance  float64 `json:"importance"`
	Address     struct {
		HouseNumber  string `json:"house_number"`
		Road         string `json:"road"`
		City         string `json:"city"`
		Town         string `json:"town"`
		Village      string `json:"village"`
		State        string `json:"state"`
		Postcode     string `json:"postcode"`
		Country      string `json:"country"`
		CountryCode  string `json:"country_code"`
	} `json:"address"`
}

func (r nominatimResult) toResult(providerName string) (GeocodeResult, error) {
	lat, err := strconv.ParseFloat(r.Lat, 64)
	if err != nil {
		return GeocodeResult{}, fmt.Errorf("bad lat: %w", err)
	}
	lng, err := strconv.ParseFloat(r.Lon, 64)
	if err != nil {
		return GeocodeResult{}, fmt.Errorf("bad lon: %w", err)
	}
	city := r.Address.City
	if city == "" {
		city = r.Address.Town
	}
	if city == "" {
		city = r.Address.Village
	}
	return GeocodeResult{
		Lat:         lat,
		Lng:         lng,
		DisplayName: r.DisplayName,
		CountryCode: strings.ToUpper(r.Address.CountryCode),
		Region:      r.Address.State,
		City:        city,
		Postcode:    r.Address.Postcode,
		Confidence:  r.Importance,
		Provider:    providerName,
	}, nil
}

func (n *Nominatim) get(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
	u := n.BaseURL + endpoint + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", n.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nominatim request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("nominatim rate limit exceeded")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nominatim: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (n *Nominatim) Forward(ctx context.Context, query string) ([]GeocodeResult, error) {
	params := url.Values{
		"q":              {query},
		"format":         {"json"},
		"addressdetails": {"1"},
		"limit":          {"5"},
	}
	body, err := n.get(ctx, "/search", params)
	if err != nil {
		return nil, err
	}
	var raw []nominatimResult
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("nominatim parse: %w", err)
	}
	results := make([]GeocodeResult, 0, len(raw))
	for _, r := range raw {
		res, err := r.toResult(n.Name())
		if err == nil {
			results = append(results, res)
		}
	}
	return results, nil
}

func (n *Nominatim) Reverse(ctx context.Context, lat, lng float64) (GeocodeResult, error) {
	params := url.Values{
		"lat":            {strconv.FormatFloat(lat, 'f', 7, 64)},
		"lon":            {strconv.FormatFloat(lng, 'f', 7, 64)},
		"format":         {"json"},
		"addressdetails": {"1"},
	}
	body, err := n.get(ctx, "/reverse", params)
	if err != nil {
		return GeocodeResult{}, err
	}
	var raw nominatimResult
	if err := json.Unmarshal(body, &raw); err != nil {
		return GeocodeResult{}, fmt.Errorf("nominatim reverse parse: %w", err)
	}
	return raw.toResult(n.Name())
}

// ─── Mapbox adapter ────────────────────────────────────────────────────────────

// Mapbox calls the Mapbox Geocoding API v5.
// Requires GEOCODING_MAPBOX_TOKEN.
type Mapbox struct {
	Token  string
	client *http.Client
}

// NewMapbox creates a Mapbox adapter.
func NewMapbox(token string) *Mapbox {
	return &Mapbox{
		Token:  token,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (m *Mapbox) Name() string { return "mapbox" }

// mapboxFeature is the relevant subset of Mapbox GeoJSON features.
type mapboxFeature struct {
	ID          string    `json:"id"`
	PlaceName   string    `json:"place_name"`
	Relevance   float64   `json:"relevance"`
	Geometry    struct {
		Coordinates [2]float64 `json:"coordinates"` // [lng, lat]
	} `json:"geometry"`
	Context []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"context"`
	Properties struct {
		ShortCode string `json:"short_code"`
	} `json:"properties"`
}

type mapboxResponse struct {
	Features []mapboxFeature `json:"features"`
}

func (f mapboxFeature) toResult() GeocodeResult {
	var countryCode, region, city, postcode string
	for _, ctx := range f.Context {
		switch {
		case strings.HasPrefix(ctx.ID, "country."):
			countryCode = ctx.Text
		case strings.HasPrefix(ctx.ID, "region."):
			region = ctx.Text
		case strings.HasPrefix(ctx.ID, "place."):
			city = ctx.Text
		case strings.HasPrefix(ctx.ID, "postcode."):
			postcode = ctx.Text
		}
	}
	// Short-code on the feature itself is a 2-letter country code for country-level results
	if countryCode == "" {
		if strings.HasPrefix(f.ID, "country.") {
			countryCode = strings.ToUpper(f.Properties.ShortCode)
		}
	} else {
		countryCode = strings.ToUpper(countryCode)
	}
	return GeocodeResult{
		Lat:         f.Geometry.Coordinates[1],
		Lng:         f.Geometry.Coordinates[0],
		DisplayName: f.PlaceName,
		CountryCode: countryCode,
		Region:      region,
		City:        city,
		Postcode:    postcode,
		Confidence:  f.Relevance,
		Provider:    "mapbox",
	}
}

func (m *Mapbox) doRequest(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mapbox request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("mapbox: invalid token")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mapbox: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (m *Mapbox) Forward(ctx context.Context, query string) ([]GeocodeResult, error) {
	encoded := url.PathEscape(query)
	u := fmt.Sprintf("https://api.mapbox.com/geocoding/v5/mapbox.places/%s.json?access_token=%s&limit=5",
		encoded, m.Token)
	body, err := m.doRequest(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp mapboxResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("mapbox parse: %w", err)
	}
	results := make([]GeocodeResult, 0, len(resp.Features))
	for _, f := range resp.Features {
		results = append(results, f.toResult())
	}
	return results, nil
}

func (m *Mapbox) Reverse(ctx context.Context, lat, lng float64) (GeocodeResult, error) {
	u := fmt.Sprintf("https://api.mapbox.com/geocoding/v5/mapbox.places/%s,%s.json?access_token=%s&limit=1",
		strconv.FormatFloat(lng, 'f', 7, 64),
		strconv.FormatFloat(lat, 'f', 7, 64),
		m.Token)
	body, err := m.doRequest(ctx, u)
	if err != nil {
		return GeocodeResult{}, err
	}
	var resp mapboxResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return GeocodeResult{}, fmt.Errorf("mapbox reverse parse: %w", err)
	}
	if len(resp.Features) == 0 {
		return GeocodeResult{}, fmt.Errorf("mapbox: no results")
	}
	return resp.Features[0].toResult(), nil
}

// ─── Timezone lookup (no external call) ────────────────────────────────────────

// TZAtCoords returns the IANA timezone name for a lat/lng.
// Uses a UTC-offset approximation based on longitude, then refines with a
// small hard-coded table of major metropolitan areas. Suitable for EPG
// scheduling where exact DST boundary precision is not required.
//
// For production-grade precision, embed a full timezone boundary dataset
// (e.g. github.com/evanoberholster/timezoneLookup).
func TZAtCoords(lat, lng float64) string {
	// Major timezone lookup by proximity to known cities (lat, lng, tz)
	type tzPoint struct {
		lat, lng float64
		tz       string
	}
	landmarks := []tzPoint{
		// Americas
		{40.7128, -74.0060, "America/New_York"},
		{41.8781, -87.6298, "America/Chicago"},
		{39.7392, -104.9903, "America/Denver"},
		{34.0522, -118.2437, "America/Los_Angeles"},
		{61.2181, -149.9003, "America/Anchorage"},
		{21.3069, -157.8583, "Pacific/Honolulu"},
		{45.4215, -75.6919, "America/Toronto"},
		{43.6532, -79.3832, "America/Toronto"},
		{45.5017, -73.5673, "America/Toronto"},
		{49.2827, -123.1207, "America/Vancouver"},
		{-23.5505, -46.6333, "America/Sao_Paulo"},
		{-34.6037, -58.3816, "America/Argentina/Buenos_Aires"},
		{19.4326, -99.1332, "America/Mexico_City"},
		// Europe
		{51.5074, -0.1278, "Europe/London"},
		{48.8566, 2.3522, "Europe/Paris"},
		{52.5200, 13.4050, "Europe/Berlin"},
		{55.7558, 37.6173, "Europe/Moscow"},
		{41.0082, 28.9784, "Europe/Istanbul"},
		{59.9139, 10.7522, "Europe/Oslo"},
		{40.4168, -3.7038, "Europe/Madrid"},
		{41.9028, 12.4964, "Europe/Rome"},
		// Asia/Pacific
		{35.6762, 139.6503, "Asia/Tokyo"},
		{37.5665, 126.9780, "Asia/Seoul"},
		{39.9042, 116.4074, "Asia/Shanghai"},
		{22.3193, 114.1694, "Asia/Hong_Kong"},
		{1.3521, 103.8198, "Asia/Singapore"},
		{28.6139, 77.2090, "Asia/Kolkata"},
		{19.0760, 72.8777, "Asia/Kolkata"},
		{31.2304, 121.4737, "Asia/Shanghai"},
		{-33.8688, 151.2093, "Australia/Sydney"},
		{-37.8136, 144.9631, "Australia/Melbourne"},
		// Africa / Middle East
		{30.0444, 31.2357, "Africa/Cairo"},
		{6.5244, 3.3792, "Africa/Lagos"},
		{24.7136, 46.6753, "Asia/Riyadh"},
		{25.2048, 55.2708, "Asia/Dubai"},
	}

	best := "UTC"
	bestDist := math.MaxFloat64
	for _, p := range landmarks {
		dlat := lat - p.lat
		dlng := lng - p.lng
		dist := dlat*dlat + dlng*dlng // squared Euclidean — sufficient for nearest-point
		if dist < bestDist {
			bestDist = dist
			best = p.tz
		}
	}

	// If we're within ~3 degrees of a known landmark, trust it.
	if bestDist <= 9.0 {
		return best
	}

	// Fallback: approximate by UTC offset from longitude.
	offsetHours := math.Round(lng / 15.0)
	sign := "+"
	if offsetHours < 0 {
		sign = "-"
		offsetHours = -offsetHours
	}
	return fmt.Sprintf("Etc/GMT%s%d", sign, int(offsetHours))
}

// ─── Haversine distance (km) ──────────────────────────────────────────────────

// HaversineKm returns the great-circle distance in kilometres.
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dphi := (lat2 - lat1) * math.Pi / 180
	dlam := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dphi/2)*math.Sin(dphi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dlam/2)*math.Sin(dlam/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
