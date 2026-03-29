package internal

import (
	"os"
	"strconv"
	"strings"
)

// LoadConfig reads environment variables and returns a Config with defaults.
func LoadConfig() *Config {
	return &Config{
		DatabaseURL:          envStr("DATABASE_URL", ""),
		Port:                 envInt("TORRENT_MANAGER_PORT", 3201),
		VPNManagerURL:        envStr("VPN_MANAGER_URL", ""),
		VPNRequired:          envStr("VPN_REQUIRED", "true") != "false",
		DefaultClient:        envStr("DEFAULT_TORRENT_CLIENT", "transmission"),
		TransmissionHost:     envStr("TRANSMISSION_HOST", "localhost"),
		TransmissionPort:     envInt("TRANSMISSION_PORT", 9091),
		TransmissionUsername: envStr("TRANSMISSION_USERNAME", ""),
		TransmissionPassword: envStr("TRANSMISSION_PASSWORD", ""),
		QBittorrentHost:      envStr("QBITTORRENT_HOST", "localhost"),
		QBittorrentPort:      envInt("QBITTORRENT_PORT", 8080),
		QBittorrentUsername:  envStr("QBITTORRENT_USERNAME", ""),
		QBittorrentPassword:  envStr("QBITTORRENT_PASSWORD", ""),
		DownloadPath:         envStr("DOWNLOAD_PATH", "/downloads"),
		EnabledSources:       envStr("ENABLED_SOURCES", "1337x,yts,torrentgalaxy,tpb"),
		SearchTimeoutMS:      envInt("SEARCH_TIMEOUT_MS", 10000),
		SearchCacheTTLSec:    envInt("SEARCH_CACHE_TTL_SECONDS", 3600),
		SeedingRatioLimit:    envFloat("SEEDING_RATIO_LIMIT", 2.0),
		SeedingTimeLimitHrs:  envInt("SEEDING_TIME_LIMIT_HOURS", 168),
		MaxActiveDownloads:   envInt("MAX_ACTIVE_DOWNLOADS", 5),
	}
}

// EnabledSourcesList returns the enabled sources as a string slice.
func (c *Config) EnabledSourcesList() []string {
	if c.EnabledSources == "" {
		return nil
	}
	parts := strings.Split(c.EnabledSources, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// envStr returns the environment variable value or a fallback.
func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envInt returns the environment variable as int or a fallback.
func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// envFloat returns the environment variable as float64 or a fallback.
func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
