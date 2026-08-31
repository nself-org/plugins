package internal

import (
	"os"
	"strconv"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Port             string
	MaxWidth         int
	MaxHeight        int
	CacheTTLDays     int
	RateLimit        int // requests per minute per IP
	SourceStorageURL string
	DiskCacheDir     string
	SigningSecret    string
	RedisURL         string
}

// LoadConfig reads env vars and returns a Config with sane defaults.
func LoadConfig() Config {
	cfg := Config{
		Port:             getEnv("STORAGE_TRANSFORM_PORT", "3085"),
		MaxWidth:         getEnvInt("STORAGE_TRANSFORM_MAX_WIDTH", 4000),
		MaxHeight:        getEnvInt("STORAGE_TRANSFORM_MAX_HEIGHT", 4000),
		CacheTTLDays:     getEnvInt("STORAGE_TRANSFORM_CACHE_TTL_DAYS", 7),
		RateLimit:        getEnvInt("STORAGE_TRANSFORM_RATE_LIMIT", 100),
		SourceStorageURL: getEnv("STORAGE_TRANSFORM_SOURCE_STORAGE_URL", "http://127.0.0.1:3060"),
		DiskCacheDir:     getEnv("STORAGE_TRANSFORM_DISK_CACHE_DIR", "/var/cache/nself/images"),
		SigningSecret:    getEnv("STORAGE_TRANSFORM_SIGNING_SECRET", ""),
		RedisURL:         getEnv("REDIS_URL", "redis://127.0.0.1:6379"),
	}
	return cfg
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
