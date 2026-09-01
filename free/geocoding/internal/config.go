package internal

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration for the geocoding plugin.
type Config struct {
	Port        string
	Host        string
	DatabaseURL string

	// Provider settings
	Providers       string
	GoogleAPIKey    string
	MapboxToken     string
	NominatimURL    string
	NominatimEmail  string
	NominatimUA     string // GEOCODING_NOMINATIM_UA — required for Nominatim (OSM policy)

	// Cache settings
	CacheTTLDays   int
	CacheEnabled   bool
	CacheTTLHours  int // GEOCODING_CACHE_TTL_HOURS — overrides CacheTTLDays when set

	// Batch settings
	MaxBatchSize int

	// Geofence settings
	GeofenceToleranceMeters int
	NotifyURL               string

	// Security
	APIKey          string
	RateLimitMax    int
	RateLimitWindow int // milliseconds
}

// LoadConfig reads environment variables and returns a Config.
// Defaults match plugin.json and TypeScript implementation.
func LoadConfig() *Config {
	return &Config{
		Port:        envOr("GEOCODING_PLUGIN_PORT", envOr("PORT", "3203")),
		Host:        envOr("GEOCODING_PLUGIN_HOST", envOr("HOST", "0.0.0.0")),
		DatabaseURL: envOr("DATABASE_URL", ""),

		Providers:      envOr("GEOCODING_PROVIDERS", "nominatim"),
		GoogleAPIKey:   envOr("GEOCODING_GOOGLE_API_KEY", ""),
		MapboxToken:    envOr("GEOCODING_MAPBOX_ACCESS_TOKEN", envOr("GEOCODING_MAPBOX_TOKEN", "")),
		NominatimURL:   envOr("GEOCODING_NOMINATIM_URL", "https://nominatim.openstreetmap.org"),
		NominatimEmail: envOr("GEOCODING_NOMINATIM_EMAIL", ""),
		NominatimUA:    envOr("GEOCODING_NOMINATIM_UA", "nself-geocoding-plugin/1.1.1 (contact@nself.org)"),

		CacheTTLDays:  envInt("GEOCODING_CACHE_TTL_DAYS", 365),
		CacheTTLHours: envInt("GEOCODING_CACHE_TTL_HOURS", 0), // 0 = use CacheTTLDays
		CacheEnabled:  envOr("GEOCODING_CACHE_ENABLED", "true") != "false",

		MaxBatchSize: envInt("GEOCODING_MAX_BATCH_SIZE", 100),

		GeofenceToleranceMeters: envInt("GEOCODING_GEOFENCE_CHECK_TOLERANCE_METERS", 50),
		NotifyURL:               envOr("GEOCODING_NOTIFY_URL", ""),

		APIKey:          envOr("GEOCODING_API_KEY", ""),
		RateLimitMax:    envInt("GEOCODING_RATE_LIMIT_MAX", 500),
		RateLimitWindow: envInt("GEOCODING_RATE_LIMIT_WINDOW_MS", 60000),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
