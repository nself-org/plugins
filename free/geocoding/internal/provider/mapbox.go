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
	ID        string  `json:"id"`
	PlaceName string  `json:"place_name"`
	Relevance float64 `json:"relevance"`
	Geometry  struct {
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
	defer func() { _ = resp.Body.Close() }()
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
