package provider

import (
	"context"
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
