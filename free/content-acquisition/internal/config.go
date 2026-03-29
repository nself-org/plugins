package internal

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// LoadConfig reads and validates all environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	// Required: DATABASE_URL
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	// Required: METADATA_ENRICHMENT_URL
	cfg.MetadataEnrichmentURL = os.Getenv("METADATA_ENRICHMENT_URL")
	if cfg.MetadataEnrichmentURL == "" {
		return nil, fmt.Errorf("METADATA_ENRICHMENT_URL environment variable is required")
	}
	if err := validateURL(cfg.MetadataEnrichmentURL, "METADATA_ENRICHMENT_URL"); err != nil {
		return nil, err
	}

	// Required: TORRENT_MANAGER_URL
	cfg.TorrentManagerURL = os.Getenv("TORRENT_MANAGER_URL")
	if cfg.TorrentManagerURL == "" {
		return nil, fmt.Errorf("TORRENT_MANAGER_URL environment variable is required")
	}
	if err := validateURL(cfg.TorrentManagerURL, "TORRENT_MANAGER_URL"); err != nil {
		return nil, err
	}

	// Required: VPN_MANAGER_URL
	cfg.VPNManagerURL = os.Getenv("VPN_MANAGER_URL")
	if cfg.VPNManagerURL == "" {
		return nil, fmt.Errorf("VPN_MANAGER_URL environment variable is required")
	}
	if err := validateURL(cfg.VPNManagerURL, "VPN_MANAGER_URL"); err != nil {
		return nil, err
	}

	// Port (default 3202)
	cfg.Port = 3202
	if v := os.Getenv("CONTENT_ACQUISITION_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("CONTENT_ACQUISITION_PORT must be a valid port (got: %s)", v)
		}
		cfg.Port = p
	} else if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p >= 1 && p <= 65535 {
			cfg.Port = p
		}
	}

	// Optional URLs with defaults
	cfg.SubtitleManagerURL = envOrDefault("SUBTITLE_MANAGER_URL", "http://plugin-subtitle-manager:3204")
	if err := validateURL(cfg.SubtitleManagerURL, "SUBTITLE_MANAGER_URL"); err != nil {
		return nil, err
	}

	cfg.MediaProcessingURL = envOrDefault("MEDIA_PROCESSING_URL", "http://plugin-media-processing:3019")
	if err := validateURL(cfg.MediaProcessingURL, "MEDIA_PROCESSING_URL"); err != nil {
		return nil, err
	}

	cfg.NTVBackendURL = envOrDefault("NTV_BACKEND_URL", "http://auth:4000")
	if err := validateURL(cfg.NTVBackendURL, "NTV_BACKEND_URL"); err != nil {
		return nil, err
	}

	// Redis
	cfg.RedisHost = envOrDefault("REDIS_HOST", "redis")
	cfg.RedisPort = 6379
	if v := os.Getenv("REDIS_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("REDIS_PORT must be a valid port (got: %s)", v)
		}
		cfg.RedisPort = p
	}

	// Log level
	cfg.LogLevel = envOrDefault("LOG_LEVEL", "info")

	// RSS check interval (minutes)
	cfg.RSSCheckInterval = 30
	if v := os.Getenv("RSS_CHECK_INTERVAL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("RSS_CHECK_INTERVAL must be a positive integer (got: %s)", v)
		}
		cfg.RSSCheckInterval = n
	}

	return cfg, nil
}

func validateURL(raw, varName string) error {
	_, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL (got: %s)", varName, raw)
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
