package internal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/nself-geocoding/internal"
	"github.com/nself-org/nself-geocoding/internal/provider"
	"github.com/nself-org/nself-geocoding/internal/ratelimit"
)

// ─── Mock provider ────────────────────────────────────────────────────────────

type mockProvider struct {
	forwardResults []provider.GeocodeResult
	forwardErr     error
	reverseResult  provider.GeocodeResult
	reverseErr     error
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Forward(_ context.Context, _ string) ([]provider.GeocodeResult, error) {
	return m.forwardResults, m.forwardErr
}

func (m *mockProvider) Reverse(_ context.Context, _, _ float64) (provider.GeocodeResult, error) {
	return m.reverseResult, m.reverseErr
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// newHandlers builds a Handlers with no DB (nil) for unit tests that don't
// need DB — cache is disabled so no DB calls are made.
func newHandlers(prov provider.Provider) *internal.Handlers {
	cfg := &internal.Config{
		CacheEnabled: false,
		MaxBatchSize: 10,
		RateLimitMax: 1000,
	}
	return &internal.Handlers{
		Cfg:     cfg,
		Prov:    prov,
		Limiter: ratelimit.New(1000),
	}
}

func postJSON(t *testing.T, h http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func getQuery(t *testing.T, h http.HandlerFunc, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// ─── POST /geocode/forward ────────────────────────────────────────────────────

func TestForwardGeocode_ReturnsResults(t *testing.T) {
	prov := &mockProvider{
		forwardResults: []provider.GeocodeResult{
			{Lat: 40.7128, Lng: -74.0060, DisplayName: "New York, NY, USA",
				CountryCode: "US", Region: "New York", City: "New York", Provider: "mock"},
		},
	}
	h := newHandlers(prov)
	rec := postJSON(t, h.ForwardGeocode, map[string]string{"address": "New York"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok || len(data) == 0 {
		t.Fatalf("expected non-empty data array, got: %v", resp)
	}
	first := data[0].(map[string]any)
	if lat, _ := first["lat"].(float64); lat == 0 {
		t.Errorf("expected non-zero lat, got 0")
	}
}

func TestForwardGeocode_MissingAddress(t *testing.T) {
	h := newHandlers(&mockProvider{})
	rec := postJSON(t, h.ForwardGeocode, map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestForwardGeocode_NoProvider(t *testing.T) {
	cfg := &internal.Config{CacheEnabled: false}
	h := &internal.Handlers{Cfg: cfg, Limiter: ratelimit.New(100)}
	rec := postJSON(t, h.ForwardGeocode, map[string]string{"address": "Test"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

// ─── POST /geocode/reverse ────────────────────────────────────────────────────

func TestReverseGeocode_ReturnsResult(t *testing.T) {
	prov := &mockProvider{
		reverseResult: provider.GeocodeResult{
			Lat: 40.71, Lng: -74.01,
			DisplayName: "Manhattan, New York, USA",
			CountryCode: "US", City: "New York", Provider: "mock",
		},
	}
	h := newHandlers(prov)
	rec := postJSON(t, h.ReverseGeocode, map[string]float64{"lat": 40.71, "lng": -74.01})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T: %v", resp["data"], resp["data"])
	}
	if v, _ := data["display_name"].(string); !strings.Contains(v, "New York") {
		t.Errorf("expected display_name to contain 'New York', got %q", v)
	}
}

func TestReverseGeocode_MissingLatLng(t *testing.T) {
	h := newHandlers(&mockProvider{})
	rec := postJSON(t, h.ReverseGeocode, map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ─── POST /geocode/batch-forward ─────────────────────────────────────────────

func TestBatchForward_ReturnsItems(t *testing.T) {
	prov := &mockProvider{
		forwardResults: []provider.GeocodeResult{
			{Lat: 40.7128, Lng: -74.006, DisplayName: "New York", Provider: "mock"},
		},
	}
	h := newHandlers(prov)
	rec := postJSON(t, h.BatchForward, map[string]any{
		"queries": []string{"New York", "London"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok || len(data) != 2 {
		t.Errorf("expected 2 items in data, got %v", resp["data"])
	}
}

func TestBatchForward_EmptyQueries(t *testing.T) {
	h := newHandlers(&mockProvider{})
	rec := postJSON(t, h.BatchForward, map[string]any{"queries": []string{}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestBatchForward_ExceedsMaxBatch(t *testing.T) {
	cfg := &internal.Config{CacheEnabled: false, MaxBatchSize: 2, RateLimitMax: 1000}
	h := &internal.Handlers{Cfg: cfg, Prov: &mockProvider{}, Limiter: ratelimit.New(1000)}
	rec := postJSON(t, h.BatchForward, map[string]any{
		"queries": []string{"a", "b", "c"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized batch, got %d", rec.Code)
	}
}

// ─── POST /geocode/batch-reverse ─────────────────────────────────────────────

func TestBatchReverse_ReturnsItems(t *testing.T) {
	prov := &mockProvider{
		reverseResult: provider.GeocodeResult{Lat: 40.71, Lng: -74.01, DisplayName: "NYC", Provider: "mock"},
	}
	h := newHandlers(prov)
	rec := postJSON(t, h.BatchReverse, map[string]any{
		"points": []map[string]float64{
			{"lat": 40.71, "lng": -74.01},
			{"lat": 51.51, "lng": -0.13},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok || len(data) != 2 {
		t.Errorf("expected 2 items, got %v", resp["data"])
	}
}

// ─── GET /geocode/autocomplete ────────────────────────────────────────────────

func TestAutocomplete_ReturnsSuggestions(t *testing.T) {
	prov := &mockProvider{
		forwardResults: []provider.GeocodeResult{
			{Lat: 40.71, Lng: -74.01, DisplayName: "New York, NY", Provider: "mock"},
			{Lat: 40.69, Lng: -73.99, DisplayName: "New York City, USA", Provider: "mock"},
		},
	}
	h := newHandlers(prov)
	rec := getQuery(t, h.Autocomplete, "q=New+Yor")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok || len(data) != 2 {
		t.Errorf("expected 2 suggestions, got %v", resp["data"])
	}
}

func TestAutocomplete_MissingQ(t *testing.T) {
	h := newHandlers(&mockProvider{})
	rec := getQuery(t, h.Autocomplete, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ─── GET /geocode/tz ──────────────────────────────────────────────────────────

func TestTimezone_NewYork(t *testing.T) {
	h := newHandlers(nil) // no provider needed
	rec := getQuery(t, h.Timezone, "lat=40.71&lng=-74.01")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	tz, _ := resp["tz"].(string)
	if tz != "America/New_York" {
		t.Errorf("expected America/New_York, got %q", tz)
	}
}

func TestTimezone_MissingParams(t *testing.T) {
	h := newHandlers(nil)
	rec := getQuery(t, h.Timezone, "lat=40.71")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTimezone_InvalidLat(t *testing.T) {
	h := newHandlers(nil)
	rec := getQuery(t, h.Timezone, "lat=999&lng=0")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for out-of-range lat, got %d", rec.Code)
	}
}

// ─── Rate limiter ─────────────────────────────────────────────────────────────

func TestRateLimit_Returns429AfterLimit(t *testing.T) {
	prov := &mockProvider{
		forwardResults: []provider.GeocodeResult{
			{Lat: 1, Lng: 1, DisplayName: "Somewhere", Provider: "mock"},
		},
	}
	cfg := &internal.Config{CacheEnabled: false, MaxBatchSize: 10, RateLimitMax: 2}
	h := &internal.Handlers{Cfg: cfg, Prov: prov, Limiter: ratelimit.New(2)}

	body := map[string]string{"address": "Test"}
	var got429 bool
	for i := 0; i < 10; i++ {
		rec := postJSON(t, h.ForwardGeocode, body)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Error("expected a 429 after exhausting rate limit tokens, got none")
	}
}

// ─── Provider: TZAtCoords ─────────────────────────────────────────────────────

func TestTZAtCoords(t *testing.T) {
	cases := []struct {
		lat, lng float64
		want     string
	}{
		{40.71, -74.01, "America/New_York"},
		{51.51, -0.13, "Europe/London"},
		{48.85, 2.35, "Europe/Paris"},
		{35.68, 139.69, "Asia/Tokyo"},
		{-33.87, 151.21, "Australia/Sydney"},
	}
	for _, c := range cases {
		got := provider.TZAtCoords(c.lat, c.lng)
		if got != c.want {
			t.Errorf("TZAtCoords(%.2f, %.2f) = %q, want %q", c.lat, c.lng, got, c.want)
		}
	}
}

// ─── Provider: HaversineKm ────────────────────────────────────────────────────

func TestHaversineKm_ZeroToOneDegree(t *testing.T) {
	km := provider.HaversineKm(0, 0, 0, 1)
	// 1 degree of longitude at equator ≈ 111.195 km
	if km < 110 || km > 113 {
		t.Errorf("expected ~111 km, got %.3f", km)
	}
}

func TestHaversineKm_SamePoint(t *testing.T) {
	km := provider.HaversineKm(40.0, -74.0, 40.0, -74.0)
	if km != 0 {
		t.Errorf("expected 0 km for same point, got %.6f", km)
	}
}
