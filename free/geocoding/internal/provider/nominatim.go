package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

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
		HouseNumber string `json:"house_number"`
		Road        string `json:"road"`
		City        string `json:"city"`
		Town        string `json:"town"`
		Village     string `json:"village"`
		State       string `json:"state"`
		Postcode    string `json:"postcode"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
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
	defer func() { _ = resp.Body.Close() }()

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
