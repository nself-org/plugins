// Package config loads and validates tenant-controller configuration from env.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the tenant-controller daemon.
type Config struct {
	// Feature flag — must be true to start the controller.
	Enabled bool

	// Admin authentication token (shared secret for CLI <-> controller RPC).
	AdminToken string

	// SQLite registry path.
	ControllerDB string

	// Postgres connection string (superuser, used for DDL).
	DatabaseURL string

	// Hasura
	HasuraAdminSecret string
	HasuraEndpoint    string
	HasuraPortStart   int

	// Domain
	BaseDomain string

	// Schema / Redis prefix settings.
	SchemaPrefix string
	RedisPrefix  string

	// Max projects hard cap.
	MaxProjects int

	// Redis URL (optional, for key-prefix isolation).
	RedisURL string

	// MinIO (optional).
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string

	// Nginx config file path.
	NginxConfPath string

	// HTTP server binding.
	Host string
	Port int

	// Audit log retention (days).
	AuditRetentionDays int

	// Observability.
	OTELEndpoint string
	LogLevel     string

	// E10: Cloud metering feature flag + settings.
	CloudMeteringEnabled  bool
	HetznerNselfToken     string
	StripePriceVcpuHour   string
	StripePriceStorageGBMonth string
	StripePriceEgressGB   string
	CloudFreeVcpuHours    int
	CloudFreeStorageGB    int
	CloudFreeEgressGB     int
	CloudUsageAlertPct    int
}

// Load reads configuration from environment variables.
// Returns an error listing all missing required vars.
func Load() (*Config, error) {
	var missing []string

	require := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	optional := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

	optionalInt := func(key string, def int) int {
		if v := os.Getenv(key); v != "" {
			n, err := strconv.Atoi(v)
			if err == nil {
				return n
			}
		}
		return def
	}

	enabledStr := optional("NSELF_CONTROLLER_ENABLED", "false")
	enabled := strings.EqualFold(enabledStr, "true") ||
		strings.EqualFold(os.Getenv("NSELF_FLAG_MULTI_TENANT_CONTROLLER"), "true")

	cfg := &Config{
		Enabled:            enabled,
		AdminToken:         require("NSELF_CONTROLLER_ADMIN_TOKEN"),
		DatabaseURL:        require("DATABASE_URL"),
		HasuraAdminSecret:  require("HASURA_GRAPHQL_ADMIN_SECRET"),
		HasuraEndpoint:     require("HASURA_GRAPHQL_ENDPOINT"),
		BaseDomain:         require("NSELF_CONTROLLER_BASE_DOMAIN"),
		ControllerDB:       optional("NSELF_CONTROLLER_DB", defaultControllerDB()),
		SchemaPrefix:       optional("NSELF_PROJECT_SCHEMA_PREFIX", "project_"),
		RedisPrefix:        optional("NSELF_PROJECT_REDIS_PREFIX", "proj:"),
		MaxProjects:        optionalInt("NSELF_CONTROLLER_MAX_PROJECTS", 50),
		HasuraPortStart:    optionalInt("NSELF_PROJECT_HASURA_PORT_START", 8080),
		RedisURL:           optional("REDIS_URL", ""),
		MinioEndpoint:      optional("MINIO_ENDPOINT", ""),
		MinioAccessKey:     optional("MINIO_ACCESS_KEY", ""),
		MinioSecretKey:     optional("MINIO_SECRET_KEY", ""),
		NginxConfPath:      optional("NGINX_CONF_PATH", "/etc/nginx/conf.d/nself-projects.conf"),
		Host:               optional("NSELF_CONTROLLER_HOST", "127.0.0.1"),
		Port:               optionalInt("NSELF_CONTROLLER_PORT", 3750),
		AuditRetentionDays: optionalInt("NSELF_AUDIT_RETENTION_DAYS", 90),
		OTELEndpoint:       optional("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		LogLevel:           optional("LOG_LEVEL", "info"),

		// E10: Cloud metering (flag Y17 — cloud_metering).
		CloudMeteringEnabled:      strings.EqualFold(optional("NSELF_FLAG_CLOUD_METERING", "false"), "true"),
		HetznerNselfToken:         optional("HETZNER_NSELF_TOKEN", ""),
		StripePriceVcpuHour:       optional("STRIPE_PRICE_ID_VCPU_HOUR", ""),
		StripePriceStorageGBMonth: optional("STRIPE_PRICE_ID_STORAGE_GB_MONTH", ""),
		StripePriceEgressGB:       optional("STRIPE_PRICE_ID_EGRESS_GB", ""),
		CloudFreeVcpuHours:        optionalInt("CLOUD_FREE_VCPU_HOURS", 500),
		CloudFreeStorageGB:        optionalInt("CLOUD_FREE_STORAGE_GB", 50),
		CloudFreeEgressGB:         optionalInt("CLOUD_FREE_EGRESS_GB", 100),
		CloudUsageAlertPct:        optionalInt("CLOUD_USAGE_ALERT_PCT", 80),
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

func defaultControllerDB() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	return home + "/.nself/controller.db"
}
