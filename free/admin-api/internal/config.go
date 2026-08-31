package internal

import (
	"os"
	"strconv"
)

const DefaultPort = 3212

type Config struct {
	Port              int
	Host              string
	DatabaseURL       string
	PrometheusURL     string
	CacheTTLSeconds   int
	RetentionDays     int
	SnapshotIntervalM int
	WSEnabled         bool
	AdminAPIKey       string
	CORSAllowedOrigin string
	DBPoolMin         int32
	DBPoolMax         int32
}

func LoadConfig() *Config {
	port := DefaultPort
	if v := os.Getenv("ADMIN_API_PLUGIN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	cacheTTL := 30
	if v := os.Getenv("ADMIN_API_CACHE_TTL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cacheTTL = n
		}
	}

	dbPoolMin := int32(2)
	if v := os.Getenv("DB_POOL_MIN"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			dbPoolMin = int32(n)
		}
	}

	dbPoolMax := int32(10)
	if v := os.Getenv("DB_POOL_MAX"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			dbPoolMax = int32(n)
		}
	}

	corsOrigin := os.Getenv("CORS_ALLOWED_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:3021"
	}

	return &Config{
		Port:              port,
		Host:              "0.0.0.0",
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		PrometheusURL:     os.Getenv("PROMETHEUS_URL"),
		CacheTTLSeconds:   cacheTTL,
		RetentionDays:     90,
		SnapshotIntervalM: 5,
		WSEnabled:         os.Getenv("ADMIN_API_WS_ENABLED") == "true",
		AdminAPIKey:       os.Getenv("ADMIN_API_KEY"),
		CORSAllowedOrigin: corsOrigin,
		DBPoolMin:         dbPoolMin,
		DBPoolMax:         dbPoolMax,
	}
}
